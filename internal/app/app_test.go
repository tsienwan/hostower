package app

import "testing"

func TestSelfCheck(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"本机回环", Config{Listen: "127.0.0.1:18443", PublicURL: "http://localhost:18443"}, true},
		{"开发模式回环", Config{Listen: "127.0.0.1:18443", PublicURL: "http://localhost:18443", Dev: true}, true},
		{"开发模式不能对外", Config{Listen: "0.0.0.0:18443", PublicURL: "http://localhost:18443", Dev: true}, false},
		{"开发模式不能用外部域名", Config{Listen: "127.0.0.1:18443", PublicURL: "https://tower.example.com", Dev: true}, false},
		{"对外没有 TLS", Config{Listen: "0.0.0.0:18443", PublicURL: "http://tower.example.com"}, false},
		{"对外没有 TLS（https 地址也不行）", Config{Listen: "0.0.0.0:18443", PublicURL: "https://tower.example.com"}, false},
		{"自己终结 TLS", Config{Listen: "0.0.0.0:18443", PublicURL: "https://tower.example.com", TLSCert: "c", TLSKey: "k"}, true},
		{"有证书但地址是 http", Config{Listen: "0.0.0.0:18443", PublicURL: "http://tower.example.com", TLSCert: "c", TLSKey: "k"}, false},
		{"只给了证书没给私钥", Config{Listen: "127.0.0.1:18443", PublicURL: "http://localhost:18443", TLSCert: "c"}, false},
		{"代理终结 TLS", Config{Listen: "0.0.0.0:18443", PublicURL: "https://tower.example.com", TLSMode: "proxy"}, true},
		{"代理模式但地址是 http", Config{Listen: "0.0.0.0:18443", PublicURL: "http://tower.example.com", TLSMode: "proxy"}, false},
		{"容器内监听、只映射到本机", Config{Listen: "0.0.0.0:18443", PublicURL: "http://localhost:18443", TLSMode: "proxy"}, true},
		{"未知 TLS 模式", Config{Listen: "127.0.0.1:18443", PublicURL: "http://localhost:18443", TLSMode: "off"}, false},
	}
	for _, c := range cases {
		c.cfg.finalize()
		err := c.cfg.SelfCheck()
		if (err == nil) != c.ok {
			t.Errorf("%s：期望通过=%v，得到 %v", c.name, c.ok, err)
		}
	}
}

func TestPanelPrecedence(t *testing.T) {
	// 环境变量设过的项，面板设置改不动；没设过的项用面板设置。
	env := Config{Listen: "127.0.0.1:18443", FromEnv: map[string]bool{"listen": true}}
	got := env.WithPanel(map[string]string{"listen": "0.0.0.0:9000", "public_url": "https://tower.example.com", "tls_mode": "proxy", "session_minutes": "60"})
	if got.Listen != "127.0.0.1:18443" {
		t.Errorf("环境变量应优先，得到 %s", got.Listen)
	}
	if got.PublicURL != "https://tower.example.com" || got.TLSMode != "proxy" || got.SessionMinutes != 60 {
		t.Errorf("面板设置没有生效：%+v", got)
	}
	// 面板设置留空时回到默认值。
	def := Config{Listen: "127.0.0.1:18443", FromEnv: map[string]bool{}}.WithPanel(map[string]string{})
	if def.PublicURL != "http://localhost:18443" || def.SessionMinutes != 720 {
		t.Errorf("默认值不对：%+v", def)
	}
	// 会让塔台对外裸奔的面板设置过不了自检。
	bad := Config{Listen: "127.0.0.1:18443", FromEnv: map[string]bool{}}.WithPanel(map[string]string{"listen": "0.0.0.0:18443"})
	if bad.SelfCheck() == nil {
		t.Error("对外监听而没有 TLS 的面板设置应被自检拒绝")
	}
}

func TestPanelWithoutEnv(t *testing.T) {
	// 环境变量对外监听、声明了代理：去掉环境变量之后回到默认（只监听本机）。
	env := Config{Listen: "0.0.0.0:18443", TLSMode: "proxy", PublicURL: "https://198.51.100.7:18443",
		FromEnv: map[string]bool{"listen": true, "tls_mode": true, "public_url": true}}
	bare := env.withoutPanelEnv()
	if bare.Listen != "127.0.0.1:18443" || bare.TLSMode != "" || bare.PublicURL != "" || len(bare.FromEnv) != 0 {
		t.Fatalf("去掉环境变量之后的配置不对：%+v", bare)
	}
	// 在页面上预先保存了对外监听、但没配 TLS：去掉环境变量之后起不来，应该被拒绝。
	if err := bare.WithPanel(map[string]string{"listen": "0.0.0.0:18443"}).check(); err == nil {
		t.Error("去掉环境变量后对外裸奔的设置应被拒绝")
	}
	// 预先保存的设置完整（对外监听 + 反向代理）：去掉环境变量之后能起来。
	if err := bare.WithPanel(map[string]string{"listen": "0.0.0.0:18443", "tls_mode": "proxy", "public_url": "https://198.51.100.7:18443"}).check(); err != nil {
		t.Errorf("完整的设置不应被拒绝：%v", err)
	}
	// 环境变量还在时，页面上保存的值不生效。
	if got := env.WithPanel(map[string]string{"listen": "127.0.0.1:9000"}); got.Listen != "0.0.0.0:18443" {
		t.Errorf("环境变量应优先，得到 %s", got.Listen)
	}
}
