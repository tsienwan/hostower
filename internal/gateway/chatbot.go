package gateway

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tsienwan/hostower/internal/i18n"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/metrics"
	"github.com/tsienwan/hostower/internal/notify"
)

// 聊天入口：所有者把自己的 Telegram 账号绑定到 Hostower 之后，可以私聊机器人查状态、提交命令、紧急暂停。
//
// 它是网关的一个入口，不是旁路：发送者由 Telegram 认证（消息里的 from.id 由 Telegram 服务器填写，别人冒充不了），
// 网关核对它就是绑定的那个账号之后，才构造 kernel.Actor（Entry 是 "chat"），之后的一切仍然走 Submit* → 判级 → 审批。
// 聊天里不能批准任何请求（CanApprove 为 false；L2 本来就只能在网页上批准），也不能恢复执行、改设置、开终端。
//
// 绑定只能由已经用 Passkey 登录的所有者在网页上发起：网页生成一次性配对码，所有者私聊机器人发 /bind 配对码。
// 绑定有有效期，过期后要回网页重新配对。陌生人发来的消息一律不回复。

const (
	chatSettingKey  = "chatbot"
	pairTTL         = 5 * time.Minute
	pairMaxTries    = 5
	chatMaxAge      = 2 * time.Minute // 比这更早的消息不处理（例如机器人离线时积压的）
	chatPerMinute   = 20
	chatWaitL0      = 30 * time.Second
	chatPollTimeout = 25
	pairAlphabet    = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// chatTransport 是机器人收发消息的方式。正式运行时是 notify.TelegramBot，测试时是假的。
type chatTransport interface {
	Updates(ctx context.Context, offset int64, timeout int) ([]notify.Update, error)
	Send(ctx context.Context, chatID int64, text string) error
	Me(ctx context.Context) (string, error)
	SetCommands(ctx context.Context, cmds []notify.BotCommand, lang string) error
}

type chatConfig struct {
	Enabled bool         `json:"enabled"`
	Channel string       `json:"channel"`
	TTLDays int          `json:"ttlDays"`
	Binding *chatBinding `json:"binding,omitempty"`
}

type chatBinding struct {
	ChatUserID int64     `json:"chatUserId"`
	ChatName   string    `json:"chatName"`
	UserID     string    `json:"userId"`
	UserName   string    `json:"userName"`
	BoundAt    time.Time `json:"boundAt"`
	// ExpiresAt 为空表示永久有效（有效期设成“永久”时）。永久的绑定仍然可以在网页上解绑，“吊销所有会话”也会解除它。
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

type chatPairing struct {
	code     string
	userID   string
	userName string
	expires  time.Time
	tries    int
}

type chatBot struct {
	s  *Server
	db *sql.DB

	mu      sync.Mutex
	cfg     chatConfig
	pair    *chatPairing
	running bool
	lastErr string
	botName string
	hits    []time.Time
	warned  bool
	restart chan struct{}

	// 测试里替换。
	transport func(channel string) (chatTransport, error)
	now       func() time.Time
}

func newChatBot(s *Server) *chatBot {
	b := &chatBot{s: s, db: s.DB, restart: make(chan struct{}, 1), now: time.Now, cfg: chatConfig{TTLDays: 30}}
	b.transport = func(channel string) (chatTransport, error) {
		if s.Notify == nil {
			return nil, errors.New("通知中心没有启动")
		}
		return s.Notify.TelegramBot(channel)
	}
	if b.db != nil {
		var raw string
		if b.db.QueryRow("SELECT value FROM settings WHERE key = ?", chatSettingKey).Scan(&raw) == nil {
			json.Unmarshal([]byte(raw), &b.cfg)
		}
	}
	if b.cfg.TTLDays < 0 || b.cfg.TTLDays > 365 {
		b.cfg.TTLDays = 30
	}
	return b
}

// bindingExpiry 算出绑定的到期时间；days 为 0 表示永久，返回 nil。
func bindingExpiry(boundAt time.Time, days int) *time.Time {
	if days == 0 {
		return nil
	}
	t := boundAt.Add(time.Duration(days) * 24 * time.Hour)
	return &t
}

// untilText 是回复里“到什么时候”的那几个字（中文、英文）。
func untilText(lang string, exp *time.Time) string {
	if exp == nil {
		return i18n.T(lang, "永久有效")
	}
	return i18n.T(lang, "{date} 到期", "date", exp.Format("2006-01-02"))
}

// saveLocked 把配置写进数据库。调用方持有 b.mu。
func (b *chatBot) saveLocked() error {
	if b.db == nil {
		return nil
	}
	raw, _ := json.Marshal(b.cfg)
	_, err := b.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, chatSettingKey, string(raw))
	return err
}

func (b *chatBot) kick() {
	select {
	case b.restart <- struct{}{}:
	default:
	}
}

func (b *chatBot) audit(ctx context.Context, e audit.Entry) error {
	if e.Entry == "" {
		e.Entry = "chat"
	}
	_, err := b.s.Kernel.Audit().Append(ctx, e)
	return err
}

// lang 是机器人回复用的语言：跟着所用 Telegram 渠道的通知语言。
func (b *chatBot) lang(channel string) string {
	if b.s.Notify == nil {
		return i18n.Source()
	}
	list, _ := b.s.Notify.Channels()
	for _, c := range list {
		if c.ID == channel {
			return c.Lang
		}
	}
	return i18n.Source()
}

// ---------- 运行 ----------

// RunChatBot 在后台接收机器人的消息，直到 ctx 结束。没启用时只是等着。
func (s *Server) RunChatBot(ctx context.Context) {
	b := s.bot
	for ctx.Err() == nil {
		b.mu.Lock()
		cfg := b.cfg
		b.mu.Unlock()
		if !cfg.Enabled || cfg.Channel == "" {
			b.setState(false, "", "")
			b.wait(ctx, time.Minute)
			continue
		}
		err := b.session(ctx, cfg.Channel)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			b.setState(false, err.Error(), "")
			s.Log.Warn("聊天机器人出错，稍后重试", "err", err)
			b.wait(ctx, 30*time.Second)
		}
	}
}

