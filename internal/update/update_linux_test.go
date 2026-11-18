//go:build linux

package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// 用一个假的发布页走完整个流程：检查 → 下载 → 验签 → 核对版本 → 就位。
// “新版本”是一段只会打印版本号的脚本。
func TestApply(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, wrong, _ := ed25519.GenerateKey(rand.Reader)
	key := base64.StdEncoding.EncodeToString(pub)
	script := func(v string) []byte { return []byte("#!/bin/sh\necho hostower " + v + " schema 5\n") }

	var tag string
	var bin []byte
	var sig string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/hostower/releases/latest":
			fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.com/r","assets":[{"name":%q,"browser_download_url":%q},{"name":%q,"browser_download_url":%q}]}`,
				tag, AssetName(), srv.URL+"/bin", AssetName()+".sig", srv.URL+"/sig")
		case "/bin":
			w.Write(bin)
		case "/sig":
			w.Write([]byte(sig))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	run := func() (*Updater, string, string, error) {
		dir := t.TempDir()
		restarted := ""
		u := New(Config{Version: "v1.0.0", Repo: "owner/hostower", PubKey: key, API: srv.URL, DataDir: dir,
			Restart: func(p string) { restarted = p }})
		if !u.Status().Supported {
			t.Fatalf("应该支持更新：%s", u.Status().Reason)
		}
		return u, dir, restarted, u.Apply(context.Background())
	}

	// 正常的新版本：就位并请求重启。
	tag, bin = "v1.1.0", script("v1.1.0")
	sig = Sign(priv, bin)
	u, dir, _, err := run()
	if err != nil {
		t.Fatal(err)
	}
	if st := u.Status(); st.State != "restarting" || st.Latest != "v1.1.0" || !st.Newer {
		t.Fatalf("状态不对：%+v", st)
	}
	if p, v, ok := Staged(dir, "v1.0.0", key); !ok || v != "v1.1.0" || p == "" {
		t.Fatalf("就位的版本不对：%s %s %v", p, v, ok)
	}
	// 上次切换没成功（标记还在）：放弃这份更新。
	MarkPending(dir)
	if _, _, ok := Staged(dir, "v1.0.0", key); ok {
		t.Fatal("切换失败过的版本不应该再接力")
	}
	if _, err := os.Stat(stagedPath(dir)); err == nil {
		t.Fatal("失败的版本应该被删掉")
	}

	// 没有更新：什么都不做。
	tag, bin = "v1.0.0", script("v1.0.0")
	sig = Sign(priv, bin)
	if u, dir, _, err = run(); err != nil || u.Status().State != "idle" {
		t.Fatalf("没有新版本时不应该动：%v %+v", err, u.Status())
	}
	if _, _, ok := Staged(dir, "v1.0.0", key); ok {
		t.Fatal("不应该有就位的版本")
	}

	// 别人签的：拒绝。
	tag, bin = "v1.2.0", script("v1.2.0")
	sig = Sign(wrong, bin)
	if u, dir, _, err = run(); err == nil {
		t.Fatal("签名不对应该失败")
	}
	if _, _, ok := Staged(dir, "v1.0.0", key); ok || u.Status().Error == "" {
		t.Fatal("签名不对时不应该留下任何东西")
	}

	// 签名是真的，但文件其实是旧版本（拿旧版本冒充新版本）：拒绝。
	tag, bin = "v1.3.0", script("v0.9.0")
	sig = Sign(priv, bin)
	if _, dir, _, err = run(); err == nil {
		t.Fatal("版本对不上应该失败")
	}
	if _, _, ok := Staged(dir, "v1.0.0", key); ok {
		t.Fatal("版本对不上时不应该留下任何东西")
	}
}
