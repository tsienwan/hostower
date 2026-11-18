// Package notify 把 Hostower 里发生的事推送到外面：Telegram 机器人、飞书机器人、Discord、邮件。
//
// 它在内核之外，只是一个“听众”：从内核的事件流里听请求和主机状态的变化，从审计的外部副本接口里听安全相关的记录。
// 它不能发起任何操作，也不能批准任何请求——通知里只有一个回到页面的链接。发送失败只记日志，绝不影响内核。
//
// 渠道的配置存在数据库里；令牌、Webhook 地址、邮箱密码这些存在凭据存储里（notify/<编号>），只写不读。
package notify

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tsienwan/hostower/internal/i18n"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/metrics"
	"github.com/tsienwan/hostower/internal/secrets"
)

// Channel 是一个通知渠道。凭据不在这里，只有“有没有设置”的标记。
type Channel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // telegram | feishu | discord | email
	Enabled bool     `json:"enabled"`
	Lang    string   `json:"lang"` // 语言代码，见 web/src/locales/languages.json
	Events  []string `json:"events"`
	// Hosts 为空表示所有主机；否则只推送和这些主机有关的通知（与主机无关的通知照常推送）。
	Hosts    []string      `json:"hosts,omitempty"`
	Telegram *TelegramConf `json:"telegram,omitempty"`
	Email    *EmailConf    `json:"email,omitempty"`
	// HasSecret 报告凭据存储里有没有这个渠道的令牌、地址或密码。
	HasSecret bool `json:"hasSecret"`
}

// TelegramConf 是 Telegram 渠道里不算机密的部分。
type TelegramConf struct {
	ChatID string `json:"chatId"`
}