func (b *chatBot) wait(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-b.restart:
	case <-time.After(d):
	}
}

func (b *chatBot) setState(running bool, errMsg, name string) {
	b.mu.Lock()
	b.running, b.lastErr = running, errMsg
	if name != "" || !running {
		b.botName = name
	}
	b.mu.Unlock()
}

// session 用一个机器人收消息，直到出错或配置改动（kick）。
func (b *chatBot) session(ctx context.Context, channel string) error {
	t, err := b.transport(channel)
	if err != nil {
		return err
	}
	name, err := t.Me(ctx)
	if err != nil {
		return err
	}
	cmds := func(lang string) []notify.BotCommand {
		return []notify.BotCommand{
			{Command: "status", Description: i18n.T(lang, "主机和审批概况")},
			{Command: "pending", Description: i18n.T(lang, "等待批准的请求")},
			{Command: "failed", Description: i18n.T(lang, "最近失败的执行")},
			{Command: "traffic", Description: i18n.T(lang, "本周期的流量用量")},
			{Command: "run", Description: i18n.T(lang, "提交命令：/run 主机 命令")},
			{Command: "pause", Description: i18n.T(lang, "紧急暂停全部执行")},
			{Command: "help", Description: i18n.T(lang, "帮助")},
		}
	}
	// 命令菜单：默认英文；清单里有 Telegram 语言代码的语言各设一份。
	t.SetCommands(ctx, cmds("en"), "")
	for _, l := range i18n.Languages() {
		if l.Telegram != "" {
			t.SetCommands(ctx, cmds(l.Code), l.Telegram)
		}
	}
	// 跳过机器人没在线时积压的消息：只取最后一条的编号，从它之后开始。
	var offset int64
	if ups, err := t.Updates(ctx, -1, 0); err != nil {
		return err
	} else if len(ups) > 0 {
		offset = ups[len(ups)-1].ID + 1
	}
	b.setState(true, "", name)

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-b.restart:
			cancel()
		case <-sctx.Done():
		}
	}()
	for {
		ups, err := t.Updates(sctx, offset, chatPollTimeout)
		if sctx.Err() != nil {
			return nil // 配置改了或者要退出：重新开始
		}
		if err != nil {
			return err
		}
		for _, u := range ups {
			if u.ID >= offset {
				offset = u.ID + 1
			}
			b.handle(sctx, t, channel, u)
		}
	}
}

