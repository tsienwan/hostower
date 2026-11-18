//go:build linux

package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/inventory"
)

func TestLocalExecAndFiles(t *testing.T) {
	l := NewLocal()
	if l == nil {
		t.Skip("这台机器没有 /bin/sh")
	}
	ctx := context.Background()
	h := inventory.Host{Name: "local"}

	var out bytes.Buffer
	code, err := l.Exec(ctx, h, "echo hi; echo err >&2; exit 3", &out)
	if err != nil || code != 3 || !strings.Contains(out.String(), "hi") || !strings.Contains(out.String(), "err") {
		t.Fatalf("code=%d err=%v out=%q", code, err, out.String())
	}

	out.Reset()
	code, err = l.ExecStdin(ctx, h, "cat", strings.NewReader("stdin-data"), &out)
	if err != nil || code != 0 || out.String() != "stdin-data" {
		t.Fatalf("code=%d err=%v out=%q", code, err, out.String())
	}

	// 取消要能结束整个进程组，不能等子进程自己退出。
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := l.Exec(cctx, h, "sleep 30 & sleep 30", io.Discard); err == nil {
		t.Fatal("超时后应该返回错误")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("取消花了 %v", time.Since(start))
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "a.conf")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.WriteFile(ctx, h, p, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, err := l.ReadFile(ctx, h, p, 0)
	if err != nil || string(got) != "new" {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("权限位变成了 %v", st.Mode().Perm())
	}
	if _, err := l.ReadFile(ctx, h, filepath.Join(dir, "none"), 0); err != ErrNotExist {
		t.Fatalf("err=%v", err)
	}
	list, err := l.ListDir(ctx, h, dir)
	if err != nil || len(list) != 1 || list[0].Name != "a.conf" {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestLocalShell(t *testing.T) {
	l := NewLocal()
	if l == nil {
		t.Skip("这台机器没有 /bin/sh")
	}
	if _, err := os.Stat("/dev/ptmx"); err != nil {
		t.Skip("没有伪终端")
	}
	sess, err := l.Shell(context.Background(), inventory.Host{Name: "local"}, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.Write([]byte("echo marker-$((40+2)); exit\n")); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(sess)
		done <- string(b)
	}()
	select {
	case out := <-done:
		if !strings.Contains(out, "marker-42") {
			t.Fatalf("终端输出里没有结果：%q", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("shell 退出后读取没有结束")
	}
}

// 宿主机模式下，指向绝对路径的符号链接要按宿主机的根目录解析，不能落到容器自己的文件系统里。
func TestLocalHostPath(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "run/real"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	must(os.WriteFile(filepath.Join(root, "run/real/resolv.conf"), []byte("host"), 0o644))
	must(os.Symlink("/run/real/resolv.conf", filepath.Join(root, "etc/resolv.conf"))) // 绝对链接
	must(os.Symlink("../run", filepath.Join(root, "etc/up")))                         // 相对链接
	must(os.Symlink("/etc/loop", filepath.Join(root, "etc/loop")))                    // 死循环

	l := &Local{root: root}
	ctx := context.Background()
	h := inventory.Host{Name: "local"}
	for _, p := range []string{"/etc/resolv.conf", "/etc/up/real/resolv.conf", "/etc/../etc/resolv.conf"} {
		got, err := l.ReadFile(ctx, h, p, 0)
		if err != nil || string(got) != "host" {
			t.Fatalf("%s: got=%q err=%v", p, got, err)
		}
	}
	if _, err := l.ReadFile(ctx, h, "/etc/loop", 0); err == nil {
		t.Fatal("循环的符号链接应该报错")
	}
	if _, err := l.ReadFile(ctx, h, "etc/resolv.conf", 0); err == nil {
		t.Fatal("相对路径应该报错")
	}
	// 写文件落在链接指向的真实位置，链接本身保留。
	must(l.WriteFile(ctx, h, "/etc/resolv.conf", []byte("new")))
	if b, _ := os.ReadFile(filepath.Join(root, "run/real/resolv.conf")); string(b) != "new" {
		t.Fatalf("写到了别处：%q", b)
	}
	if st, err := os.Lstat(filepath.Join(root, "etc/resolv.conf")); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("符号链接被替换掉了")
	}
	if _, err := l.ReadFile(ctx, h, "/etc/none", 0); err != ErrNotExist {
		t.Fatalf("err=%v", err)
	}
}
