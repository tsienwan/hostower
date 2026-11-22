package kernel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// Config 是内核的配置。
type Config struct {
	DataDir        string
	CommandTimeout time.Duration // 单条命令超时
	PendingTTL     time.Duration // 请求等待批准的最长时间
	TwoPersonL2    bool          // L2 需要发起人之外的另一个人批准
	MaxOutput      int           // 每台主机保留的输出上限（字节）
	MaxHosts       int           // 一次请求最多的主机数
	// Secret 按引用名取凭据本体，只在把它送进命令的标准输入时调用。
	Secret func(name string) ([]byte, error)
}

func (c *Config) defaults() {
	if c.CommandTimeout == 0 {
		c.CommandTimeout = 10 * time.Minute
	}
	if c.PendingTTL == 0 {
		c.PendingTTL = 30 * time.Minute
	}
	if c.MaxOutput == 0 {
		c.MaxOutput = 256 << 10
	}
	if c.MaxHosts == 0 {
		c.MaxHosts = 500
	}
}

// Kernel 是安全内核。
type Kernel struct {
	cfg    Config
	db     *sql.DB
	engine *risk.Engine
	audit  *audit.Log
	execs  executor.Registry
	inv    atomic.Pointer[inventory.Inventory]
	paused atomic.Bool
	// auditRoutine：日常查看（翻目录、页面上的只读查询）要不要写审计。所有者的设置，默认不写。
	auditRoutine atomic.Bool

	mu      sync.Mutex
	reqs    map[string]*Request
	order   []string // 按创建时间升序
	cancels map[string]context.CancelFunc

	subMu sync.Mutex
	subs  map[chan Event]struct{}
}

// New 创建内核。它在启动时确认指标采样命令是只读的。
func New(cfg Config, db *sql.DB, engine *risk.Engine, log *audit.Log, execs executor.Registry, inv *inventory.Inventory) (*Kernel, error) {
	cfg.defaults()
	for _, c := range []string{executor.ProbeCommand, executor.StreamProbeCommand, executor.ContainerProbeCommand, executor.HardwareProbeCommand} {
		if v := engine.Classify(c); v.Level != risk.L0 {
			return nil, fmt.Errorf("内核自检失败：采样命令被判为 %s，必须是 L0", v.Level)
		}
	}
	k := &Kernel{
		cfg: cfg, db: db, engine: engine, audit: log, execs: execs,
		reqs: map[string]*Request{}, cancels: map[string]context.CancelFunc{},
		subs: map[chan Event]struct{}{},
	}
	k.inv.Store(inv)
	var paused string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = 'paused'").Scan(&paused); err == nil && paused == "1" {
		k.paused.Store(true)
	}
	var routine string
	if err := db.QueryRow("SELECT value FROM settings WHERE key = 'audit.routine'").Scan(&routine); err == nil && routine == "1" {
		k.auditRoutine.Store(true)
	}
	if err := k.load(); err != nil {
		return nil, err
	}
	go k.janitor()
	return k, nil
}

// Inventory 返回当前清单。
func (k *Kernel) Inventory() *inventory.Inventory { return k.inv.Load() }

// SetInventory 替换清单（重新加载配置时）。
func (k *Kernel) SetInventory(inv *inventory.Inventory) { k.inv.Store(inv) }

// Engine 返回判级引擎，供“预览判级”使用。预览没有副作用。
func (k *Kernel) Engine() *risk.Engine { return k.engine }

// Audit 返回审计日志（只有追加和读取）。
func (k *Kernel) Audit() *audit.Log { return k.audit }

// ---------- 事件 ----------

// Subscribe 订阅事件。慢的订阅者会丢事件，而不是把内存撑满。
func (k *Kernel) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	k.subMu.Lock()
	k.subs[ch] = struct{}{}
	k.subMu.Unlock()
	return ch, func() {
		k.subMu.Lock()
		delete(k.subs, ch)
		k.subMu.Unlock()
	}
}

// Publish 广播一个事件。
func (k *Kernel) Publish(e Event) {
	k.subMu.Lock()
	defer k.subMu.Unlock()
	for ch := range k.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// ---------- 持久化 ----------

func (k *Kernel) load() error {
	rows, err := k.db.Query("SELECT body FROM requests ORDER BY created_at DESC LIMIT 300")
	if err != nil {
		return err
	}
	defer rows.Close()
	var loaded []*Request
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return err
		}
		var r Request
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			continue
		}
		loaded = append(loaded, &r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for i := len(loaded) - 1; i >= 0; i-- {
		r := loaded[i]
		if !r.State.terminal() {
			// 塔台重启：未完成的请求不会自动继续，必须重新提交。
			r.State = StateInterrupted
			now := time.Now()
			r.FinishedAt = &now
			k.save(r)
		}
		k.reqs[r.ID] = r
		k.order = append(k.order, r.ID)
	}
	return nil
}