// ---------- 处理一条消息 ----------

func displayName(u *notify.Update) string {
	f := u.Message.From
	if f.Username != "" {
		return "@" + f.Username
	}
	return clipRunes(f.FirstName, 40)
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func splitCommand(text string) (string, string) {
	text = strings.TrimSpace(text)
	cmd, args, _ := strings.Cut(text, " ")
	cmd, _, _ = strings.Cut(strings.ToLower(cmd), "@") // /status@机器人名
	return cmd, strings.TrimSpace(args)
}

func (b *chatBot) handle(ctx context.Context, t chatTransport, channel string, u notify.Update) {
	m := u.Message
	// 只认私聊里真人发的消息；群组、频道、别的机器人一律忽略。
	if m == nil || m.From == nil || m.From.IsBot || m.Chat.Type != "private" || m.Chat.ID != m.From.ID {
		return
	}
	now := b.now()
	if now.Sub(time.Unix(m.Date, 0)) > chatMaxAge {
		return
	}
	cmd, args := splitCommand(m.Text)
	lang := b.lang(channel)
	reply := func(text string) { t.Send(ctx, m.Chat.ID, text) }

	b.mu.Lock()
	bind := b.cfg.Binding
	if bind == nil || bind.ChatUserID != m.From.ID {
		// 不是绑定的账号：只有网页上刚生成了配对码、对方发的是 /bind 时才回应。其他一律沉默。
		p := b.pair
		if cmd != "/bind" || p == nil || now.After(p.expires) {
			b.mu.Unlock()
			return
		}
		code := strings.ToUpper(strings.ReplaceAll(args, " ", ""))
		if subtle.ConstantTimeCompare([]byte(code), []byte(p.code)) != 1 {
			p.tries++
			left := pairMaxTries - p.tries
			if left <= 0 {
				b.pair = nil
			}
			b.mu.Unlock()
			b.audit(ctx, audit.Entry{Type: "chat.bind.failed", Actor: displayName(&u), Message: "聊天账号绑定失败：配对码不对",
				Detail: map[string]any{"chatUserId": m.From.ID}})
			if left <= 0 {
				reply(i18n.T(lang, "配对码不对。错误次数太多，这个配对码已作废，请回网页重新生成。"))
			} else {
				reply(i18n.T(lang, "配对码不对，还可以再试 {n} 次。", "n", left))
			}
			return
		}
		nb := &chatBinding{ChatUserID: m.From.ID, ChatName: displayName(&u), UserID: p.userID, UserName: p.userName,
			BoundAt: now, ExpiresAt: bindingExpiry(now, b.cfg.TTLDays)}
		if err := b.audit(ctx, audit.Entry{Type: "chat.bound", Actor: p.userName, Message: "绑定聊天账号 " + nb.ChatName,
			Detail: map[string]any{"chatUserId": nb.ChatUserID, "expiresAt": nb.ExpiresAt}}); err != nil {
			b.mu.Unlock()
			reply(i18n.T(lang, "写审计失败，没有绑定。"))
			return
		}
		b.cfg.Binding, b.pair = nb, nil
		b.saveLocked()
		b.mu.Unlock()
		reply(i18n.T(lang, "已绑定到 Hostower 账号 {user}，{until}。", "user", nb.UserName, "until", untilText(lang, nb.ExpiresAt)) + "\n\n" + helpText(lang))
		return
	}
	// 绑定的账号：先看绑定还有没有效。
	if bind.ExpiresAt != nil && now.After(*bind.ExpiresAt) {
		b.cfg.Binding = nil
		b.saveLocked()
		b.mu.Unlock()
		b.audit(ctx, audit.Entry{Type: "chat.binding.expired", Actor: bind.UserName, Message: "聊天账号绑定已过期：" + bind.ChatName})
		reply(i18n.T(lang, "绑定已过期。请回 Hostower 网页的“设置 → 通知与机器人”重新配对。"))
		return
	}
	// 限流：每分钟最多 20 条命令，超出的只提醒一次。
	var keep []time.Time
	for _, h := range b.hits {
		if now.Sub(h) < time.Minute {
			keep = append(keep, h)
		}
	}
	if len(keep) >= chatPerMinute {
		b.hits = keep
		warn := !b.warned
		b.warned = true
		b.mu.Unlock()
		if warn {
			reply(i18n.T(lang, "命令太频繁了，请一分钟后再试。"))
		}
		return
	}
	b.hits, b.warned = append(keep, now), false
	b.mu.Unlock()

	// 绑定的 Hostower 账号必须还在，而且仍然是所有者。
	user, err := b.s.Auth.User(bind.UserID)
	if err != nil || user.Role != "owner" {
		b.unbind(ctx, "绑定的 Hostower 账号已不可用")
		reply(i18n.T(lang, "绑定的 Hostower 账号已不可用，绑定已解除。"))
		return
	}
	a := kernel.Actor{ID: user.ID, Name: user.Name, Kind: kernel.Human, Entry: "chat", MaxLevel: risk.L2, CanApprove: false}

	// 每条命令都写审计；写不进去就不执行。
	if err := b.audit(ctx, audit.Entry{Type: "chat.command", Actor: user.Name, Message: clipRunes(kernel.Redact(cmd+" "+args), 200),
		Detail: map[string]any{"chatUserId": m.From.ID}}); err != nil {
		reply(i18n.T(lang, "写审计失败，命令没有执行。"))
		return
	}
	switch cmd {
	case "/start", "/help":
		reply(helpText(lang))
	case "/bind":
		reply(i18n.T(lang, "这个账号已经绑定好了。"))
	case "/status":
		reply(b.status(lang))
	case "/pending":
		reply(b.pending(lang))
	case "/failed":
		reply(b.failed(lang))
	case "/traffic":
		reply(b.traffic(ctx, lang))
	case "/run":
		reply(b.run(ctx, a, args, lang))
	case "/pause":
		if b.s.Kernel.Paused() {
			reply(i18n.T(lang, "执行本来就是暂停的。"))
		} else if err := b.s.Kernel.SetPaused(ctx, a, true); err != nil {
			reply(i18n.T(lang, "暂停失败：") + err.Error())
		} else {
			reply(i18n.T(lang, "已暂停全部执行。恢复只能在网页上操作。"))
		}
	case "/unbind":
		b.unbind(ctx, "在聊天里解除绑定")
		reply(i18n.T(lang, "已解除绑定。"))
	default:
		reply(i18n.T(lang, "不认识这个命令。") + "\n\n" + helpText(lang))
	}
}

func helpText(lang string) string {
	return i18n.T(lang, "可以用的命令：\n/status 主机和审批概况\n/pending 等待批准的请求\n/failed 最近失败的执行\n/traffic 本周期的流量用量\n/run 主机[,主机] 命令 —— 提交命令（只读的直接执行，其他的要到网页上批准）\n/pause 紧急暂停全部执行（恢复只能在网页上）\n/unbind 解除绑定\n\n这里不能批准请求、不能恢复执行、不能改设置。")
}

// traffic 列出各主机本周期的流量：设了配额的显示用量、百分比、剩余天数和预计用量，按百分比排在前面；没设的按本月用量排。
func (b *chatBot) traffic(ctx context.Context, lang string) string {
	if b.s.Sampler == nil {
		return i18n.T(lang, "还没有流量数据。")
	}
	var names []string
	for _, h := range b.s.Kernel.Inventory().Hosts() {
		names = append(names, h.Name)
	}
	list, err := b.s.Sampler.TrafficRanking(ctx, names)
	if err != nil {
		return i18n.T(lang, "读取流量失败：") + err.Error()
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, c := list[i].Quota, list[j].Quota
		if (a != nil) != (c != nil) {
			return a != nil
		}
		if a != nil {
			return a.Pct > c.Pct
		}
		return list[i].In+list[i].Out > list[j].In+list[j].Out
	})
	var lines []string
	for _, t := range list {
		if q := t.Quota; q != nil {
			qq := metrics.Quota{Unit: q.Unit, Base: q.Base}
			line := i18n.T(lang, "{host}：{used} / {limit}（{pct}%），还剩 {days} 天", "host", t.Host, "used", qq.FormatBytes(q.Used),
				"limit", qq.FormatBytes(q.Limit), "pct", fmt.Sprintf("%.1f", q.Pct), "days", fmt.Sprintf("%.1f", q.DaysLeft))
			if q.Projected > 0 {
				line += i18n.T(lang, "，预计 {v}", "v", qq.FormatBytes(q.Projected))
				if q.Projected > q.Limit {
					line += " ⚠"
				}
			}
			lines = append(lines, line)
		} else if t.In+t.Out > 0 {
			qq := metrics.Quota{Unit: "GB", Base: 1000}
			lines = append(lines, i18n.T(lang, "{host}：本月 ↓{in} ↑{out}", "host", t.Host, "in", qq.FormatBytes(t.In), "out", qq.FormatBytes(t.Out)))
		}
		if len(lines) >= 20 {
			break
		}
	}
	if len(lines) == 0 {
		return i18n.T(lang, "还没有流量数据。")
	}
	return i18n.T(lang, "本周期流量：") + "\n" + strings.Join(lines, "\n")
}

