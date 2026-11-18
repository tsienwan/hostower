//go:build linux

package app

import (
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 内置的本机：默认就在清单里，能改标签和敏感度，改不了连接方式，也删不掉。
func TestBuiltinLocalHost(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("这台机器没有 /bin/sh")
	}
	dir := t.TempDir()
	cfg := Config{DataDir: dir, ConfigDir: dir, Listen: "127.0.0.1:18443", Dev: true}
	cfg.finalize()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := Open(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := a.Kernel.Inventory().Host(inventory.LocalName)
	if !ok || h.Source != "builtin" || h.Executor != "local" || h.Sensitivity != risk.SensNormal {
		t.Fatalf("内置本机不对：%+v ok=%v", h, ok)
	}

	// 想把它改成 SSH 主机：连接方式保持不变，标签和敏感度生效。
	edit := inventory.Host{Name: inventory.LocalName, Addr: "192.0.2.9", Executor: "ssh", Credential: "ref:ssh/x",
		Tags: []string{"core"}, Sensitivity: risk.SensHigh}
	if err := a.SaveHost(edit, false); err != nil {
		t.Fatal(err)
	}
	check := func(a *App) {
		t.Helper()
		h, _ := a.Kernel.Inventory().Host(inventory.LocalName)
		if h.Source != "builtin" || h.Executor != "local" || h.Addr != "localhost" || h.Credential != "" {
			t.Fatalf("连接方式被改了：%+v", h)
		}
		if h.Sensitivity != risk.SensHigh || len(h.Tags) != 1 || h.Tags[0] != "core" {
			t.Fatalf("修改没有生效：%+v", h)
		}
	}
	check(a)
	if err := a.DeleteHost(inventory.LocalName); err == nil {
		t.Fatal("内置本机不应该能删除")
	}
	if err := a.SaveHost(inventory.Host{Name: inventory.LocalName, Addr: "192.0.2.9", Executor: "sim"}, true); err == nil {
		t.Fatal("不应该能新建同名主机")
	}
	a.Close()

	// 重启之后修改还在。
	a, err = Open(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	check(a)
}