// EmailConf 是邮件渠道里不算机密的部分。
type EmailConf struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Security string   `json:"security"` // tls | starttls | none
	Username string   `json:"username,omitempty"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

// Secret 是渠道的机密部分，只存在凭据存储里。
type Secret struct {
	Token    string `json:"token,omitempty"`    // Telegram 机器人令牌
	Webhook  string `json:"webhook,omitempty"`  // 飞书、Discord 的 Webhook 地址（地址本身就是凭据）
	Sign     string `json:"sign,omitempty"`     // 飞书的签名密钥（可选）
	Password string `json:"password,omitempty"` // 邮箱密码或授权码
}

// Status 是一个渠道最近的发送情况（只在内存里）。
type Status struct {
	LastOK    int64  `json:"lastOk,omitempty"`
	LastError string `json:"lastError,omitempty"`
	LastErrAt int64  `json:"lastErrorAt,omitempty"`
	Sent      int    `json:"sent"`
	Dropped   int    `json:"dropped"`
}

// Hub 管理渠道并分发通知。
type Hub struct {
	db      *sql.DB
	secrets *secrets.Store
	log     *slog.Logger
	base    func() string

	mu       sync.Mutex
	channels []*Channel
	status   map[string]*Status
	sent     map[string][]time.Time // 渠道 → 最近一分钟的发送时间（限流）
	recent   map[string]time.Time   // 最近发过的通知（去重）

	queue chan Message

	// 状态跟踪
	reqState  map[string]kernel.State
	hostState map[string]*hostTrack
}

type hostTrack struct {
	online    bool
	since     time.Time
	announced bool   // 离线已经通知过
	lastErr   string // 离线的原因
}

const (
	perMinute   = 20              // 每个渠道每分钟最多发多少条
	dedupWindow = time.Minute     // 同样的通知一分钟内只发一次
	offlineFor  = 2 * time.Minute // 离线持续这么久才通知，免得网络抖一下就报警
	queueSize   = 256
)

// New 创建通知中心。base 返回页面的访问地址，用来拼通知里的链接。
func New(db *sql.DB, sec *secrets.Store, log *slog.Logger, base func() string) (*Hub, error) {
	h := &Hub{db: db, secrets: sec, log: log, base: base,
		status: map[string]*Status{}, sent: map[string][]time.Time{}, recent: map[string]time.Time{},
		queue: make(chan Message, queueSize), reqState: map[string]kernel.State{}, hostState: map[string]*hostTrack{}}
	return h, h.load()
}

func (h *Hub) load() error {
	rows, err := h.db.Query("SELECT body FROM notify_channels ORDER BY created_at")
	if err != nil {
		return err
	}
	defer rows.Close()
	var list []*Channel
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return err
		}
		var c Channel
		if json.Unmarshal([]byte(body), &c) == nil {
			list = append(list, &c)
		}
	}
	h.mu.Lock()
	h.channels = list
	h.mu.Unlock()
	return rows.Err()
}

// Channels 返回全部渠道和它们的发送情况。
func (h *Hub) Channels() ([]Channel, map[string]Status) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Channel, 0, len(h.channels))
	st := map[string]Status{}
	for _, c := range h.channels {
		cc := *c
		_, err := h.secrets.Get(secretName(c.ID))
		cc.HasSecret = err == nil
		out = append(out, cc)
		if s := h.status[c.ID]; s != nil {
			st[c.ID] = *s
		}
	}
	return out, st
}

func secretName(id string) string { return "notify/" + id }

// ---------- 保存与校验 ----------

// Save 新建（ID 为空）或修改一个渠道。sec 里为空的字段表示“保持原来的”。
func (h *Hub) Save(c Channel, sec Secret) (*Channel, error) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len([]rune(c.Name)) > 40 {
		return nil, errors.New("名称不能为空，最长 40 个字")
	}
	if !i18n.Known(c.Lang) {
		c.Lang = i18n.Source()
	}
	events := []string{}
	for _, e := range c.Events {
		if knownEvent(e) {
			events = append(events, e)
		}
	}
	sort.Strings(events)
	c.Events = events
	if !h.secrets.Available() {
		return nil, errors.New("没有配置主密钥，不能保存通知渠道的令牌和密码")
	}

	h.mu.Lock()
	var cur *Channel
	for _, x := range h.channels {
		if x.ID == c.ID {
			cur = x
		}
	}
	h.mu.Unlock()
	if c.ID != "" && cur == nil {
		return nil, errors.New("通知渠道不存在")
	}
	if cur != nil && cur.Kind != c.Kind {
		return nil, errors.New("不能修改渠道类型")
	}
	if c.ID == "" {
		b := make([]byte, 8)
		rand.Read(b)
		c.ID = hex.EncodeToString(b)
	}

	// 合并机密：空的字段沿用已保存的值。
	var old Secret
	if raw, err := h.secrets.Get(secretName(c.ID)); err == nil {
		json.Unmarshal(raw, &old)
	}
	if sec.Token == "" {
		sec.Token = old.Token
	}
	if sec.Webhook == "" {
		sec.Webhook = old.Webhook
	}
	if sec.Sign == "" {
		sec.Sign = old.Sign
	}
	if sec.Password == "" {
		sec.Password = old.Password
	}
	if err := validate(&c, &sec); err != nil {
		return nil, err
	}

	raw, _ := json.Marshal(sec)
	if err := h.secrets.Put(secretName(c.ID), raw); err != nil {
		return nil, err
	}
	c.HasSecret = false
	body, _ := json.Marshal(c)
	if _, err := h.db.Exec(`INSERT INTO notify_channels (id, body, created_at, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at`,
		c.ID, string(body), time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	if err := h.load(); err != nil {
		return nil, err
	}
	c.HasSecret = true
	return &c, nil
}

func validate(c *Channel, sec *Secret) error {
	switch c.Kind {
	case "telegram":
		c.Email = nil
		if c.Telegram == nil || !telegramChatRe.MatchString(strings.TrimSpace(c.Telegram.ChatID)) {
			return errors.New("Telegram 的会话编号不对：应该是一串数字（群组以 - 开头），或者 @频道名")
		}
		c.Telegram.ChatID = strings.TrimSpace(c.Telegram.ChatID)
		if !telegramTokenRe.MatchString(sec.Token) {
			return errors.New("Telegram 机器人令牌格式不对：应该形如 123456789:AA…（从 @BotFather 获取）")
		}
	case "feishu":
		c.Telegram, c.Email = nil, nil
		if !okURL(sec.Webhook, feishuHookRe) {
			return errors.New("飞书 Webhook 地址不对：应该以 https://open.feishu.cn/open-apis/bot/v2/hook/ 开头")
		}
	case "discord":
		c.Telegram, c.Email = nil, nil
		sec.Sign = ""
		if !okURL(sec.Webhook, discordHookRe) {
			return errors.New("Discord Webhook 地址不对：应该以 https://discord.com/api/webhooks/ 开头")
		}
	case "email":
		c.Telegram = nil
		e := c.Email
		if e == nil {
			return errors.New("缺少邮件服务器设置")
		}
		e.Host = strings.TrimSpace(e.Host)
		if !smtpHostRe.MatchString(e.Host) {
			return errors.New("邮件服务器地址不对")
		}
		if e.Port <= 0 || e.Port > 65535 {
			return errors.New("邮件服务器端口不对")
		}
		switch e.Security {
		case "tls", "starttls", "none":
		default:
			return errors.New("加密方式只能是 SSL/TLS、STARTTLS 或不加密")
		}
		if !emailRe.MatchString(e.From) {
			return errors.New("发件人地址不对")
		}
		var to []string
		for _, a := range e.To {
			if a = strings.TrimSpace(a); a == "" {
				continue
			}
			if !emailRe.MatchString(a) {
				return fmt.Errorf("收件人地址 %s 不对", a)
			}
			to = append(to, a)
		}
		if len(to) == 0 || len(to) > 20 {
			return errors.New("收件人要填 1 到 20 个")
		}
		e.To = to
		if e.Username != "" && sec.Password == "" {
			return errors.New("填了用户名就要填密码（或授权码）")
		}
		if e.Security == "none" && e.Username != "" {
			return errors.New("不加密的连接上不能发送密码，请选择 SSL/TLS 或 STARTTLS")
		}
	default:
		return errors.New("未知的通知渠道类型")
	}
	return nil
}

// Delete 删除一个渠道和它的机密。
func (h *Hub) Delete(id string) error {
	res, err := h.db.Exec("DELETE FROM notify_channels WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("通知渠道不存在")
	}
	h.secrets.Delete(secretName(id))
	h.mu.Lock()
	delete(h.status, id)
	h.mu.Unlock()
	return h.load()
}

// Test 立刻发一条测试通知，返回发送结果。
func (h *Hub) Test(ctx context.Context, id string) error {
	h.mu.Lock()
	var c *Channel
	for _, x := range h.channels {
		if x.ID == id {
			cc := *x
			c = &cc
		}
	}
	h.mu.Unlock()
	if c == nil {
		return errors.New("通知渠道不存在")
	}
	m := Message{Event: "test", Severity: Info, Time: time.Now(), Link: "/settings",
		Title:  i18n.Tx("Hostower 测试通知"),
		Fields: []Field{f(i18n.Tx("渠道"), c.Name), f(i18n.Tx("说明"), i18n.Tx("能看到这条消息，说明通知渠道配置正确。").In(c.Lang))}}
	err := h.deliver(ctx, c, m)
	h.record(c.ID, err)
	return err
}

// ---------- 分发 ----------

// Run 开始监听内核事件并发送通知，直到 ctx 结束。
func (h *Hub) Run(ctx context.Context, k *kernel.Kernel) {
	events, unsubscribe := k.Subscribe()
	defer unsubscribe()
	go h.worker(ctx)
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			switch e.Type {
			case "request":
				if r, ok := e.Data.(*kernel.Request); ok {
					h.onRequest(r)
				}
			case "hosts":
				if all, ok := e.Data.(map[string]metrics.Status); ok {
					h.onHosts(all)
				}
			}
		case <-tick.C:
			h.checkOffline()
		}
	}
}

// Write 实现审计的外部副本接口：安全相关的记录也转成通知。它只往队列里放，不会阻塞，也永远不返回错误。
func (h *Hub) Write(rec audit.Record) error {
	var e audit.Entry
	if json.Unmarshal(rec.Body, &e) != nil {
		return nil
	}
	if m, ok := fromAudit(e, rec.Seq); ok {
		h.enqueue(m)
	}
	return nil
}

func (h *Hub) enqueue(m Message) {
	if m.Time.IsZero() {
		m.Time = time.Now()
	}
	select {
	case h.queue <- m:
	default:
		h.log.Warn("通知队列已满，丢弃一条通知", "event", m.Event)
	}
}

func (h *Hub) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-h.queue:
			h.dispatch(ctx, m)
		}
	}
}

func (h *Hub) dispatch(ctx context.Context, m Message) {
	h.mu.Lock()
	key := m.Event + "|" + m.Title.In(i18n.Source()) + "|" + fieldsKey(m)
	if t, ok := h.recent[key]; ok && time.Since(t) < dedupWindow {
		h.mu.Unlock()
		return
	}
	h.recent[key] = time.Now()
	for k, t := range h.recent {
		if time.Since(t) > dedupWindow {
			delete(h.recent, k)
		}
	}
	var targets []*Channel
	for _, c := range h.channels {
		if c.Enabled && subscribed(c, m) {
			cc := *c
			targets = append(targets, &cc)
		}
	}
	h.mu.Unlock()
	for _, c := range targets {
		if !h.allow(c.ID) {
			h.mu.Lock()
			h.st(c.ID).Dropped++
			h.mu.Unlock()
			continue
		}
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = h.deliver(ctx, c, m); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(attempt+1) * 3 * time.Second):
			}
		}
		h.record(c.ID, err)
		if err != nil {
			h.log.Warn("通知发送失败", "channel", c.Name, "event", m.Event, "err", err)
		}
	}
}

func fieldsKey(m Message) string {
	var b strings.Builder
	for _, f := range m.Fields {
		b.WriteString(f.Value + "|")
	}
	return b.String()
}

func subscribed(c *Channel, m Message) bool {
	ok := false
	for _, e := range c.Events {
		ok = ok || e == m.Event
	}
	if !ok {
		return false
	}
	if len(c.Hosts) == 0 {
		return true
	}
	hosts := hostsOf(m)
	if len(hosts) == 0 {
		return true // 与主机无关的通知（登录失败、更新等）照常推送
	}
	for _, want := range c.Hosts {
		for _, h := range hosts {
			if want == h {
				return true
			}
		}
	}
	return false
}

func hostsOf(m Message) []string {
	for _, f := range m.Fields {
		if f.Name.Key == "主机" {
			return strings.Split(strings.ReplaceAll(f.Value, " ", ""), ",")
		}
	}
	return nil
}

func (h *Hub) allow(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range h.sent[id] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= perMinute {
		h.sent[id] = keep
		return false
	}
	h.sent[id] = append(keep, now)
	return true
}

func (h *Hub) st(id string) *Status {
	s := h.status[id]
	if s == nil {
		s = &Status{}
		h.status[id] = s
	}
	return s
}

func (h *Hub) record(id string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.st(id)
	if err != nil {
		s.LastError, s.LastErrAt = err.Error(), time.Now().UnixMilli()
	} else {
		s.LastOK, s.Sent = time.Now().UnixMilli(), s.Sent+1
	}
}

func (h *Hub) deliver(ctx context.Context, c *Channel, m Message) error {
	raw, err := h.secrets.Get(secretName(c.ID))
	if err != nil {
		return errors.New("读不到这个渠道的令牌或密码")
	}
	var sec Secret
	if json.Unmarshal(raw, &sec) != nil {
		return errors.New("这个渠道保存的令牌或密码已损坏，请重新填写")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	base := ""
	if h.base != nil {
		base = h.base()
	}
	switch c.Kind {
	case "telegram":
		return sendTelegram(ctx, sec, c, m, base)
	case "feishu":
		return sendFeishu(ctx, sec, c, m, base)
	case "discord":
		return sendDiscord(ctx, sec, c, m, base)
	case "email":
		return sendEmail(ctx, sec, c, m, base)
	}
	return errors.New("未知的通知渠道类型")
}

// ---------- 从事件到通知 ----------

func hostList(r *kernel.Request) string {
	var hs []string
	for _, t := range r.Targets {
		hs = append(hs, t.Host)
	}
	if len(hs) > 6 {
		return strings.Join(hs[:6], ", ") + fmt.Sprintf(" … (+%d)", len(hs)-6)
	}
	return strings.Join(hs, ", ")
}

func firstCommand(r *kernel.Request) string {
	if r.Kind == "file.write" && r.File != nil {
		return "写入 " + r.File.Path
	}
	if len(r.Targets) > 0 {
		return clip(kernel.Redact(r.Targets[0].Command), 160)
	}
	return ""
}

func (h *Hub) onRequest(r *kernel.Request) {
	h.mu.Lock()
	prev, seen := h.reqState[r.ID]
	h.reqState[r.ID] = r.State
	if len(h.reqState) > 5000 {
		h.reqState = map[string]kernel.State{r.ID: r.State}
	}
	h.mu.Unlock()
	if seen && prev == r.State {
		return
	}
	link := "/requests/" + r.ID
	base := []Field{
		f(i18n.Tx("主机"), hostList(r)), f(i18n.Tx("等级"), r.Level.String()),
		f(i18n.Tx("发起人"), r.Actor.Name), f(i18n.Tx("意图"), clip(r.Intent, 120)), f(i18n.Tx("命令"), firstCommand(r)),
	}
	switch r.State {
	case kernel.StatePending:
		if r.Kind == "terminal" {
			return // 开终端的确认是当场进行的，不用通知别人去批准
		}
		h.enqueue(Message{Event: "approval.pending", Severity: Warn, Link: link, Fields: base,
			Title: i18n.Tx("有一个 {level} 请求等待批准", "level", r.Level.String())})
	case kernel.StateFailed:
		var failed []string
		for _, t := range r.Targets {
			if t.State == "failed" || t.State == "error" {
				failed = append(failed, t.Host)
			}
		}
		h.enqueue(Message{Event: "request.failed", Severity: Crit, Link: link,
			Fields: append(base, f(i18n.Tx("失败的主机"), strings.Join(failed, ", "))),
			Title:  i18n.Tx("执行失败")})
	case kernel.StateDone:
		if r.Level.String() == "L0" {
			return // 只读操作自动执行，太多了，不通知
		}
		h.enqueue(Message{Event: "request.done", Severity: OK, Link: link, Fields: base, Title: i18n.Tx("执行完成")})
	case kernel.StateRejected:
		h.enqueue(Message{Event: "request.rejected", Severity: Crit, Link: link, Fields: base,
			Title: i18n.Tx("一个禁止的操作被内核拒绝（L3）")})
	}
}

func (h *Hub) onHosts(all map[string]metrics.Status) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for name, st := range all {
		if !st.Known {
			continue
		}
		t := h.hostState[name]
		if t == nil {
			// 第一次见到：只记下状态，不通知（启动时的状态不是“变化”）。
			h.hostState[name] = &hostTrack{online: st.Online, since: now, announced: !st.Online}
			continue
		}
		if t.online == st.Online {
			continue
		}
		t.online, t.since = st.Online, now
		if st.Online && t.announced {
			t.announced = false
			h.queueLocked(Message{Event: "host.online", Severity: OK, Link: "/hosts/" + name,
				Title: i18n.Tx("主机恢复在线：{host}", "host", name), Fields: []Field{f(i18n.Tx("主机"), name)}})
		}
		if !st.Online {
			t.announced = false
			t.lastErr = st.Error
		}
	}
	for name := range h.hostState {
		if _, ok := all[name]; !ok {
			delete(h.hostState, name)
		}
	}
}

// checkOffline 发出“离线超过两分钟”的通知。
func (h *Hub) checkOffline() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for name, t := range h.hostState {
		if !t.online && !t.announced && time.Since(t.since) >= offlineFor {
			t.announced = true
			h.queueLocked(Message{Event: "host.offline", Severity: Crit, Link: "/hosts/" + name,
				Title:  i18n.Tx("主机离线：{host}", "host", name),
				Fields: []Field{f(i18n.Tx("主机"), name), f(i18n.Tx("离线时间"), t.since.Format("2006-01-02 15:04:05")), f(i18n.Tx("原因"), clip(t.lastErr, 200))}})
		}
	}
}

func (h *Hub) queueLocked(m Message) {
	m.Time = time.Now()
	select {
	case h.queue <- m:
	default:
	}
}

func ipOf(e audit.Entry) string {
	if v, ok := e.Detail["ip"].(string); ok {
		return v
	}
	return ""
}

// fromAudit 把安全相关的审计记录转成通知。
func fromAudit(e audit.Entry, seq int64) (Message, bool) {
	link := fmt.Sprintf("/audit?seq=%d", seq)
	who := e.Actor
	if who == "" {
		who = "—"
	}
	switch e.Type {
	case "auth.login.failed", "auth.reauth.failed", "auth.setup.failed", "chat.bind.failed":
		return Message{Event: "auth.failed", Severity: Crit, Link: "/audit",
			Title:  i18n.Tx("登录或身份验证失败"),
			Fields: []Field{f(i18n.Tx("事件"), e.Type), f(i18n.Tx("说明"), clip(e.Message, 200)), f(i18n.Tx("来源"), ipOf(e))}}, true
	case "terminal.opened":
		return Message{Event: "terminal.opened", Severity: Warn, Link: link,
			Title:  i18n.Tx("有人打开了交互终端"),
			Fields: []Field{f(i18n.Tx("主机"), e.Host), f(i18n.Tx("谁"), who)}}, true
	case "system.paused":
		return Message{Event: "system.paused", Severity: Crit, Link: "/settings",
			Title: i18n.Tx("全部执行已暂停"), Fields: []Field{f(i18n.Tx("谁"), who)}}, true
	case "system.resumed":
		return Message{Event: "system.paused", Severity: OK, Link: "/settings",
			Title: i18n.Tx("已恢复执行"), Fields: []Field{f(i18n.Tx("谁"), who)}}, true
	case "system.sessions.revoked":
		return Message{Event: "system.paused", Severity: Warn, Link: "/settings",
			Title: i18n.Tx("所有会话已被吊销"), Fields: []Field{f(i18n.Tx("谁"), who)}}, true
	case "secret.updated", "secret.deleted":
		return Message{Event: "secret.changed", Severity: Warn, Link: link,
			Title:  i18n.Tx("凭据有改动"),
			Fields: []Field{f(i18n.Tx("说明"), clip(e.Message, 200)), f(i18n.Tx("谁"), who)}}, true
	case "system.update.applied":
		return Message{Event: "system.update", Severity: OK, Link: "/settings",
			Title: i18n.Tx("Hostower 已更新"), Fields: []Field{f(i18n.Tx("说明"), e.Message)}}, true
	case "system.update.rejected":
		return Message{Event: "system.update", Severity: Crit, Link: "/settings",
			Title: i18n.Tx("一次更新因签名不对被拒绝"), Fields: []Field{f(i18n.Tx("说明"), e.Message)}}, true
	}
	return Message{}, false
}
