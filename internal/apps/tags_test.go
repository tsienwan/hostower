package apps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewerTags(t *testing.T) {
	tags := []string{"latest", "v3.9.0", "3.9.0", "v3.10.2", "v3.11.0", "3.11.0", "v3.12.0-rc1", "v4.0.1", "v4.1.0", "v2.9.9"}
	same, major := NewerTags("v3.9.0", tags)
	if same != "v3.11.0" || major != "v4.1.0" {
		t.Errorf("v3.9.0：得到 %q %q，期望 v3.11.0、v4.1.0", same, major)
	}
	if same, _ := NewerTags("3.9.0", tags); same != "3.11.0" {
		t.Errorf("不带 v 的写法只和不带 v 的比：得到 %q", same)
	}
	if same, major := NewerTags("v4.1.0", tags); same != "" || major != "" {
		t.Errorf("已经是最新：得到 %q %q", same, major)
	}
	if same, major := NewerTags("lts", tags); same != "" || major != "" {
		t.Error("不是 x.y.z 写法时不比较")
	}
	if same, _ := NewerTags("v3.9.0", []string{"v3.10.0", "v3.9.10"}); same != "v3.10.0" {
		t.Errorf("按数字比较而不是按字符：得到 %q", same)
	}
}

func TestTagsPaging(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.Write([]byte(`{"token":"t"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer t" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="s",scope="repository:mhsanaei/3x-ui:pull"`)
			w.WriteHeader(401)
			return
		}
		if r.URL.Query().Get("last") == "" {
			w.Header().Set("Link", `</v2/mhsanaei/3x-ui/tags/list?n=1000&last=v3.9.0>; rel="next"`)
			w.Write([]byte(`{"tags":["v3.8.5","v3.9.0"]}`))
			return
		}
		w.Write([]byte(`{"tags":["v3.10.2","v3.11.0"]}`))
	}))
	defer srv.Close()
	r := NewRegistry()
	r.RegistryBase = func(string) string { return srv.URL }
	tags, err := r.Tags(context.Background(), "ghcr.io/mhsanaei/3x-ui:v3.9.0")
	if err != nil || strings.Join(tags, ",") != "v3.8.5,v3.9.0,v3.10.2,v3.11.0" {
		t.Fatalf("翻页读标签不对：%v %v", tags, err)
	}
}
