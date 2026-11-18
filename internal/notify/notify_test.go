package notify

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/tsienwan/hostower/internal/i18n"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/metrics"
	"github.com/tsienwan/hostower/internal/secrets"
	"github.com/tsienwan/hostower/internal/store"
)

type captured struct {
	mu    sync.Mutex
	paths []string
	body  []map[string]any
}

func (c *captured) handler(reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		c.mu.Lock()
		c.paths = append(c.paths, r.URL.Path)
		c.body = append(c.body, m)
		c.mu.Unlock()
		io.WriteString(w, reply)
	}
}

func (c *captured) count() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.body) }

func newHub(t *testing.T) *Hub {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := make([]byte, 32)
	rand.Read(key)
	h, err := New(db, secrets.New(db, key), slog.New(slog.NewTextHandler(io.Discard, nil)), func() string { return "https://hostower.example.com" })
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestValidation(t *testing.T) {
	h := newHub(t)
	bad := []struct {
		c   Channel
		sec Secret
	}{
		{Channel{Name: "tg", Kind: "telegram", Telegram: &TelegramConf{ChatID: "12345"}}, Secret{Token: "not-a-token"}},
		{Channel{Name: "tg", Kind: "telegram", Telegram: &TelegramConf{ChatID: "abc"}}, Secret{Token: "123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}},
		// Webhook 只能是官方地址：不能借通知功能去访问内网。
		{Channel{Name: "fs", Kind: "feishu"}, Secret{Webhook: "http://192.0.2.1/hook"}},
		{Channel{Name: "dc", Kind: "discord"}, Secret{Webhook: "https://discord.example.com/api/webhooks/1/x"}},
		{Channel{Name: "mail", Kind: "email", Email: &EmailConf{Host: "smtp.example.com", Port: 25, Security: "none", Username: "u", From: "a@example.com", To: []string{"b@example.com"}}}, Secret{Password: "p"}},
		{Channel{Name: "mail", Kind: "email", Email: &EmailConf{Host: "smtp.example.com", Port: 465, Security: "tls", From: "a@example.com", To: []string{"not-an-address"}}}, Secret{}},
		{Channel{Name: "", Kind: "discord"}, Secret{Webhook: "https://discord.com/api/webhooks/123456/abcdefghijklmnopqrstuvwxyz"}},
	}
	for i, b := range bad {
		if _, err := h.Save(b.c, b.sec); err == nil {
			t.Errorf("第 %d 个应该被拒绝：%+v", i, b.c)
		}
	}
	c, err := h.Save(Channel{Name: "ops", Kind: "discord", Enabled: true, Events: []string{"host.offline", "nonsense"}},
		Secret{Webhook: "https://discord.com/api/webhooks/123456/abcdefghijklmnopqrstuvwxyz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Events) != 1 || c.Events[0] != "host.offline" {
		t.Fatalf("不认识的事件应该被去掉：%v", c.Events)
	}
	// 修改时不填机密就沿用原来的；列表里看不到机密本身。
	c.Name = "ops2"
	if _, err := h.Save(*c, Secret{}); err != nil {
		t.Fatal(err)
	}
	list, _ := h.Channels()
	if len(list) != 1 || list[0].Name != "ops2" || !list[0].HasSecret {
		t.Fatalf("列表不对：%+v", list)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "abcdefghijklmnopqrstuvwxyz") {
		t.Fatal("列表里不应该出现 Webhook 地址")
	}
	if err := h.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.secrets.Get(secretName(c.ID)); err == nil {
		t.Fatal("删除渠道后机密也应该删掉")
	}
}

// 用本地的假服务走一遍三种 Webhook 类的渠道：请求的形状、不提醒任何人、飞书签名。
func TestSendWebhooks(t *testing.T) {
	var tg, fs, dc captured
	mux := http.NewServeMux()
	mux.HandleFunc("/bot123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/sendMessage", tg.handler(`{"ok":true}`))
	mux.HandleFunc("/feishu", fs.handler(`{"code":0}`))
	mux.HandleFunc("/discord", dc.handler(``))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	telegramAPI = srv.URL
	allowTestURL = func(u string) bool { return strings.HasPrefix(u, srv.URL) }
	defer func() { telegramAPI, allowTestURL = "https://api.telegram.org", nil }()

	h := newHub(t)
	all := []string{"approval.pending", "host.offline"}
	for _, c := range []struct {
		ch  Channel
		sec Secret
	}{
		{Channel{Name: "tg", Kind: "telegram", Enabled: true, Events: all, Telegram: &TelegramConf{ChatID: "-1001234"}}, Secret{Token: "123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}},
		{Channel{Name: "fs", Kind: "feishu", Enabled: true, Events: all}, Secret{Webhook: srv.URL + "/feishu", Sign: "s3cret"}},
		{Channel{Name: "dc", Kind: "discord", Enabled: true, Lang: "en", Events: all}, Secret{Webhook: srv.URL + "/discord"}},
	} {
		if _, err := h.Save(c.ch, c.sec); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.worker(ctx)

	r := &kernel.Request{ID: "rq_1", Kind: "command", Intent: "重启 nginx @everyone", Level: risk.L1, State: kernel.StatePending,
		Actor: kernel.Actor{Name: "owner"}, Targets: []*kernel.Target{{Host: "web-1", Command: "systemctl restart nginx"}}}
	h.onRequest(r)
	h.onRequest(r) // 同一个状态再来一次：不重复通知
	waitFor(t, func() bool { return tg.count() == 1 && fs.count() == 1 && dc.count() == 1 })

	if tg.body[0]["chat_id"] != "-1001234" || !strings.Contains(tg.body[0]["text"].(string), "web-1") {
		t.Errorf("Telegram 请求不对：%v", tg.body[0])
	}
	if !strings.Contains(tg.body[0]["text"].(string), "https://hostower.example.com/requests/rq_1") {
		t.Errorf("通知里应该带回到页面的链接：%v", tg.body[0]["text"])
	}
	if fs.body[0]["sign"] == nil || fs.body[0]["timestamp"] == nil || fs.body[0]["msg_type"] != "post" {
		t.Errorf("飞书请求不对：%v", fs.body[0])
	}
	am, _ := dc.body[0]["allowed_mentions"].(map[string]any)
	if am == nil || len(am["parse"].([]any)) != 0 {
		t.Errorf("Discord 不应该允许任何提及：%v", dc.body[0])
	}
	embed := dc.body[0]["embeds"].([]any)[0].(map[string]any)
	if !strings.Contains(embed["title"].(string), "waiting for approval") {
		t.Errorf("英文渠道应该用英文标题：%v", embed["title"])
	}

	// 只订阅了 host.offline 的渠道不会收到别的；主机掉线两分钟以内不通知。
	h.onHosts(map[string]metrics.Status{"web-1": {Known: true, Online: true}})
	h.onHosts(map[string]metrics.Status{"web-1": {Known: true, Online: false, Error: "连接超时"}})
	h.checkOffline()
	time.Sleep(200 * time.Millisecond)
	if tg.count() != 1 {
		t.Fatal("离线不到两分钟不应该通知")
	}
	h.mu.Lock()
	h.hostState["web-1"].since = time.Now().Add(-3 * time.Minute)
	h.mu.Unlock()
	h.checkOffline()
	waitFor(t, func() bool { return tg.count() == 2 })
	if !strings.Contains(tg.body[1]["text"].(string), "连接超时") {
		t.Errorf("离线通知里应该有原因：%v", tg.body[1]["text"])
	}
}

// 假的 SMTP 服务：只够走完一次发信流程，用来检查邮件的内容。
func TestSendEmail(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rd := bufio.NewReader(conn)
		say := func(s string) { io.WriteString(conn, s+"\r\n") }
		say("220 test")
		var data strings.Builder
		inData := false
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					say("250 ok")
					got <- data.String()
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250 test")
			case cmd == "DATA":
				inData = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	c := &Channel{Name: "mail", Kind: "email", Email: &EmailConf{Host: "127.0.0.1", Port: port, Security: "none", From: "hostower@example.com", To: []string{"ops@example.com"}}}
	m := Message{Event: "host.offline", Severity: Crit, Time: time.Now(), Link: "/hosts/web-1",
		Title: i18n.Tx("主机离线：{host}", "host", "web-1"), Fields: []Field{f(i18n.Tx("主机"), "web-1")}}
	if err := sendEmail(context.Background(), Secret{}, c, m, "https://hostower.example.com"); err != nil {
		t.Fatal(err)
	}
	mail := <-got
	if !strings.Contains(mail, "Subject: =?utf-8?") || !strings.Contains(mail, "To: ops@example.com") {
		t.Fatalf("邮件头不对：%s", mail)
	}
	body := mail[strings.Index(mail, "\r\n\r\n")+4:]
	dec, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.TrimSpace(body), "\r\n", ""))
	if !strings.Contains(string(dec), "web-1") || !strings.Contains(string(dec), "https://hostower.example.com/hosts/web-1") {
		t.Fatalf("邮件正文不对：%s", dec)
	}
}

func TestFromAudit(t *testing.T) {
	m, ok := fromAudit(audit.Entry{Type: "auth.login.failed", Message: "Passkey 不对", Detail: map[string]any{"ip": "192.0.2.7"}}, 9)
	if !ok || m.Event != "auth.failed" || !strings.Contains(fieldsKey(m), "192.0.2.7") {
		t.Fatalf("登录失败应该变成通知：%+v", m)
	}
	if _, ok := fromAudit(audit.Entry{Type: "file.read"}, 1); ok {
		t.Fatal("普通的审计记录不应该变成通知")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("等待超时")
}

// 机器人客户端：长轮询、发消息；冲突给出能看懂的错误，错误里不出现令牌。
func TestTelegramBot(t *testing.T) {
	const token = "123456789:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	var got captured
	conflict := false
	mux := http.NewServeMux()
	mux.HandleFunc("/bot"+token+"/getMe", got.handler(`{"ok":true,"result":{"username":"hostower_bot"}}`))
	mux.HandleFunc("/bot"+token+"/sendMessage", got.handler(`{"ok":true,"result":{}}`))
	mux.HandleFunc("/bot"+token+"/getUpdates", func(w http.ResponseWriter, r *http.Request) {
		if conflict {
			w.WriteHeader(409)
			io.WriteString(w, `{"ok":false,"description":"Conflict: terminated by other getUpdates request"}`)
			return
		}
		io.WriteString(w, `{"ok":true,"result":[{"update_id":7,"message":{"date":1,"text":"/status","from":{"id":42},"chat":{"id":42,"type":"private"}}}]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	telegramAPI = srv.URL
	defer func() { telegramAPI = "https://api.telegram.org" }()

	h := newHub(t)
	c, err := h.Save(Channel{Name: "tg", Kind: "telegram", Enabled: true, Telegram: &TelegramConf{ChatID: "42"}}, Secret{Token: token})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.TelegramBot("nope"); err == nil {
		t.Fatal("不存在的渠道应该报错")
	}
	b, err := h.TelegramBot(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if name, err := b.Me(ctx); err != nil || name != "hostower_bot" {
		t.Fatalf("getMe 不对：%q %v", name, err)
	}
	ups, err := b.Updates(ctx, 0, 1)
	if err != nil || len(ups) != 1 || ups[0].Message.From.ID != 42 || ups[0].Message.Text != "/status" {
		t.Fatalf("getUpdates 不对：%+v %v", ups, err)
	}
	if err := b.Send(ctx, 42, "hi"); err != nil || got.body[len(got.body)-1]["disable_web_page_preview"] != true {
		t.Fatalf("sendMessage 不对：%v", err)
	}
	conflict = true
	_, err = b.Updates(ctx, 0, 1)
	if err == nil || !strings.Contains(err.Error(), "别的程序") || strings.Contains(err.Error(), token) {
		t.Fatalf("冲突的错误不对：%v", err)
	}
}