// save 把请求写回数据库。调用方持有 k.mu，或请求尚未发布。
// 执行中途（每台主机结束时都会存一次）不带输出：一个五十台主机的请求，每次都把所有输出序列化一遍、
// 而且是在持有 k.mu 的时候，会拖慢所有正在执行的命令。输出在请求结束时随最后一次保存一起写进去。
func (k *Kernel) save(r *Request) {
	var v any = r
	if r.State == StateRunning {
		v = clone(r, false)
	}
	body, err := json.Marshal(v)
	if err != nil {
		return
	}
	k.db.Exec(`INSERT INTO requests (id, created_at, state, body) VALUES (?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET state=excluded.state, body=excluded.body`,
		r.ID, r.CreatedAt.UTC().Format(time.RFC3339Nano), string(r.State), string(body))
}

func clone(r *Request, withOutput bool) *Request {
	c := *r
	c.Targets = make([]*Target, len(r.Targets))
	for i, t := range r.Targets {
		tc := *t
		if !withOutput {
			tc.Output = ""
		}
		c.Targets[i] = &tc
	}
	if r.File != nil && !withOutput {
		c.File = &FileWrite{Path: r.File.Path, OldExists: r.File.OldExists}
	}
	c.Approvals = append([]Approval(nil), r.Approvals...)
	return &c
}

// changed 保存并广播请求的最新状态。调用方持有 k.mu。
func (k *Kernel) changed(r *Request) {
	k.save(r)
	k.Publish(Event{Type: "request", Data: clone(r, false)})
}

// Get 返回一个请求的完整快照。
func (k *Kernel) Get(id string) (*Request, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.reqs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(r, true), nil
}

// List 返回最近的请求（不含输出），新的在前。
func (k *Kernel) List(limit int) []*Request {
	k.mu.Lock()
	defer k.mu.Unlock()
	if limit <= 0 || limit > len(k.order) {
		limit = len(k.order)
	}
	out := make([]*Request, 0, limit)
	for i := len(k.order) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, clone(k.reqs[k.order[i]], false))
	}
	return out
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "rq_" + hex.EncodeToString(b)
}

