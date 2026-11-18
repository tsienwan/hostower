package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/snippet"
)

// 文件的下载和上传。
// 下载是只读操作：和“查看文件”走内核里同一条读路径（私钥、凭据文件照样读不了），只是上限更大。
// 上传没有自己的通道：文件先暂存在 Hostower 上，再变成一条“cat > 目标路径”的命令请求，
// 文件内容从标准输入送过去。判级、审批、摘要绑定（换了文件批准就作废）、审计都和手敲命令一样。

func (s *Server) fileDownload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, ok := cleanRemote(q.Get("path"))
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	// preview=1：页面要在浏览器里预览它（图片、PDF、Office 文档）。上限小一些，审计里记为“读文件”。
	// 返回的东西一样是按附件下载的二进制，页面用脚本取回后自己解析，浏览器不会直接把它当网页打开。
	preview := q.Get("preview") == "1"
	read := s.Kernel.DownloadFile
	if preview {
		read = s.Kernel.PreviewFile
	}
	data, err := read(r.Context(), actor(sessionOf(r)), q.Get("host"), p)
	if err != nil {
		if errors.Is(err, executor.ErrNotExist) {
			writeErr(w, 404, "文件不存在")
			return
		}
		if errors.Is(err, kernel.ErrTooLarge) {
			if preview {
				writeErr(w, 413, "文件超过 20 MB，页面上预览不了，可以下载后查看")
			} else {
				writeErr(w, 413, "文件超过 100 MB，页面上下载不了，请用 scp 之类的工具")
			}
			return
		}
		fail(w, err)
		return
	}
	name := path.Base(p)
	// 一律按附件下载，不让浏览器把主机上的文件当网页打开。
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func validFileName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 255 && !strings.ContainsAny(name, "/\x00\n\r")
}

func (s *Server) fileUpload(w http.ResponseWriter, r *http.Request) {
	var in struct{ Host, Dir, Name, Upload string }
	if !readJSON(w, r, &in) {
		return
	}
	dir, ok := cleanRemote(in.Dir)
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	if !validFileName(in.Name) {
		writeErr(w, 400, "文件名不合法")
		return
	}
	up, err := s.Kernel.UploadPath(in.Upload)
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
	target := path.Join(dir, in.Name)
	spec := kernel.CommandSpec{
		Hosts:    []string{in.Host},
		Commands: map[string]string{in.Host: "cat > " + snippet.Quote(target)},
		Intent:   "上传文件 " + target,
		Input:    &kernel.Input{Kind: "upload", Label: in.Name, Upload: info.ID, Size: info.Size, SHA256: info.SHA256},
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), spec)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

// fileSearch 在一个目录下面按名字找文件。它是一条只读的 find 命令，走内核的只读查询。
// 不跨文件系统、最多往下六层、最多两百条：这是在页面上找东西，不是全盘扫描。
func (s *Server) fileSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dir, ok := cleanRemote(q.Get("path"))
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	word := strings.TrimSpace(q.Get("q"))
	if word == "" || len(word) > 100 || strings.ContainsAny(word, "*?[]\\/\x00\n\r") {
		writeErr(w, 400, "搜索词不能为空，也不能含 * ? [ ] \\ / 这些符号")
		return
	}
	cmd := "find " + snippet.Quote(dir) + " -xdev -maxdepth 6 -iname " + snippet.Quote("*"+word+"*") + " 2>/dev/null | head -n 200"
	out, _, err := s.Kernel.Query(r.Context(), actor(sessionOf(r)), q.Get("host"), cmd)
	if err != nil {
		fail(w, err)
		return
	}
	paths := []string{}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimRight(line, "\r"); strings.HasPrefix(line, "/") && line != dir {
			paths = append(paths, line)
		}
	}
	writeJSON(w, 200, map[string]any{"paths": paths, "truncated": len(paths) >= 199})
}
