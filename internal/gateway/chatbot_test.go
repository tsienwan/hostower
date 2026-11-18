package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/notify"
	"github.com/tsienwan/hostower/internal/store"
)

type fakeChat struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeChat) Updates(context.Context, int64, int) ([]notify.Update, error) { return nil, nil }
func (f *fakeChat) Me(context.Context) (string, error)                           { return "hostower_test_bot", nil }
func (f *fakeChat) SetCommands(context.Context, []notify.BotCommand, string) error {
	return nil
}
func (f *fakeChat) Send(_ context.Context, _ int64, text string) error {
	f.mu.Lock()
	f.sent = append(f.sent, text)
	f.mu.Unlock()
	return nil
}

// take 返回并清空收到的回复。
func (f *fakeChat) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.sent
	f.sent = nil
	return out
}

const chatInventory = `
apiVersion: hostower/v1
kind: Inventory
hosts:
  web-1: {addr: 192.0.2.10, executor: sim}
`

func chatSetup(t *testing.T) (*Server, *fakeChat) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv, err := inventory.Parse([]byte(chatInventory))
	if err != nil {
		t.Fatal(err)
	}
	k, err := kernel.New(kernel.Config{DataDir: dir}, db, risk.New(), audit.Open(db, nil), executor.Registry{"sim": executor.NewSim()}, inv)
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := auth.New(db, "http://localhost:18443", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO users (id, name, role, created_at) VALUES ('u1', 'owner', 'owner', '2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{PublicURL: "https://hostower.example.com", Kernel: k, Auth: authSvc, DB: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	s.bot.cfg = chatConfig{Enabled: true, Channel: "c1", TTLDays: 30}
	return s, &fakeChat{}
}

func msg(from int64, chatType, text string, age time.Duration) notify.Update {
	raw := fmt.Sprintf(`{"update_id":1,"message":{"date":%d,"text":%q,"from":{"id":%d,"username":"ops%d"},"chat":{"id":%d,"type":%q}}}`,
		time.Now().Add(-age).Unix(), text, from, from, from, chatType)
	var u notify.Update
	json.Unmarshal([]byte(raw), &u)
	return u
}

const owner, stranger = int64(1001), int64(2002)

func say(s *Server, f *fakeChat, from int64, text string) []string {
	s.bot.handle(context.Background(), f, "c1", msg(from, "private", text, 0))
	return f.take()
}

func pair(s *Server, code string) {
	s.bot.mu.Lock()
	s.bot.pair = &chatPairing{code: code, userID: "u1", userName: "owner", expires: time.Now().Add(pairTTL)}
	s.bot.mu.Unlock()
}

func TestChatBinding(t *testing.T) {
	s, f := chatSetup(t)
	// 没有配对码时，谁发什么都不回。
	if out := say(s, f, stranger, "/bind ABCDEFGH"); len(out) != 0 {
		t.Fatalf("没有配对码时不应该回复：%v", out)
	}
	if out := say(s, f, stranger, "/status"); len(out) != 0 {
		t.Fatalf("陌生人不应该得到回复：%v", out)
	}
	pair(s, "ABCDEFGH")
	// 错 5 次作废。
	for i := 1; i <= pairMaxTries; i++ {
		out := say(s, f, stranger, "/bind WRONG000")
		if len(out) != 1 {
			t.Fatalf("第 %d 次错的配对码应该得到提示", i)
		}
	}
	if out := say(s, f, owner, "/bind ABCDEFGH"); len(out) != 0 || s.bot.cfg.Binding != nil {
		t.Fatalf("错太多次之后配对码应该作废：%v", out)
	}
	// 群里发的配对码不算。
	pair(s, "ABCDEFGH")
	s.bot.handle(context.Background(), f, "c1", msg(owner, "group", "/bind ABCDEFGH", 0))
	if s.bot.cfg.Binding != nil || len(f.take()) != 0 {
		t.Fatal("群组里的消息应该被忽略")
	}
	// 太早的消息不处理。
	s.bot.handle(context.Background(), f, "c1", msg(owner, "private", "/bind ABCDEFGH", 3*time.Minute))
	if s.bot.cfg.Binding != nil {
		t.Fatal("积压的旧消息不应该被处理")
	}
	if out := say(s, f, owner, "/bind abcd efgh"); len(out) != 1 || !strings.Contains(out[0], "已绑定") {
		t.Fatalf("配对应该成功：%v", out)
	}
	if b := s.bot.cfg.Binding; b == nil || b.ChatUserID != owner || b.UserID != "u1" {
		t.Fatalf("绑定不对：%+v", b)
	}
	if s.bot.pair != nil {
		t.Fatal("配对码用过一次就应该作废")
	}
	// 绑定之后，别人发命令依然没有回应。
	if out := say(s, f, stranger, "/pause"); len(out) != 0 || s.Kernel.Paused() {
		t.Fatal("陌生人的命令不应该生效")
	}
	if out := say(s, f, owner, "/status"); len(out) != 1 || !strings.Contains(out[0], "主机") {
		t.Fatalf("/status 不对：%v", out)
	}
	// 过期之后要回网页重新配对。
	s.bot.now = func() time.Time { return time.Now().Add(31 * 24 * time.Hour) }
	s.bot.handle(context.Background(), f, "c1", msg(owner, "private", "/status", -31*24*time.Hour))
	s.bot.now = time.Now
	if s.bot.cfg.Binding != nil {
		t.Fatal("过期的绑定应该被清掉")
	}
	if out := f.take(); len(out) != 1 || !strings.Contains(out[0], "过期") {
		t.Fatalf("应该提示绑定已过期：%v", out)
	}
}

func bind(s *Server) {
	s.bot.cfg.Binding = &chatBinding{ChatUserID: owner, ChatName: "@ops", UserID: "u1", UserName: "owner", BoundAt: time.Now(), ExpiresAt: bindingExpiry(time.Now().Add(-23*time.Hour), 1)}
}

func TestChatRun(t *testing.T) {
	s, f := chatSetup(t)
	bind(s)
	// 只读命令直接执行，结果带回来。
	out := say(s, f, owner, "/run web-1 df -h")
	if len(out) != 1 || !strings.Contains(out[0], "[web-1] ok") || !strings.Contains(out[0], "https://hostower.example.com/requests/") {
		t.Fatalf("L0 应该直接执行并带回结果：%v", out)
	}
	// 变更要到网页上批准，聊天里的身份批准不了。
	out = say(s, f, owner, "/run web-1 systemctl restart nginx")
	if len(out) != 1 || !strings.Contains(out[0], "等待批准") {
		t.Fatalf("L1 应该等待批准：%v", out)
	}
	var pending *kernel.Request
	for _, r := range s.Kernel.List(10) {
		if r.State == kernel.StatePending {
			pending = r
		}
	}
	if pending == nil || pending.Actor.Entry != "chat" || pending.Actor.CanApprove {
		t.Fatalf("请求的发起人应该是聊天入口、不能批准：%+v", pending)
	}
	if _, err := s.Kernel.Approve(context.Background(), pending.ID, pending.Actor, kernel.Proof{Digest: pending.Digest}); !errors.Is(err, kernel.ErrNotAllowed) {
		t.Fatalf("聊天入口的身份不能批准：%v", err)
	}
	if out := say(s, f, owner, "/pending"); len(out) != 1 || !strings.Contains(out[0], pending.ID) {
		t.Fatalf("/pending 应该列出等待中的请求：%v", out)
	}
	if out := say(s, f, owner, "/run web-1"); len(out) != 1 || !strings.Contains(out[0], "用法") {
		t.Fatalf("缺参数应该提示用法：%v", out)
	}
	// 紧急暂停能做，恢复不能。
	say(s, f, owner, "/pause")
	if !s.Kernel.Paused() {
		t.Fatal("/pause 应该暂停全部执行")
	}
	if out := say(s, f, owner, "/resume"); len(out) != 1 || !strings.Contains(out[0], "不认识") || !s.Kernel.Paused() {
		t.Fatal("聊天里不能恢复执行")
	}
}

func TestChatRateLimit(t *testing.T) {
	s, f := chatSetup(t)
	bind(s)
	var replies []string
	for i := 0; i < chatPerMinute+5; i++ {
		replies = append(replies, say(s, f, owner, "/help")...)
	}
	if len(replies) != chatPerMinute+1 || !strings.Contains(replies[len(replies)-1], "太频繁") {
		t.Fatalf("超出限流后应该只提醒一次：收到 %d 条", len(replies))
	}
}

func TestChatOwnerGone(t *testing.T) {
	s, f := chatSetup(t)
	bind(s)
	s.DB.Exec("UPDATE users SET role = 'viewer' WHERE id = 'u1'")
	if out := say(s, f, owner, "/status"); len(out) != 1 || s.bot.cfg.Binding != nil {
		t.Fatalf("账号不再是所有者时应该解除绑定：%v", out)
	}
}
