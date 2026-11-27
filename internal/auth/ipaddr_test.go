package auth

import (
	"errors"
	"testing"

	"github.com/tsienwan/hostower/internal/store"
)

// 用 IP 地址访问时塔台照常启动，只是用不了 Passkey；用域名访问时 Passkey 可用。
func TestPublicURLWithIP(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, u := range []string{"https://192.0.2.10:8693", "https://[2001:db8::1]:8693"} {
		s, err := New(db, u, false)
		if err != nil {
			t.Fatalf("%s：用 IP 地址访问也应该能启动：%v", u, err)
		}
		if s.PasskeyAvailable() {
			t.Fatalf("%s：IP 地址上不该启用 Passkey", u)
		}
		if _, _, err := s.LoginBegin(); !errors.Is(err, ErrNoPasskey) {
			t.Fatalf("%s：应该说明用不了 Passkey：%v", u, err)
		}
	}
	s, err := New(db, "https://hostower.example.com", false)
	if err != nil || !s.PasskeyAvailable() {
		t.Fatalf("域名上应该能用 Passkey：%v", err)
	}
}