func (b *chatBot) link(path string) string {
	return strings.TrimRight(b.s.PublicURL, "/") + path
}

func (b *chatBot) status(lang string) string {
	var online, offline, unknown []string
	if b.s.Sampler != nil {
		for name, st := range b.s.Sampler.All(false) {
			switch {
			case !st.Known:
				unknown = append(unknown, name)
			case st.Online:
				online = append(online, name)
			default:
				offline = append(offline, name)
			}
		}
	}
	sort.Strings(offline)
	pending := 0
	for _, r := range b.s.Kernel.List(500) {
		if r.State == kernel.StatePending {
			pending++
		}
	}
	var sb strings.Builder
	if b.s.Kernel.Paused() {
		sb.WriteString(i18n.T(lang, "⛔ 全部执行已暂停\n"))
	}
	sb.WriteString(i18n.T(lang, "主机：{online} 在线，{offline} 离线", "online", len(online), "offline", len(offline)))
	if len(unknown) > 0 {
		sb.WriteString(i18n.T(lang, "，{n} 还在连接", "n", len(unknown)))
	}
	sb.WriteString("\n")
	if len(offline) > 0 {
		if len(offline) > 10 {
			offline = append(offline[:10], "…")
		}
		sb.WriteString(i18n.T(lang, "离线：") + strings.Join(offline, ", ") + "\n")
	}
	sb.WriteString(i18n.T(lang, "等待批准：{n}", "n", pending))
	return sb.String()
}

