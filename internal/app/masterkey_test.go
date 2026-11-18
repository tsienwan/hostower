package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// 自动生成的主密钥：第一次生成，之后每次读回同一把；文件坏了就报错，绝不悄悄换一把。
func TestAutoMasterKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "master_key")
	k1, err := autoMasterKey(p)
	if err != nil || len(k1) != 32 {
		t.Fatalf("第一次应该生成一把 32 字节的密钥：%v %d", err, len(k1))
	}
	k2, err := autoMasterKey(p)
	if err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("第二次应该读回同一把：%v", err)
	}
	if err := os.WriteFile(p, []byte("坏掉的内容\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := autoMasterKey(p); err == nil {
		t.Fatal("文件损坏时应该报错，而不是重新生成")
	}
	if raw, _ := os.ReadFile(p); string(raw) != "坏掉的内容\n" {
		t.Fatal("损坏的文件不应该被覆盖")
	}
}
