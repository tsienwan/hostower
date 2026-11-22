package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/hostenv"
)

func asOwner(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ctxKey{}, &auth.Session{User: &auth.User{ID: "u1", Name: "owner", Role: "owner"}}))
}

// 装好 3x-ui 以后：已安装列表里带着渲染好的上手指南，“读取初始账号”走只读查询把日志里那一行读回来。
func TestAppGuideAndReveal(t *testing.T) {
	s := appSetup(t)
	a, _ := s.apps.Get("3x-ui")
	vals, _ := a.Values(nil)
	in, err := a.PlanInstall("3x-ui", vals, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	runApproved(t, s, "web-1", in.Command)

	list := s.installedOn(context.Background(), testOwner, []string{"web-1"})[0]
	if list.Error != "" || len(list.Installed) != 1 {
		t.Fatalf("已安装列表不对：%+v", list)
	}
	g := list.Installed[0].Guide
	if g == nil || len(g.Open) != 1 || g.Open[0].Port != "2053" || g.Reveal == nil || len(g.Steps) == 0 {
		t.Fatalf("上手指南不对：%+v", g)
	}

	w := httptest.NewRecorder()
	s.appReveal(w, asOwner(httptest.NewRequest("GET", "/api/v1/apps/reveal?host=web-1&instance=3x-ui", nil)))
	var out struct{ Output string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || !strings.Contains(out.Output, "Username:") {
		t.Fatalf("读取初始账号：%d %s", w.Code, w.Body.String())
	}
	// 没有这个实例、实例名不合法：都读不了。
	for _, q := range []string{"host=web-1&instance=nope", "host=web-1&instance=x;reboot", "host=nope&instance=3x-ui"} {
		w := httptest.NewRecorder()
		s.appReveal(w, asOwner(httptest.NewRequest("GET", "/api/v1/apps/reveal?"+strings.ReplaceAll(q, ";", "%3B"), nil)))
		if w.Code == 200 {
			t.Errorf("%s 不应读得出来：%s", q, w.Body.String())
		}
	}
}

// 环境检测：命令、接口、出口位置都查得出来（模拟主机）。
func TestHostEnv(t *testing.T) {
	s := appSetup(t)
	r := httptest.NewRequest("GET", "/api/v1/hosts/web-1/env", nil)
	r.SetPathValue("name", "web-1")
	w := httptest.NewRecorder()
	s.hostEnv(w, asOwner(r))
	var out struct {
		Tools    []hostenv.ToolStatus
		Services []hostenv.ServiceStatus
		Sites    []hostenv.SiteStatus
		Egress   hostenv.Egress
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(out.Tools) != len(hostenv.Tools) || len(out.Services) != len(hostenv.Services) || len(out.Sites) != len(hostenv.Sites) || out.Egress.V4 == nil || out.Egress.V4.Country == "" || out.Egress.V4.Type == "" {
		t.Fatalf("检测结果不完整：%+v", out)
	}
	for _, sv := range out.Services {
		if sv.State == "" || sv.Code == 0 && sv.State != hostenv.StateUnreachable {
			t.Errorf("%s 的结论不对：%+v", sv.ID, sv)
		}
	}
}
