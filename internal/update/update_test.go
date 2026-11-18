package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func TestVersions(t *testing.T) {
	for _, v := range []string{"v1.2.3", "v0.0.1", "v10.20.30"} {
		if !validVersion(v) {
			t.Errorf("%s 应该是合法版本", v)
		}
	}
	for _, v := range []string{"dev", "local", "1.2.3", "v1.2", "v1.2.3-rc1", "v01.2.3", "v1.2.x", ""} {
		if validVersion(v) {
			t.Errorf("%s 不应该是合法版本", v)
		}
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"v1.2.4", "v1.2.3", true}, {"v1.10.0", "v1.9.9", true}, {"v2.0.0", "v1.99.99", true},
		{"v1.2.3", "v1.2.3", false}, {"v1.2.2", "v1.2.3", false}, {"v1.2.3", "dev", false}, {"dev", "v1.2.3", false},
	}
	for _, c := range cases {
		if newer(c.a, c.b) != c.want {
			t.Errorf("newer(%s, %s) 应该是 %v", c.a, c.b, c.want)
		}
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	bin := []byte("binary-content")
	sig := []byte(Sign(priv, bin))
	if err := Verify(pub, bin, sig); err != nil {
		t.Fatal(err)
	}
	if Verify(pub, []byte("binary-content!"), sig) == nil {
		t.Error("内容被改过应该验不过")
	}
	if Verify(other, bin, sig) == nil {
		t.Error("换一把公钥应该验不过")
	}
	if Verify(pub, bin, []byte("not-base64!!")) == nil {
		t.Error("乱写的签名应该验不过")
	}
	// 直接对文件内容签名（没有用途前缀）的签名不能通过：防止别处的签名被拿来冒充。
	raw := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, bin))
	if Verify(pub, bin, []byte(raw)) == nil {
		t.Error("没有用途前缀的签名应该验不过")
	}
}

func TestUnsupportedBuild(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	key := base64.StdEncoding.EncodeToString(pub)
	cases := []Config{
		{Version: "dev", Repo: "owner/hostower", PubKey: key},
		{Version: "v1.0.0", Repo: "", PubKey: key},
		{Version: "v1.0.0", Repo: "owner/hostower", PubKey: "bad"},
		{Version: "v1.0.0", Repo: "owner/hostower", PubKey: key, API: "http://updates.example.com"},
	}
	for _, c := range cases {
		u := New(c)
		if u.Status().Supported {
			t.Errorf("%+v 不应该支持更新", c)
		}
		if u.SetAuto(true) == nil {
			t.Errorf("%+v 不应该能打开自动更新", c)
		}
	}
}