func reqLine(r *kernel.Request) string {
	hosts := make([]string, 0, len(r.Targets))
	for _, t := range r.Targets {
		hosts = append(hosts, t.Host)
	}
	what := r.Intent
	if what == "" && len(r.Targets) > 0 {
		what = r.Targets[0].Command
	}
	return fmt.Sprintf("%s · %s · %s", r.Level, clipRunes(strings.Join(hosts, ","), 40), clipRunes(kernel.Redact(what), 80))
}

func (b *chatBot) pending(lang string) string {
	var lines []string
	for _, r := range b.s.Kernel.List(500) {
		if r.State == kernel.StatePending && len(lines) < 10 {
			lines = append(lines, "• "+reqLine(r)+"\n  "+b.link("/requests/"+r.ID))
		}
	}
	if len(lines) == 0 {
		return i18n.T(lang, "没有等待批准的请求。")
	}
	return i18n.T(lang, "等待批准（请到网页上批准）：\n") + strings.Join(lines, "\n")
}

func (b *chatBot) failed(lang string) string {
	var lines []string
	since := b.now().Add(-24 * time.Hour)
	for _, r := range b.s.Kernel.List(500) {
		if r.State == kernel.StateFailed && r.CreatedAt.After(since) && len(lines) < 5 {
			lines = append(lines, "• "+reqLine(r)+"\n  "+b.link("/requests/"+r.ID))
		}
	}
	if len(lines) == 0 {
		return i18n.T(lang, "最近 24 小时没有失败的执行。")
	}
	return i18n.T(lang, "最近 24 小时失败的执行：\n") + strings.Join(lines, "\n")
}

