package gateway

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticGzip(t *testing.T) {
	js := strings.Repeat("console.log('塔台');\n", 200)
	png := strings.Repeat("\x89PNG", 200)
	s := &Server{Options: Options{Web: fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html>" + strings.Repeat("<div></div>", 100))},
		"assets/app-1.js":  {Data: []byte(js)},
		"assets/logo.png":  {Data: []byte(png)},
		"assets/tiny-2.js": {Data: []byte("x")},
	}}}
	h := s.static()
	get := func(path, enc string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if enc != "" {
			r.Header.Set("Accept-Encoding", enc)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := get("/assets/app-1.js", "gzip, deflate, br")
	if w.Header().Get("Content-Encoding") != "gzip" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("脚本应该压缩发送：%v", w.Header())
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if out, _ := io.ReadAll(zr); string(out) != js {
		t.Fatal("解压后的内容和原文件不一致")
	}
	if !strings.Contains(w.Header().Get("Vary"), "Accept-Encoding") || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("缓存头不对：%v", w.Header())
	}

	// 不支持 gzip、明确拒绝 gzip、图片、压缩后不变小的文件：原样发送。
	for _, c := range []struct{ path, enc string }{
		{"/assets/app-1.js", ""},
		{"/assets/app-1.js", "gzip;q=0"},
		{"/assets/logo.png", "gzip"},
		{"/assets/tiny-2.js", "gzip"},
	} {
		w := get(c.path, c.enc)
		if w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != "" {
			t.Fatalf("%s（%q）不该压缩：%d %v", c.path, c.enc, w.Code, w.Header())
		}
	}

	// 单页应用的路由回退到 index.html，也压缩。
	w = get("/hosts/web-1", "gzip")
	if w.Header().Get("Content-Encoding") != "gzip" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("页面应该压缩并且不缓存：%v", w.Header())
	}
}
