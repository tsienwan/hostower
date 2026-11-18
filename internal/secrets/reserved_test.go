package secrets

import "testing"

func TestReservedNames(t *testing.T) {
	for _, n := range []string{"auth/totp/u_1", "oidc/client", "notify/c1"} {
		if !Reserved(n) {
			t.Fatalf("%s 应该是系统保留的", n)
		}
		if _, err := RefName("ref:" + n); err == nil {
			t.Fatalf("主机凭据不能引用 %s", n)
		}
	}
	if Reserved("ssh/web-1") {
		t.Fatal("普通凭据名不该被保留")
	}
	if name, err := RefName("ref:ssh/web-1"); err != nil || name != "ssh/web-1" {
		t.Fatal("普通引用应该能用", err)
	}
}