func digest(kind string, parts ...string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	for _, p := range parts {
		h.Write([]byte{0})
		h.Write([]byte(strconv.Itoa(len(p))))
		h.Write([]byte{0})
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func reasons(fs []risk.Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range fs {
		if f.Level == risk.L0 {
			continue
		}
		s := f.Level.String() + " " + f.Reason
		if f.Command != "" {
			s += "：" + f.Command
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ---------- 提交 ----------

// CommandSpec 是一次命令请求。Commands 是每台主机上渲染后的实际命令。
type CommandSpec struct {
	Hosts    []string
	Commands map[string]string
	Intent   string
	Snippet  string     // 来源 Snippet 及其版本，仅用于留痕
	Declared risk.Level // Snippet 声明的等级
	Strategy Strategy
	Input    *Input // 送进标准输入的凭据或上传文件
}

// Assess 给一条命令在一台主机上判级，没有副作用。
func (k *Kernel) Assess(h inventory.Host, command string, declared risk.Level) (risk.Level, []risk.Finding) {
	v := k.engine.Classify(command)
	lvl, bump := risk.Final(v.Level, declared, h.Sensitivity)
	fs := v.Findings
	if declared > v.Level {
		fs = append(fs, risk.Finding{Level: declared, Rule: "snippet.declared", Reason: "Snippet 声明的等级"})
	}
	if bump != nil {
		fs = append(fs, *bump)
	}
	return lvl, fs
}

func (k *Kernel) refusePaused(ctx context.Context, actor Actor, what string) error {
	if !k.paused.Load() {
		return nil
	}
	k.audit.Append(ctx, audit.Entry{Type: "request.refused", Actor: actor.Name, ActorKind: string(actor.Kind), Entry: actor.Entry, Message: "执行已暂停：" + what})
	return ErrPaused
}

// SubmitCommand 是命令进入内核的唯一入口。
// L0 自动执行；L1/L2 进入等待批准；L3 直接拒绝并留痕。
func (k *Kernel) SubmitCommand(ctx context.Context, actor Actor, spec CommandSpec) (*Request, error) {
	if err := k.refusePaused(ctx, actor, "提交命令"); err != nil {
		return nil, err
	}
	if len(spec.Hosts) == 0 {
		return nil, errors.New("没有选中任何主机")
	}
	if len(spec.Hosts) > k.cfg.MaxHosts {
		return nil, fmt.Errorf("一次最多 %d 台主机", k.cfg.MaxHosts)
	}
	inv := k.Inventory()
	r := &Request{
		ID: newID(), Kind: "command", Intent: spec.Intent, Actor: actor,
		Snippet: spec.Snippet, Declared: spec.Declared, Strategy: spec.Strategy, Input: spec.Input,
		CreatedAt: time.Now(),
	}
	var parts []string
	seen := map[string]bool{}
	for _, name := range spec.Hosts {
		h, ok := inv.Host(name)
		if !ok {
			return nil, fmt.Errorf("主机 %s 不存在", name)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		cmd := spec.Commands[name]
		if cmd == "" {
			return nil, fmt.Errorf("主机 %s 没有要执行的命令", name)
		}
		lvl, fs := k.Assess(h, cmd, spec.Declared)
		r.Targets = append(r.Targets, &Target{Host: name, Sensitivity: h.Sensitivity, Command: cmd, Level: lvl, Findings: fs, State: "waiting"})
		r.Level = risk.Max(r.Level, lvl)
		parts = append(parts, name, cmd)
	}
	parts = append(parts, spec.Input.identity())
	r.Digest = digest(r.Kind, parts...)
	return k.admit(ctx, r)
}

// SubmitFileWrite 是“写文件”进入内核的入口。写文件至少是 L1。
func (k *Kernel) SubmitFileWrite(ctx context.Context, actor Actor, host, path string, content []byte, intent string) (*Request, error) {
	if err := k.refusePaused(ctx, actor, "写文件"); err != nil {
		return nil, err
	}
	if len(content) > 1<<20 {
		return nil, errors.New("文件超过 1 MB 上限")
	}
	h, ok := k.Inventory().Host(host)
	if !ok {
		return nil, fmt.Errorf("主机 %s 不存在", host)
	}
	base, reason := risk.WritePathLevel(path)
	lvl, bump := risk.Final(base, risk.L0, h.Sensitivity)
	fs := []risk.Finding{{Command: "写入 " + path, Level: base, Rule: "file.write", Reason: reason}}
	if bump != nil {
		fs = append(fs, *bump)
	}
	fw := &FileWrite{Path: path, Content: content}
	if lvl < risk.L3 {
		if ex, ok := k.execs[h.Executor]; ok {
			if rl, _ := risk.ReadPathLevel(path); rl == risk.L0 {
				cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				old, err := ex.ReadFile(cctx, h, path, 1<<20)
				cancel()
				if err == nil {
					fw.Old, fw.OldExists = old, true
				}
			}
		}
	}
	sum := sha256.Sum256(content)
	r := &Request{
		ID: newID(), Kind: "file.write", Intent: intent, Actor: actor, CreatedAt: time.Now(),
		Level: lvl, File: fw,
		Targets: []*Target{{Host: host, Sensitivity: h.Sensitivity, Command: "写入 " + path, Level: lvl, Findings: fs, State: "waiting"}},
		Digest:  digest("file.write", host, path, hex.EncodeToString(sum[:])),
	}
	return k.admit(ctx, r)
}

// admit 完成提交：留痕，然后拒绝、自动执行或进入等待批准。
func (k *Kernel) admit(ctx context.Context, r *Request) (*Request, error) {
	var all []risk.Finding
	hosts := make([]string, len(r.Targets))
	for i, t := range r.Targets {
		all = append(all, t.Findings...)
		hosts[i] = t.Host
	}
	if len(r.Targets) == 1 {
		r.Confirm = r.Targets[0].Host
	} else {
		r.Confirm = strconv.Itoa(len(r.Targets))
	}
	entry := audit.Entry{
		Actor: r.Actor.Name, ActorKind: string(r.Actor.Kind), Entry: r.Actor.Entry,
		RequestID: r.ID, Intent: r.Intent, Command: r.Targets[0].Command, Level: r.Level.String(),
		Reasons: reasons(all), Snippet: r.Snippet,
		Detail: map[string]any{"hosts": hosts, "digest": r.Digest, "kind": r.Kind},
	}
	if r.Level >= risk.L3 {
		entry.Type = "request.rejected"
		r.State = StateRejected
		now := time.Now()
		r.FinishedAt = &now
	} else {
		entry.Type = "request.submitted"
	}
	seq, err := k.audit.Append(ctx, entry)
	if err != nil {
		return nil, fmt.Errorf("写审计失败，操作中止：%w", err)
	}
	r.AuditSeq = seq

	k.mu.Lock()
	defer k.mu.Unlock()
	k.reqs[r.ID] = r
	k.order = append(k.order, r.ID)
	switch {
	case r.State == StateRejected:
		k.changed(r)
	case r.Level == risk.L0:
		k.start(r)
	default:
		r.State = StatePending
		exp := time.Now().Add(k.cfg.PendingTTL)
		r.ExpiresAt = &exp
		k.changed(r)
	}
	return clone(r, true), nil
}

// ---------- 审批 ----------

// Approve 批准一个等待中的请求。批准绑定到请求内容的摘要。
func (k *Kernel) Approve(ctx context.Context, id string, approver Actor, proof Proof) (*Request, error) {
	if k.paused.Load() {
		return nil, ErrPaused
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.reqs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if r.State != StatePending {
		return nil, ErrNotPending
	}
	if r.ExpiresAt != nil && time.Now().After(*r.ExpiresAt) {
		k.expire(r)
		return nil, ErrExpired
	}
	deny := func(e error) (*Request, error) {
		k.audit.Append(ctx, audit.Entry{Type: "approval.refused", Actor: approver.Name, ActorKind: string(approver.Kind),
			Entry: approver.Entry, RequestID: r.ID, Level: r.Level.String(), Message: e.Error()})
		return nil, e
	}
	switch {
	case r.Level >= risk.L3:
		return deny(ErrForbidden)
	case approver.Kind != Human:
		return deny(ErrHumanOnly)
	case !approver.CanApprove || approver.MaxLevel < r.Level:
		return deny(ErrNotAllowed)
	case subtle.ConstantTimeCompare([]byte(proof.Digest), []byte(r.Digest)) != 1:
		return deny(ErrDigest)
	}
	if r.Level == risk.L2 {
		switch {
		case approver.Entry != "web":
			return deny(ErrL2WebOnly)
		case !proof.Reauthed:
			return deny(ErrReauth)
		case proof.Typed != r.Confirm:
			return deny(ErrConfirm)
		case k.cfg.TwoPersonL2 && approver.ID == r.Actor.ID:
			return deny(ErrSelfApproval)
		}
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: "approval.granted", Actor: r.Actor.Name, Approver: approver.Name,
		Entry: approver.Entry, RequestID: r.ID, Level: r.Level.String(),
		Detail: map[string]any{"digest": r.Digest, "reauth": proof.Reauthed && !proof.DevBypass, "devBypass": proof.DevBypass}}); err != nil {
		return nil, fmt.Errorf("写审计失败，操作中止：%w", err)
	}
	now := time.Now()
	r.DecidedAt = &now
	r.Approvals = append(r.Approvals, Approval{By: approver.Name, ByID: approver.ID, At: now, Reauth: proof.Reauthed && !proof.DevBypass, DevBypass: proof.DevBypass})
	k.start(r)
	return clone(r, true), nil
}

// Deny 拒绝一个等待中的请求。
func (k *Kernel) Deny(ctx context.Context, id string, by Actor, reason string) (*Request, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.reqs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if r.State != StatePending {
		return nil, ErrNotPending
	}
	if by.Kind != Human {
		return nil, ErrHumanOnly
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: "approval.denied", Actor: r.Actor.Name, Approver: by.Name, Entry: by.Entry,
		RequestID: r.ID, Level: r.Level.String(), Message: reason}); err != nil {
		return nil, err
	}
	now := time.Now()
	r.State, r.DecidedAt, r.FinishedAt = StateDenied, &now, &now
	r.DeniedBy, r.DenyReason = by.Name, reason
	k.changed(r)
	return clone(r, true), nil
}

// Cancel 取消等待中或执行中的请求。
func (k *Kernel) Cancel(ctx context.Context, id string, by Actor) (*Request, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.reqs[id]
	if !ok {
		return nil, ErrNotFound
	}
	if r.State.terminal() {
		return clone(r, true), nil
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: "request.cancelled", Actor: by.Name, Entry: by.Entry, RequestID: r.ID}); err != nil {
		// 写不进审计就不取消（审计失败时操作必须中止）。
		return nil, fmt.Errorf("写审计失败，没有取消：%w", err)
	}
	if r.State == StateRunning {
		if cancel, ok := k.cancels[id]; ok {
			go cancel() // 取消函数可能要拿 k.mu（关终端时），不能在持锁时直接调
			return clone(r, true), nil
		}
		if r.Kind != "terminal" {
			return clone(r, true), nil
		}
		// 批准了但还没连上的终端：直接作废。
	}
	now := time.Now()
	r.State, r.FinishedAt = StateCancelled, &now
	k.changed(r)
	return clone(r, true), nil
}

func (k *Kernel) expire(r *Request) {
	now := time.Now()
	r.State, r.FinishedAt = StateExpired, &now
	if _, err := k.audit.Append(context.Background(), audit.Entry{Type: "request.expired", RequestID: r.ID, Level: r.Level.String()}); err != nil {
		slog.Error("写审计失败：请求过期没有记下来", "request", r.ID, "err", err)
	}
	k.changed(r)
}

func (k *Kernel) janitor() {
	for range time.Tick(20 * time.Second) {
		k.mu.Lock()
		for _, r := range k.reqs {
			if r.State == StatePending && r.ExpiresAt != nil && time.Now().After(*r.ExpiresAt) {
				k.expire(r)
			}
			// 批准了却一直没连上的终端请求。
			if r.Kind == "terminal" && r.State == StateRunning && r.Terminal == nil && r.ExpiresAt != nil && time.Now().After(*r.ExpiresAt) {
				k.expire(r)
			}
		}
		k.cleanUploads()
		// 内存里只保留最近的请求；更早的在数据库里。
		for len(k.order) > 500 {
			if r := k.reqs[k.order[0]]; r != nil && !r.State.terminal() {
				break
			}
			delete(k.reqs, k.order[0])
			k.order = k.order[1:]
		}
		k.mu.Unlock()
	}
}

// ---------- 紧急开关 ----------

// Paused 报告是否已暂停全部执行。
func (k *Kernel) Paused() bool { return k.paused.Load() }

// SetPaused 暂停或恢复全部执行。暂停时取消所有正在执行的请求。
func (k *Kernel) SetPaused(ctx context.Context, by Actor, paused bool) error {
	typ, val := "system.resumed", "0"
	if paused {
		typ, val = "system.paused", "1"
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: typ, Actor: by.Name, Entry: by.Entry}); err != nil {
		return err
	}
	if _, err := k.db.Exec(`INSERT INTO settings (key, value) VALUES ('paused', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, val); err != nil {
		return err
	}
	k.paused.Store(paused)
	if paused {
		k.mu.Lock()
		for _, cancel := range k.cancels {
			go cancel() // 同上：不在持锁时直接调
		}
		k.mu.Unlock()
	}
	k.Publish(Event{Type: "system", Data: map[string]any{"paused": paused}})
	return nil
}

// AuditRoutine 报告日常查看是否写审计。
func (k *Kernel) AuditRoutine() bool { return k.auditRoutine.Load() }

// SetAuditRoutine 设置日常查看（翻目录、页面上的只读查询）要不要写审计。
// 这是审计写入上唯一的开关：它只管 audit.Routine 里的那两类只读记录，改动本身一定写审计；
// 读文件内容、下载、上传、执行、批准、登录、改设置这些，无论它开还是关都照常记录。
func (k *Kernel) SetAuditRoutine(ctx context.Context, by Actor, on bool) error {
	msg, val := "日常查看不再写入审计", "0"
	if on {
		msg, val = "日常查看开始写入审计", "1"
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: "system.audit.routine", Actor: by.Name, Entry: by.Entry, Message: msg,
		Detail: map[string]any{"record": on, "types": audit.Routine}}); err != nil {
		return err
	}
	if _, err := k.db.Exec(`INSERT INTO settings (key, value) VALUES ('audit.routine', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, val); err != nil {
		return err
	}
	k.auditRoutine.Store(on)
	return nil
}

// routine 写一条日常查看的审计；设置为不记录时什么都不做。
func (k *Kernel) routine(ctx context.Context, e audit.Entry) error {
	if !k.auditRoutine.Load() {
		return nil
	}
	_, err := k.audit.Append(ctx, e)
	return err
}

// sortForRun 把高敏感主机排到最后。
func sortForRun(ts []*Target) []*Target {
	out := append([]*Target(nil), ts...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Sensitivity != risk.SensHigh && out[j].Sensitivity == risk.SensHigh
	})
	return out
}

// VerifyAudit 校验审计哈希链。
func (k *Kernel) VerifyAudit(ctx context.Context) (audit.VerifyResult, error) {
	return audit.Verify(ctx, k.db)
}
