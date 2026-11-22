package gateway

import (
	"archive/zip"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"encoding/json"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/snippet"
)

// 文件同步：把文件分发到若干台主机，或者把若干台主机上匹配规则的文件收集下来。两件事都没有自己的通道。
//
// 分发：文件先暂存在 Hostower 上（和单台上传一样），每个文件变成一个请求——所有目标主机上各执行一条
// “mkdir -p 目录 && cat > 目标路径”，文件内容从标准输入送过去。判级、审批、摘要绑定、审计和手敲命令一样。
//
// 收集：先用一条只读的 find（内核的只读查询，只能是 L0）列出每台主机上匹配的文件，
// 再逐个走“下载文件”那条读路径（私钥、凭据文件照样读不了，每个文件都记审计），边读边打成一个 zip 发给浏览器。

const (
	maxSyncHosts    = 50
	maxSyncFiles    = 20      // 一次分发的文件数
	maxCollectFiles = 200     // 每台主机最多列出、收集的文件数
	maxCollectBytes = 1 << 30 // 一次收集的总大小上限
	maxCollectDepth = 8
)

// collectPatternRe 是文件名的匹配规则：普通的文件名字符加通配符 * ? [ ]，不能带路径分隔符。
var collectPatternRe = regexp.MustCompile(`^[A-Za-z0-9._*?\[\]@+=, -]{1,100}$`)

func (s *Server) syncHosts(names []string) ([]string, error) {
	var hosts []string
	seen := map[string]bool{}
	for _, h := range names {
		if h = strings.TrimSpace(h); h == "" || seen[h] {
			continue
		}
		if _, ok := s.Kernel.Inventory().Host(h); !ok {
			return nil, fmt.Errorf("主机 %s 不存在", h)
		}
		seen[h] = true
		hosts = append(hosts, h)
	}
	if len(hosts) == 0 || len(hosts) > maxSyncHosts {
		return nil, errors.New("请选择 1 到 " + strconv.Itoa(maxSyncHosts) + " 台主机")
	}
	return hosts, nil
}

