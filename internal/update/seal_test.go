package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSeal(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	key := base64.StdEncoding.EncodeToString(pub)
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bin := []byte("\x7fELF pretend this is a program")
	sealed := Seal(priv, bin)
	if got := CheckSeal(write("ok", sealed), key); got.Status != BuildOfficial || got.Fingerprint != Fingerprint(pub) {
		t.Fatalf("封好的文件应校验通过：%+v", got)
	}
	// 重复封签：去掉旧的再封，结果一样能通过，长度不变。
	if again := Seal(priv, sealed); len(again) != len(sealed) || CheckSeal(write("again", again), key).Status != BuildOfficial {
		t.Fatal("重复封签出错")
	}
	tampered := append([]byte{}, sealed...)
	tampered[3] ^= 1
	if got := CheckSeal(write("bad", tampered), key).Status; got != BuildInvalid {
		t.Fatalf("改过的文件应判为 invalid，得到 %s", got)
	}
	if got := CheckSeal(write("plain", bin), key).Status; got != BuildUnsealed {
		t.Fatalf("没有封签应判为 unsealed，得到 %s", got)
	}
	if got := CheckSeal(write("ok2", sealed), base64.StdEncoding.EncodeToString(other)).Status; got != BuildInvalid {
		t.Fatalf("换一把公钥应判为 invalid，得到 %s", got)
	}
	if got := CheckSeal(write("ok3", sealed), "").Status; got != BuildNoKey {
		t.Fatalf("没有公钥应判为 nokey，得到 %s", got)
	}
	// 封签和发布页上的 .sig 用的不是同一种消息：.sig 的签名拿来当封签用不了。
	d := Sign(priv, bin)
	raw, _ := base64.StdEncoding.DecodeString(d)
	fake := append(append(append([]byte{}, bin...), raw...), sealMagic...)
	if got := CheckSeal(write("fake", fake), key).Status; got != BuildInvalid {
		t.Fatalf("发布签名不应能冒充封签，得到 %s", got)
	}
}
