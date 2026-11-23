package gateway

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// 面板地址：所有者给实例设一个覆盖值，列表里带着它；带账号密码的地址不收；每次改动写审计；恢复默认就删掉。
func TestAppPanel(t *testing.T) {
	s := appSetup(t)
	a, _ := s.apps.Get("3x-ui")
	vals, _ := a.Values(nil)
	in, err := a.PlanInstall("3x-ui", vals, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	runApproved(t, s, "web-1", in.Command)

	put := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.appPanelSet(w, asOwner(httptest.NewRequest("PUT", "/api/v1/apps/panel", strings.NewReader(body))))
		return w
	}
	audited := func() int {
		recs, err := s.Kernel.Audit().List(context.Background(), 0, 500)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, r := range recs {
			if strings.Contains(string(r.Body), `"type":"app.panel"`) {
				n++
			}
		}
		return n
	}
	if w := put(`{"host":"web-1","instance":"3x-ui","panel":{"mode":"url","url":"https://admin:pw@panel.example.com/"}}`); w.Code != 400 {
		t.Fatalf("带密码的地址应该被拒绝：%d %s", w.Code, w.Body.String())
	}
	if w := put(`{"host":"web-1","instance":"nope","panel":{"mode":"url","url":"https://panel.example.com/"}}`); w.Code != 404 {
		t.Fatalf("没装的实例不能设：%d %s", w.Code, w.Body.String())
	}
	if w := put(`{"host":"web-1","instance":"3x-ui","panel":{"mode":"direct","port":"54321","path":"/xui/"}}`); w.Code != 200 {
		t.Fatalf("设置面板地址：%d %s", w.Code, w.Body.String())
	}
	list := s.installedOn(context.Background(), testOwner, []string{"web-1"})[0]
	if p := list.Installed[0].Panel; p == nil || p.Port != "54321" || p.Path != "/xui/" {
		t.Fatalf("列表里没有带上面板地址：%+v", p)
	}
	if n := audited(); n != 1 {
		t.Fatalf("改面板地址要写审计：%d 条", n)
	}
	// 没变就不再写一条。
	put(`{"host":"web-1","instance":"3x-ui","panel":{"mode":"direct","port":"54321","path":"/xui/"}}`)
	if n := audited(); n != 1 {
		t.Fatalf("没变的设置不该再写审计：%d 条", n)
	}
	if w := put(`{"host":"web-1","instance":"3x-ui","panel":{"mode":"direct"}}`); w.Code != 200 {
		t.Fatalf("恢复默认：%d %s", w.Code, w.Body.String())
	}
	list = s.installedOn(context.Background(), testOwner, []string{"web-1"})[0]
	if list.Installed[0].Panel != nil || audited() != 2 {
		t.Fatalf("恢复默认以后不该再有覆盖值：%+v", list.Installed[0].Panel)
	}
}