// fileDistribute 把暂存好的文件分发到若干台主机的同一个目录下：每个文件一个请求。
func (s *Server) fileDistribute(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hosts []string `json:"hosts"`
		Dir   string   `json:"dir"`
		Files []struct {
			Name   string `json:"name"`
			Upload string `json:"upload"`
		} `json:"files"`
		Strategy kernel.Strategy `json:"strategy"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	hosts, err := s.syncHosts(in.Hosts)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	dir, ok := cleanRemote(in.Dir)
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	if len(in.Files) == 0 || len(in.Files) > maxSyncFiles {
		writeErr(w, 400, "一次分发 1 到 "+strconv.Itoa(maxSyncFiles)+" 个文件")
		return
	}
	type staged struct {
		name string
		info uploadInfo
	}
	var files []staged
	names := map[string]bool{}
	for _, f := range in.Files {
		if !validFileName(f.Name) || names[f.Name] {
			writeErr(w, 400, "文件名不合法或重复")
			return
		}
		names[f.Name] = true
		up, err := s.Kernel.UploadPath(f.Upload)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		meta, err := os.ReadFile(up + ".json")
		var info uploadInfo
		if err != nil || json.Unmarshal(meta, &info) != nil {
			writeErr(w, 400, "上传的文件不存在或已过期")
			return
		}
		files = append(files, staged{f.Name, info})
	}
	st := in.Strategy
	if st.Concurrency <= 0 {
		st.Concurrency = 4
	}
	a := actor(sessionOf(r))
	var reqs []*kernel.Request
	for _, f := range files {
		target := path.Join(dir, f.name)
		cmd := "mkdir -p " + snippet.Quote(dir) + " && cat > " + snippet.Quote(target)
		spec := kernel.CommandSpec{Hosts: hosts, Commands: map[string]string{}, Strategy: st,
			Intent: "分发文件 " + target,
			Input:  &kernel.Input{Kind: "upload", Label: f.name, Upload: f.info.ID, Size: f.info.Size, SHA256: f.info.SHA256}}
		if len(hosts) > 1 {
			spec.Intent += "（" + strconv.Itoa(len(hosts)) + " 台主机）"
		}
		for _, h := range hosts {
			spec.Commands[h] = cmd
		}
		req, err := s.Kernel.SubmitCommand(r.Context(), a, spec)
		if err != nil {
			if len(reqs) == 0 {
				fail(w, err)
			} else {
				// 前面的请求已经提交了：把它们和这次的错误一起告诉页面。
				writeJSON(w, 200, map[string]any{"requests": reqs, "error": err.Error()})
			}
			return
		}
		reqs = append(reqs, req)
	}
	writeJSON(w, 200, map[string]any{"requests": reqs})
}

// collectQuery 是“收集”的参数：哪些主机、哪个目录、文件名规则、往下找几层。
type collectQuery struct {
	hosts   []string
	dir     string
	pattern string
	depth   int
}

func (s *Server) parseCollect(r *http.Request) (collectQuery, error) {
	q := r.URL.Query()
	var c collectQuery
	var err error
	if c.hosts, err = s.syncHosts(strings.Split(q.Get("hosts"), ",")); err != nil {
		return c, err
	}
	var ok bool
	if c.dir, ok = cleanRemote(q.Get("dir")); !ok {
		return c, errors.New("路径必须是绝对路径")
	}
	c.pattern = strings.TrimSpace(q.Get("pattern"))
	if !collectPatternRe.MatchString(c.pattern) {
		return c, errors.New("文件名规则只能用字母、数字、. _ - 和通配符 * ? [ ]，不能带路径")
	}
	c.depth = 1
	if d := q.Get("depth"); d != "" {
		if c.depth, err = strconv.Atoi(d); err != nil || c.depth < 1 || c.depth > maxCollectDepth {
			return c, errors.New("目录层数要在 1 到 " + strconv.Itoa(maxCollectDepth) + " 之间")
		}
	}
	return c, nil
}

// collectCommand 是列出匹配文件的只读命令：不跨文件系统、只要普通文件、最多 maxCollectFiles 条。
// 能给出大小就给（GNU find 的 -printf）；不支持 -printf 的 find（busybox）只列路径。必须是 L0。
func collectCommand(c collectQuery) string {
	base := "find " + snippet.Quote(c.dir) + " -xdev -maxdepth " + strconv.Itoa(c.depth) + " -type f -name " + snippet.Quote(c.pattern)
	return "{ " + base + ` -printf '%s\t%p\n' 2>/dev/null || ` + base + " 2>/dev/null; } | head -n " + strconv.Itoa(maxCollectFiles+1)
}

type collectFile struct {
	Path string `json:"path"`
	// Size 是字节数；主机上的 find 给不出大小时是 -1。
	Size int64 `json:"size"`
}

type collectHost struct {
	Host      string        `json:"host"`
	Files     []collectFile `json:"files"`
	Truncated bool          `json:"truncated,omitempty"`
	Error     string        `json:"error,omitempty"`
}

func parseCollect(out, dir string) ([]collectFile, bool) {
	files := []collectFile{}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		size := int64(-1)
		p := line
		if a, b, ok := strings.Cut(line, "\t"); ok {
			if n, err := strconv.ParseInt(a, 10, 64); err == nil && n >= 0 {
				size, p = n, b
			}
		}
		// 只收 find 在这个目录下面找到的绝对路径；带换行或别的怪东西的文件名解析不出来就跳过。
		if !strings.HasPrefix(p, "/") || seen[p] || path.Clean(p) != p || !strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/") {
			continue
		}
		seen[p] = true
		files = append(files, collectFile{Path: p, Size: size})
	}
	truncated := len(files) > maxCollectFiles
	if truncated {
		files = files[:maxCollectFiles]
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, truncated
}

func (s *Server) collectList(r *http.Request, c collectQuery) []collectHost {
	a := actor(sessionOf(r))
	out := make([]collectHost, len(c.hosts))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, h := range c.hosts {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = collectHost{Host: h, Files: []collectFile{}}
			res, _, err := s.Kernel.Query(r.Context(), a, h, collectCommand(c))
			if err != nil {
				out[i].Error = err.Error()
				return
			}
			out[i].Files, out[i].Truncated = parseCollect(res, c.dir)
		}(i, h)
	}
	wg.Wait()
	return out
}

// fileCollect 列出各台主机上匹配规则的文件（只读查询），给页面预览。
func (s *Server) fileCollect(w http.ResponseWriter, r *http.Request) {
	c, err := s.parseCollect(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"hosts": s.collectList(r, c), "limit": maxCollectFiles})
}

// fileCollectDownload 把匹配的文件打成一个 zip：<主机名>/<文件的绝对路径>。
// 每个文件都走“下载文件”的读路径（L3 规则照旧、每个文件一条审计）；读不了的、超过上限的记在包里的 _跳过的文件.txt 里。
func (s *Server) fileCollectDownload(w http.ResponseWriter, r *http.Request) {
	c, err := s.parseCollect(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	a := actor(sessionOf(r))
	list := s.collectList(r, c)
	total := 0
	for _, h := range list {
		total += len(h.Files)
	}
	if total == 0 {
		writeErr(w, 404, "没有匹配的文件")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="hostower-files-`+time.Now().UTC().Format("20060102-150405")+`.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()
	var skipped []string
	var written int64
	for _, h := range list {
		if h.Error != "" {
			skipped = append(skipped, h.Host+"\t"+h.Error)
			continue
		}
		if h.Truncated {
			skipped = append(skipped, h.Host+"\t匹配的文件超过 "+strconv.Itoa(maxCollectFiles)+" 个，只收了前面这些")
		}
		for _, f := range h.Files {
			if r.Context().Err() != nil {
				return
			}
			if written >= maxCollectBytes {
				skipped = append(skipped, h.Host+"\t"+f.Path+"\t总大小超过上限，没有收")
				continue
			}
			data, err := s.Kernel.DownloadFile(r.Context(), a, h.Host, f.Path)
			if err != nil {
				reason := err.Error()
				switch {
				case errors.Is(err, executor.ErrNotExist):
					reason = "文件不存在"
				case errors.Is(err, kernel.ErrTooLarge):
					reason = "文件超过 100 MB"
				}
				skipped = append(skipped, h.Host+"\t"+f.Path+"\t"+reason)
				continue
			}
			hdr := &zip.FileHeader{Name: h.Host + f.Path, Method: zip.Deflate}
			hdr.Modified = time.Now()
			fw, err := zw.CreateHeader(hdr)
			if err != nil {
				return
			}
			if _, err := fw.Write(data); err != nil {
				return
			}
			written += int64(len(data))
		}
	}
	if len(skipped) > 0 {
		if fw, err := zw.Create("_跳过的文件.txt"); err == nil {
			fw.Write([]byte(strings.Join(skipped, "\n") + "\n"))
		}
	}
}
