package apps

import "testing"

// 带网页面板的应用都标了 panel，按钮才有地方可开；没有页面的（数据库、接口）不标。
func TestPanels(t *testing.T) {
	c := catalog(t)
	want := map[string]bool{"3x-ui": true, "litellm": true, "n8n": true, "new-api": true, "open-webui": true, "openlist": true, "portainer": true, "uptime-kuma": true}
	for _, a := range c.List() {
		got := false
		if a.Guide != nil {
			for _, l := range a.Guide.Open {
				got = got || l.Panel
			}
		}
		if got != want[a.Name] {
			t.Errorf("%s 的网页面板标记是 %v，应该是 %v", a.Name, got, want[a.Name])
		}
	}
	// 面板地址跟着这个实例当前的参数走：改了端口和监听地址，渲染出来的入口也跟着变。
	a, _ := c.Get("openlist")
	g, err := a.GuideFor(Record{App: a.Name, Type: a.Type, Instance: "files", Params: map[string]string{"bind": "127.0.0.1", "port": "6000"}})
	if err != nil || !g.Open[0].Panel || g.Open[0].Port != "6000" || !g.Open[0].Local {
		t.Errorf("面板入口没有跟着参数变：%+v %v", g, err)
	}
	if _, err := Parse([]byte(`apiVersion: hostower/v1
kind: App
name: two-panels
version: 1.0.0-1
title: x
category: 运维
icon: box
summary: x
type: compose
compose: |
  services:
    x:
      image: example/x:1
guide:
  open:
    - {title: a, port: "80", panel: true}
    - {title: b, port: "81", panel: true}
`)); err == nil {
		t.Error("两个网页面板应该被拒绝")
	}
}

func TestPanelSetting(t *testing.T) {
	ok := []PanelSetting{
		{Mode: PanelURL, URL: "https://panel.example.com/xui/"},
		{Mode: PanelURL, URL: "http://192.0.2.10:8080"},
		{Mode: PanelURL, URL: "https://[2001:db8::1]:8443/ui/"},
		{Mode: PanelDirect, Port: "54321", Path: "abc/"},
		{Mode: PanelSSH},
		{Mode: PanelSSH, LocalPort: "12053", Path: "/xui/"},
	}
	for _, p := range ok {
		keep, err := p.Normalize()
		if err != nil || !keep {
			t.Errorf("%+v 应该能保存：%v %v", p, keep, err)
		}
	}
	p := PanelSetting{Mode: PanelDirect, Path: "abc/"}
	if _, err := p.Normalize(); err != nil || p.Path != "/abc/" {
		t.Errorf("路径要补上开头的 /：%q %v", p.Path, err)
	}
	// 什么都不覆盖的直接访问就是默认值，不用存。
	if keep, err := (&PanelSetting{}).Normalize(); keep || err != nil {
		t.Errorf("默认值不该保存：%v %v", keep, err)
	}
	bad := []PanelSetting{
		{Mode: PanelURL, URL: ""},
		{Mode: PanelURL, URL: "ftp://example.com/"},
		{Mode: PanelURL, URL: "javascript:alert(1)"},
		{Mode: PanelURL, URL: "https://admin:secret@panel.example.com/"},
		{Mode: PanelURL, URL: "https://panel.example.com/?token=abc"},
		{Mode: PanelURL, URL: "https://panel.example.com/#token"},
		{Mode: PanelURL, URL: "https://panel.example.com/a b"},
		{Mode: PanelURL, URL: "https://panel.example.com:99999/"},
		{Mode: PanelURL, URL: "//panel.example.com/"},
		{Mode: PanelDirect, Port: "0"},
		{Mode: PanelDirect, Path: "/a?b"},
		{Mode: PanelSSH, LocalPort: "70000"},
		{Mode: "proxy"},
	}
	for _, p := range bad {
		if _, err := p.Normalize(); err == nil {
			t.Errorf("%+v 应该被拒绝", p)
		}
	}
	if ParsePanelSetting(`{"v":1,"mode":"url","url":"https://admin:x@example.com/"}`) != nil {
		t.Error("存着的设置也要重新校验")
	}
	if p := ParsePanelSetting(`{"v":1,"mode":"ssh","localPort":"12053"}`); p == nil || p.LocalPort != "12053" {
		t.Errorf("读不出存着的设置：%+v", p)
	}
}
