package i18n

import "testing"

func TestT(t *testing.T) {
	cases := []struct{ lang, want string }{
		{"zh-CN", "主机离线：web-1"},
		{"", "主机离线：web-1"},   // 不认识的语言按原文
		{"xx", "主机离线：web-1"}, // 同上
		{"en", "Host offline: web-1"},
		{"ja", "ホストがオフライン：web-1"},
		{"zh-TW", "主機離線：web-1"},
	}
	for _, c := range cases {
		if got := T(c.lang, "主机离线：{host}", "host", "web-1"); got != c.want {
			t.Errorf("%q：%q，应为 %q", c.lang, got, c.want)
		}
	}
	// 没有译文时：日文退到英文，繁体退回原文。
	dicts["en"]["测试专用"] = "test only"
	defer delete(dicts["en"], "测试专用")
	if T("ja", "测试专用") != "test only" || T("zh-TW", "测试专用") != "测试专用" {
		t.Fatal("退回的顺序不对")
	}
	if Get("xx").Code != Source() || Get("fr").Colon == "" {
		t.Fatal("语言设置不对")
	}
}
