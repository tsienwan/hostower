package kernel_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

func TestTerminal(t *testing.T) {
	k, verify := setup(t)
	ctx := context.Background()

	// 只有人能开终端。
	if _, err := k.SubmitTerminal(ctx, ai, "web-1"); !errors.Is(err, kernel.ErrHumanOnly) {
		t.Fatalf("AI 不应能开终端: %v", err)
	}
	r, err := k.SubmitTerminal(ctx, owner, "web-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Level != risk.L2 || r.State != kernel.StatePending {
		t.Fatalf("开终端应是等待批准的 L2: %s %s", r.Level, r.State)
	}
	// 没批准不能连。
	if _, err := k.AttachTerminal(ctx, r.ID, owner, 80, 24); err == nil {
		t.Fatal("没批准的终端不应能连")
	}
	// 批准要二次验证和主机名。
	if _, err := k.Approve(ctx, r.ID, owner, kernel.Proof{Digest: r.Digest, Typed: "web-1"}); !errors.Is(err, kernel.ErrReauth) {
		t.Fatalf("没有二次验证应被拒: %v", err)
	}
	if _, err := k.Approve(ctx, r.ID, owner, kernel.Proof{Digest: r.Digest, Reauthed: true, Typed: "web-1"}); err != nil {
		t.Fatal(err)
	}
	// 别人不能连我的终端。
	other := owner
	other.ID = "u2"
	if _, err := k.AttachTerminal(ctx, r.ID, other, 80, 24); !errors.Is(err, kernel.ErrNotAllowed) {
		t.Fatalf("别人不应能连: %v", err)
	}
	term, err := k.AttachTerminal(ctx, r.ID, owner, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	// 只能连一次。
	if _, err := k.AttachTerminal(ctx, r.ID, owner, 80, 24); err == nil {
		t.Fatal("同一个终端请求不应能连两次")
	}
	term.Write([]byte("hostname\r"))
	var out strings.Builder
	buf := make([]byte, 4096)
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(out.String(), "\r\nweb-1\r\n") && time.Now().Before(deadline) {
		n, err := term.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if !strings.Contains(out.String(), "\r\nweb-1\r\n") {
		t.Fatalf("终端输出不对: %q", out.String())
	}
	term.Close()
	term.Close() // 重复关闭不出错
	got, _ := k.Get(r.ID)
	if got.State != kernel.StateDone || got.Terminal == nil || got.Terminal.ClosedAt == nil || got.Terminal.Bytes == 0 {
		t.Fatalf("关闭后应记为完成: %s %+v", got.State, got.Terminal)
	}
	p, err := k.RecordingPath(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := os.ReadFile(p)
	if err != nil || !strings.Contains(string(rec), `"version":2`) || !strings.Contains(string(rec), "web-1") {
		t.Fatalf("录像应记下输出: %v %q", err, rec)
	}
	if res := verify(); !res.OK {
		t.Fatalf("审计链: %+v", res)
	}

	// 暂停会断开正在用的终端。
	r2, _ := k.SubmitTerminal(ctx, owner, "web-2")
	k.Approve(ctx, r2.ID, owner, kernel.Proof{Digest: r2.Digest, Reauthed: true, Typed: "web-2"})
	term2, err := k.AttachTerminal(ctx, r2.ID, owner, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	k.SetPaused(ctx, owner, true)
	done := make(chan struct{})
	go func() {
		for {
			if _, err := term2.Read(buf); err != nil {
				close(done)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("暂停后终端应被断开")
	}
	if _, err := k.SubmitTerminal(ctx, owner, "web-1"); !errors.Is(err, kernel.ErrPaused) {
		t.Fatalf("暂停后不应能开终端: %v", err)
	}
}
