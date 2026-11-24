package gateway

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

// gzipFiles 把内嵌前端里的文本文件（脚本、样式、页面、图标）压缩一次后放在内存里，
// 浏览器支持 gzip 时直接发压缩过的。前端文件编进了程序、运行时不会变，所以按路径缓存就够了。
// 不压缩的话，首次打开（以及每次升级后的第一次）要下载几 MB 的脚本，远程访问时很慢。
type gzipFiles struct {
	fsys fs.FS
	m    sync.Map // 路径 → []byte（压缩后的内容）；压缩后不变小的存 nil
}

// 要压缩的类型和它们的 Content-Type。已经压缩过的格式（字体、图片）再压一遍没有用。
// 类型写死在这里，不查系统的 MIME 表：Windows 上注册表可能把 .js 登记成 text/plain。
var gzipTypes = map[string]string{
	".js":   "text/javascript; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".svg":  "image/svg+xml",
	".json": "application/json",
	".txt":  "text/plain; charset=utf-8",
}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, q, _ := strings.Cut(strings.TrimSpace(part), ";")
		if strings.EqualFold(strings.TrimSpace(name), "gzip") && strings.ReplaceAll(strings.TrimSpace(q), " ", "") != "q=0" {
			return true
		}
	}
	return false
}

// get 返回压缩后的内容；不该压缩或压缩后不更小时返回 nil。
func (g *gzipFiles) get(p string) []byte {
	if gzipTypes[strings.ToLower(path.Ext(p))] == "" {
		return nil
	}
	if v, ok := g.m.Load(p); ok {
		return v.([]byte)
	}
	data, err := fs.ReadFile(g.fsys, p)
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(data)
	zw.Close()
	var out []byte
	if buf.Len() < len(data) {
		out = buf.Bytes()
	}
	g.m.Store(p, out)
	return out
}

// serve 在浏览器支持时发压缩过的文件，返回是否已经处理。
func (g *gzipFiles) serve(w http.ResponseWriter, r *http.Request, p string) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	// 不管这次发没发压缩版，缓存都要按 Accept-Encoding 区分。
	w.Header().Add("Vary", "Accept-Encoding")
	if !acceptsGzip(r) || r.Header.Get("Range") != "" {
		return false
	}
	z := g.get(p)
	if z == nil {
		return false
	}
	h := w.Header()
	h.Set("Content-Type", gzipTypes[strings.ToLower(path.Ext(p))])
	h.Set("Content-Encoding", "gzip")
	h.Set("Content-Length", strconv.Itoa(len(z)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		w.Write(z)
	}
	return true
}
