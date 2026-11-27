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

// 内置的本机：默认就在清单里，能改标签和敏感度，连接方式只能是直连或经 SSH 连回本机，也删不掉。
func TestBuiltinLocalHost(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("这台机器没有 /bin/sh")
	}
	dir := t.TempDir()
	cfg := Config{DataDir: dir, ConfigDir: dir, Listen: "127.0.0.1:8693", Dev: true}
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

	// 想把它改成指向别的机器的 SSH 主机：不行，它始终是这台机器。
	edit := inventory.Host{Name: inventory.LocalName, Addr: "192.0.2.9", Executor: "ssh", Credential: "ref:ssh/x",
		Tags: []string{"core"}, Sensitivity: risk.SensHigh}
	if err := a.SaveHost(edit, false); err == nil {
		t.Fatal("本机不应该能改成连到别的地址")
	}
	// 经 SSH 连回本机要选凭据。
	if err := a.SaveHost(inventory.Host{Name: inventory.LocalName, Addr: "127.0.0.1", Executor: "ssh"}, false); err == nil {
		t.Fatal("没选凭据不应该能改成 SSH")
	}
	// 经 SSH 连回本机：还是内置的那一台，删不掉，重启之后还在。
	edit.Addr = "127.0.0.1:2222"
	if err := a.SaveHost(edit, false); err != nil {
		t.Fatal(err)
	}
	viaSSH := func(a *App) {
		t.Helper()
		h, _ := a.Kernel.Inventory().Host(inventory.LocalName)
		if h.Source != "builtin" || h.Executor != "ssh" || h.Addr != "127.0.0.1:2222" || h.Credential != "ref:ssh/x" || h.Sensitivity != risk.SensHigh {
			t.Fatalf("没有改成经 SSH 连回本机：%+v", h)
		}
	}
	viaSSH(a)
	if err := a.DeleteHost(inventory.LocalName); err == nil {
		t.Fatal("内置本机不应该能删除")
	}
	a.Close()
	if a, err = Open(cfg, log); err != nil {
		t.Fatal(err)
	}
	viaSSH(a)
	// 改回本机直连：地址和凭据回到默认，标签和敏感度生效。
	edit.Executor = "local"
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