// run 提交一条命令。它和网页上的“执行”走同一个内核入口：判级、审批一样不少。
func (b *chatBot) run(ctx context.Context, a kernel.Actor, args, lang string) string {
	hostPart, command, _ := strings.Cut(args, " ")
	command = strings.TrimSpace(command)
	if hostPart == "" || command == "" {
		return i18n.T(lang, "用法：/run 主机[,主机] 命令\n例如：/run web-1 df -h")
	}
	spec := kernel.CommandSpec{Commands: map[string]string{}, Intent: "从 Telegram 提交"}
	for _, h := range strings.Split(hostPart, ",") {
		if h = strings.TrimSpace(h); h != "" {
			spec.Hosts = append(spec.Hosts, h)
			spec.Commands[h] = command
		}
	}
	r, err := b.s.Kernel.SubmitCommand(ctx, a, spec)
	if err != nil {
		return i18n.T(lang, "提交失败：") + err.Error()
	}
	link := b.link("/requests/" + r.ID)
	switch r.State {
	case kernel.StateRejected:
		return i18n.T(lang, "被内核拒绝：这是禁止的操作（L3），任何人、任何入口都不能放行。\n") + link
	case kernel.StatePending:
		return i18n.T(lang, "这是 {level} 操作，已提交，等待批准。请到网页上批准：\n", "level", r.Level.String()) + link
	}
	// L0：自动执行，等一会儿把结果带回来。
	deadline := time.Now().Add(chatWaitL0)
	for time.Now().Before(deadline) && r.State == kernel.StateRunning {
		select {
		case <-ctx.Done():
			return link
		case <-time.After(300 * time.Millisecond):
		}
		if r, err = b.s.Kernel.Get(r.ID); err != nil {
			return link
		}
	}
	if r.State == kernel.StateRunning {
		return i18n.T(lang, "还在执行，结果见：\n") + link
	}
	var sb strings.Builder
	budget := 3200 / max(1, len(r.Targets))
	for _, t := range r.Targets {
		code := ""
		if t.ExitCode != nil {
			code = fmt.Sprintf(" (exit %d)", *t.ExitCode)
		}
		fmt.Fprintf(&sb, "[%s] %s%s\n", t.Host, t.State, code)
		out := strings.TrimSpace(kernel.Redact(t.Output))
		if t.Error != "" {
			out = strings.TrimSpace(out + "\n" + t.Error)
		}
		if rs := []rune(out); len(rs) > budget {
			out = "…" + string(rs[len(rs)-budget:])
		}
		if out != "" {
			sb.WriteString(out + "\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(link)
	return sb.String()
}

func (b *chatBot) unbind(ctx context.Context, why string) {
	b.mu.Lock()
	old := b.cfg.Binding
	b.cfg.Binding = nil
	b.saveLocked()
	b.mu.Unlock()
	if old != nil {
		b.audit(ctx, audit.Entry{Type: "chat.unbound", Actor: old.UserName, Message: "解除聊天账号绑定 " + old.ChatName + "：" + why})
	}
}

// ---------- 网页上的管理接口（只有所有者） ----------

type chatView struct {
	Enabled     bool         `json:"enabled"`
	Channel     string       `json:"channel"`
	TTLDays     int          `json:"ttlDays"`
	Binding     *chatBinding `json:"binding,omitempty"`
	Pairing     *pairingView `json:"pairing,omitempty"`
	Running     bool         `json:"running"`
	LastError   string       `json:"lastError,omitempty"`
	BotUsername string       `json:"botUsername,omitempty"`
	Channels    []chanView   `json:"channels"`
}

type pairingView struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type chanView struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) chatView(userID string) chatView {
	b := s.bot
	b.mu.Lock()
	v := chatView{Enabled: b.cfg.Enabled, Channel: b.cfg.Channel, TTLDays: b.cfg.TTLDays, Binding: b.cfg.Binding,
		Running: b.running, LastError: b.lastErr, BotUsername: b.botName, Channels: []chanView{}}
	if p := b.pair; p != nil && p.userID == userID && b.now().Before(p.expires) {
		v.Pairing = &pairingView{Code: p.code, ExpiresAt: p.expires}
	}
	b.mu.Unlock()
	if s.Notify != nil {
		list, _ := s.Notify.Channels()
		for _, c := range list {
			if c.Kind == "telegram" {
				v.Channels = append(v.Channels, chanView{ID: c.ID, Name: c.Name, Enabled: c.Enabled})
			}
		}
	}
	return v
}

func (s *Server) chatGet(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	writeJSON(w, 200, s.chatView(sessionOf(r).User.ID))
}

func (s *Server) chatPut(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Enabled bool   `json:"enabled"`
		Channel string `json:"channel"`
		TTLDays int    `json:"ttlDays"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.TTLDays < 0 || in.TTLDays > 365 {
		writeErr(w, 400, "绑定有效期要在 1 到 365 天之间，或者选永久")
		return
	}
	if in.Channel != "" {
		ok := false
		for _, c := range s.chatView("").Channels {
			ok = ok || c.ID == in.Channel
		}
		if !ok {
			writeErr(w, 400, "选择的 Telegram 渠道不存在")
			return
		}
	}
	if in.Enabled && in.Channel == "" {
		writeErr(w, 400, "先选择用哪个 Telegram 渠道的机器人")
		return
	}
	b := s.bot
	b.mu.Lock()
	prev := b.cfg
	if in.Channel != prev.Channel {
		// 换了机器人：原来的绑定、配对码都作废。
		b.cfg.Binding, b.pair = nil, nil
	}
	b.cfg.Enabled, b.cfg.Channel, b.cfg.TTLDays = in.Enabled, in.Channel, in.TTLDays
	if bd := b.cfg.Binding; bd != nil {
		bd.ExpiresAt = bindingExpiry(bd.BoundAt, in.TTLDays)
	}
	if !in.Enabled {
		b.pair = nil
	}
	err := b.saveLocked()
	b.mu.Unlock()
	if err != nil {
		writeErr(w, 500, "保存失败")
		return
	}
	s.audit(r, audit.Entry{Type: "chat.config.updated", Message: "修改聊天命令设置",
		Detail: map[string]any{"enabled": in.Enabled, "channel": in.Channel, "ttlDays": in.TTLDays, "bindingCleared": prev.Binding != nil && in.Channel != prev.Channel}})
	b.kick()
	writeJSON(w, 200, s.chatView(sessionOf(r).User.ID))
}

func (s *Server) chatPair(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	b := s.bot
	b.mu.Lock()
	if !b.cfg.Enabled || b.cfg.Channel == "" {
		b.mu.Unlock()
		writeErr(w, 400, "先打开聊天命令并选择机器人")
		return
	}
	buf := make([]byte, 8)
	rand.Read(buf)
	code := make([]byte, 8)
	for i, c := range buf {
		code[i] = pairAlphabet[int(c)%len(pairAlphabet)]
	}
	u := sessionOf(r).User
	b.pair = &chatPairing{code: string(code), userID: u.ID, userName: u.Name, expires: b.now().Add(pairTTL)}
	b.mu.Unlock()
	s.audit(r, audit.Entry{Type: "chat.pair.started", Message: "生成聊天账号配对码"})
	writeJSON(w, 200, s.chatView(u.ID))
}

func (s *Server) chatUnbind(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	b := s.bot
	b.mu.Lock()
	old := b.cfg.Binding
	b.cfg.Binding, b.pair = nil, nil
	err := b.saveLocked()
	b.mu.Unlock()
	if err != nil {
		writeErr(w, 500, "保存失败")
		return
	}
	if old != nil {
		s.audit(r, audit.Entry{Type: "chat.unbound", Message: "解除聊天账号绑定 " + old.ChatName + "：在网页上解除"})
	}
	writeJSON(w, 200, s.chatView(sessionOf(r).User.ID))
}
