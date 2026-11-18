package apps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseImage(t *testing.T) {
	for in, want := range map[string]ImageRef{
		"redis:8-alpine":                  {"registry-1.docker.io", "library/redis", "8-alpine"},
		"louislam/uptime-kuma:2":          {"registry-1.docker.io", "louislam/uptime-kuma", "2"},
		"ghcr.io/mhsanaei/3x-ui:v3.11.0":  {"ghcr.io", "mhsanaei/3x-ui", "v3.11.0"},
		"registry.example.com:5000/a/b:1": {"registry.example.com:5000", "a/b", "1"},
	} {
		got, err := ParseImage(in)
		if err != nil || got != want {
			t.Errorf("ParseImage(%q) = %+v, %v；期望 %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"redis", "redis@sha256:abc", "Redis:1", "a/../b:1", "a:b c"} {
		if _, err := ParseImage(bad); err == nil {
			t.Errorf("ParseImage(%q) 应报错", bad)
		}
	}
	for in, want := range map[string]string{"2": "3", "8-alpine": "9-alpine", "18-alpine": "19-alpine", "v3.11.0": "", "11.8": "", "lts": ""} {
		if got := NextMajorTag(in); got != want {
			t.Errorf("NextMajorTag(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// 假的镜像仓库：先 401 给出 Bearer 挑战，带令牌再来才返回摘要；标签 3 存在，表示上游出了新大版本。
func TestImageDigest(t *testing.T) {
	heads := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			if r.URL.Query().Get("scope") != "repository:louislam/uptime-kuma:pull" {
				http.Error(w, "bad scope", 400)
				return
			}
			w.Write([]byte(`{"token":"t0k"}`))
		case strings.HasPrefix(r.URL.Path, "/v2/louislam/uptime-kuma/manifests/"):
			if r.Method != http.MethodHead {
				http.Error(w, "head only", 405)
				return
			}
			if r.Header.Get("Authorization") != "Bearer t0k" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="registry.test",scope="repository:louislam/uptime-kuma:pull"`)
				w.WriteHeader(401)
				return
			}
			heads++
			switch strings.TrimPrefix(r.URL.Path, "/v2/louislam/uptime-kuma/manifests/") {
			case "2":
				w.Header().Set("Docker-Content-Digest", "sha256:new2")
			case "3":
				w.Header().Set("Docker-Content-Digest", "sha256:three")
			default:
				w.WriteHeader(404)
			}
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	r := NewRegistry()
	r.RegistryBase = func(string) string { return srv.URL }
	ctx := context.Background()
	d, ok, err := r.ImageDigest(ctx, "louislam/uptime-kuma:2")
	if err != nil || !ok || d != "sha256:new2" {
		t.Fatalf("取摘要失败：%q %v %v", d, ok, err)
	}
	if _, ok, err := r.ImageDigest(ctx, "louislam/uptime-kuma:9"); err != nil || ok {
		t.Errorf("不存在的标签应返回 ok=false：%v %v", ok, err)
	}
	before := heads
	r.ImageDigest(ctx, "louislam/uptime-kuma:2")
	if heads != before {
		t.Error("缓存期内不应再请求仓库")
	}
	n := r.CheckImages(ctx, []string{"louislam/uptime-kuma:2"}, map[string]string{"louislam/uptime-kuma:2": "sha256:old"})
	if !n.Rebuilt || n.Major != "3" || n.Remote != "sha256:new2" {
		t.Errorf("应发现新构建和上游大版本 3：%+v", n)
	}
	if n := r.CheckImages(ctx, []string{"louislam/uptime-kuma:2"}, map[string]string{"louislam/uptime-kuma:2": "sha256:new2"}); n.Rebuilt {
		t.Error("摘要一样时不算新构建")
	}
	if n := r.CheckImages(ctx, []string{"louislam/uptime-kuma:2"}, nil); n.Rebuilt || n.Remote != "" {
		t.Error("主机上没有摘要时跳过，不去查仓库")
	}
	r.TTL = time.Nanosecond
	time.Sleep(time.Millisecond)
	r.ImageDigest(ctx, "louislam/uptime-kuma:2")
	if heads == before {
		t.Error("过了缓存期应重新请求")
	}
	c := catalog(t)
	k, _ := c.Get("uptime-kuma")
	if imgs := k.Images(); len(imgs) != 1 || imgs[0] != "louislam/uptime-kuma:2" {
		t.Errorf("清单里的镜像不对：%v", imgs)
	}
}
