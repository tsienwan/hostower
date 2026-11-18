package kernel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// start 开始执行一个已获放行的请求。调用方持有 k.mu。
// 只有两条路能走到这里：L0 自动放行，或 Approve 通过。
func (k *Kernel) start(r *Request) {
	if r.Kind == "terminal" {
		// 终端不是“跑完就结束”的命令：批准后等发起人来连，过时不连就作废。
		r.State = StateRunning
		exp := time.Now().Add(terminalAttachTTL)
		r.ExpiresAt = &exp
		k.changed(r)
		return
	}
	r.State = StateRunning
	ctx, cancel := context.WithCancel(context.Background())
	k.cancels[r.ID] = cancel
	k.changed(r)
	go k.run(ctx, r)
}

func (k *Kernel) run(ctx context.Context, r *Request) {
	defer func() {
		k.mu.Lock()
		if cancel, ok := k.cancels[r.ID]; ok {
			cancel()
			delete(k.cancels, r.ID)
		}
		k.mu.Unlock()
	}()

	targets := sortForRun(r.Targets)
	conc := r.Strategy.Concurrency
	if conc <= 0 {
		conc = 10
	}
	// 组里有高敏感主机时强制滚动。
	if len(targets) > 1 {
		for _, t := range targets {
			if t.Sensitivity == risk.SensHigh {
				conc = 1
				break
			}
		}
	}

	var stop sync.Once
	stopped := make(chan struct{})
	halt := func() { stop.Do(func() { close(stopped) }) }
	isStopped := func() bool {
		select {
		case <-stopped:
			return true
		case <-ctx.Done():
			return true
		default:
			return false
		}
	}
	runOne := func(t *Target) {
		if isStopped() {
			k.mu.Lock()
			t.State = "skipped"
			k.mu.Unlock()
			return
		}
		ok := k.runTarget(ctx, r, t)
		if !ok && r.Strategy.StopOnError {
			halt()
		}
	}

	rest := targets
	if r.Strategy.Canary && len(targets) > 1 {
		// 先在一台上预演：失败就停，其余主机不动。
		first := targets[0]
		if !k.runTarget(ctx, r, first) {
			halt()
		}
		rest = targets[1:]
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for _, t := range rest {
		sem <- struct{}{}
		wg.Add(1)
		go func(t *Target) {
			defer wg.Done()
			defer func() { <-sem }()
			runOne(t)
		}(t)
	}
	wg.Wait()

	k.mu.Lock()
	defer k.mu.Unlock()
	okN, failN, skipN := 0, 0, 0
	for _, t := range r.Targets {
		switch t.State {
		case "ok":
			okN++
		case "skipped", "waiting":
			t.State = "skipped"
			skipN++
		default:
			failN++
		}
	}
	now := time.Now()
	r.FinishedAt = &now
	switch {
	case ctx.Err() != nil:
		r.State = StateCancelled
	case failN > 0 || skipN > 0:
		r.State = StateFailed
	default:
		r.State = StateDone
	}
	if len(r.Targets) > 1 {
		k.audit.Append(context.Background(), audit.Entry{Type: "exec.finished", Actor: r.Actor.Name, RequestID: r.ID, Parent: r.AuditSeq,
			Level: r.Level.String(), Detail: map[string]any{"ok": okN, "failed": failN, "skipped": skipN, "state": r.State}})
	}
	k.changed(r)
}

// capture 收集一台主机的输出：有上限，边产生边广播。
type capture struct {
	k      *Kernel
	r      *Request
	t      *Target
	max    int
	remain []byte // 不完整的 UTF-8 尾巴
}

func (c *capture) Write(p []byte) (int, error) {
	n := len(p)
	c.k.mu.Lock()
	c.t.OutputBytes += int64(n)
	room := c.max - len(c.t.Output)
	if room <= 0 {
		c.t.Truncated = true
		c.k.mu.Unlock()
		return n, nil
	}
	data := append(c.remain, p...)
	// 不在多字节字符中间切开。
	cut := len(data)
	for cut > 0 && cut > len(data)-utf8.UTFMax && !utf8.Valid(data[:cut]) {
		cut--
	}
	if !utf8.Valid(data[:cut]) {
		cut = len(data)
	}
	c.remain = append([]byte(nil), data[cut:]...)
	chunk := strings.ToValidUTF8(string(data[:cut]), "�")
	if len(chunk) > room {
		chunk = strings.ToValidUTF8(chunk[:room], "")
		c.t.Truncated = true
	}
	c.t.Output += chunk
	c.k.mu.Unlock()
	if chunk != "" {
		c.k.Publish(Event{Type: "output", Data: OutputChunk{ID: c.r.ID, Host: c.t.Host, Chunk: chunk}})
	}
	return n, nil
}

// runTarget 在一台主机上执行，并写一条审计子记录。返回是否成功。
func (k *Kernel) runTarget(ctx context.Context, r *Request, t *Target) bool {
	now := time.Now()
	k.mu.Lock()
	t.State, t.StartedAt = "running", &now
	k.Publish(Event{Type: "request", Data: clone(r, false)})
	k.mu.Unlock()

	var (
		code    int
		err     error
		backup  string
		summary string
	)
	h, ok := k.Inventory().Host(t.Host)
	ex := k.execs[h.Executor]
	switch {
	case !ok:
		err = fmt.Errorf("主机 %s 已不在清单里", t.Host)
	case ex == nil:
		err = fmt.Errorf("执行器 %q 不可用", h.Executor)
	case k.paused.Load():
		err = ErrPaused
	case r.Kind == "file.write":
		backup, err = k.writeFile(ctx, ex, h, r)
	default:
		cctx, cancel := context.WithTimeout(ctx, k.cfg.CommandTimeout)
		w := &capture{k: k, r: r, t: t, max: k.cfg.MaxOutput}
		if r.Input == nil {
			code, err = ex.Exec(cctx, h, t.Command, w)
		} else if se, ok := ex.(executor.StdinExecutor); !ok {
			err = fmt.Errorf("执行器 %q 不支持标准输入", h.Executor)
		} else if in, ierr := k.openInput(r.Input); ierr != nil {
			err = ierr
		} else {
			code, err = se.ExecStdin(cctx, h, t.Command, in, w)
			in.Close()
		}
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("超过 %s 未结束，已终止", k.cfg.CommandTimeout)
		}
		cancel()
	}

	end := time.Now()
	k.mu.Lock()
	t.EndedAt = &end
	t.Backup = backup
	switch {
	case err != nil:
		t.State, t.Error = "error", err.Error()
		if errors.Is(err, context.Canceled) {
			t.Error = "已取消"
		}
	case code != 0:
		t.State = "failed"
		t.ExitCode = &code
	default:
		t.State = "ok"
		t.ExitCode = &code
	}
	summary = Summarize(t.Output)
	state := t.State
	k.mu.Unlock()

	entry := audit.Entry{
		Type: "exec.host", Actor: r.Actor.Name, ActorKind: string(r.Actor.Kind), Entry: r.Actor.Entry,
		RequestID: r.ID, Parent: r.AuditSeq, Host: t.Host, Intent: r.Intent, Command: t.Command,
		Level: t.Level.String(), Reasons: reasons(t.Findings), Output: summary, Backup: backup, Snippet: r.Snippet,
	}
	if len(r.Approvals) > 0 {
		entry.Approver = r.Approvals[len(r.Approvals)-1].By
	}
	if err != nil {
		entry.Message = t.Error
	} else {
		entry.ExitCode = &code
	}
	seq, _ := k.audit.Append(context.Background(), entry)

	k.mu.Lock()
	t.AuditSeq = seq
	k.changed(r)
	k.mu.Unlock()
	return state == "ok"
}

// writeFile 先把旧内容备份到数据目录，再写新内容。
func (k *Kernel) writeFile(ctx context.Context, ex executor.Executor, h inventory.Host, r *Request) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	backup := ""
	if rl, _ := risk.ReadPathLevel(r.File.Path); rl == risk.L0 {
		old, err := ex.ReadFile(cctx, h, r.File.Path, 8<<20)
		switch {
		case err == nil:
			// 批准时看到的 diff 是对着旧内容算的；旧内容变了就不能照旧批准执行。
			if r.File.OldExists && !bytes.Equal(old, r.File.Old) {
				return "", errors.New("文件在提交之后被改动过，批准作废，请重新提交")
			}
			dir := filepath.Join(k.cfg.DataDir, "backups", r.ID, h.Name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return "", fmt.Errorf("无法备份旧文件：%w", err)
			}
			backup = filepath.Join(dir, path.Base(r.File.Path))
			if err := os.WriteFile(backup, old, 0o600); err != nil {
				return "", fmt.Errorf("无法备份旧文件：%w", err)
			}
		case errors.Is(err, executor.ErrNotExist):
			if r.File.OldExists {
				return "", errors.New("文件在提交之后被删除，批准作废，请重新提交")
			}
		default:
			return "", fmt.Errorf("无法读取旧文件做备份：%w", err)
		}
	}
	return backup, ex.WriteFile(cctx, h, r.File.Path, r.File.Content)
}

// ---------- 只读文件操作（L0，直接执行并留痕） ----------

// ListDir 列目录。
func (k *Kernel) ListDir(ctx context.Context, actor Actor, host, dir string) ([]executor.FileInfo, error) {
	h, ex, err := k.readable(ctx, actor, host, "列目录")
	if err != nil {
		return nil, err
	}
	if err := k.routine(ctx, audit.Entry{Type: "file.list", Actor: actor.Name, ActorKind: string(actor.Kind), Entry: actor.Entry,
		Host: host, Command: "列目录 " + dir, Level: "L0"}); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return ex.ListDir(cctx, h, dir)
}

// ReadFile 读文件（最多 1 MB，给页面上查看用）。私钥和凭据文件是 L3，直接拒绝。
func (k *Kernel) ReadFile(ctx context.Context, actor Actor, host, p string) ([]byte, error) {
	return k.readFile(ctx, actor, host, p, "file.read", 1<<20, 20*time.Second)
}

// DownloadLimit 是页面上下载单个文件的上限：文件要整个读进内存再发给浏览器。
const DownloadLimit = 100 << 20

// DownloadFile 读出整个文件用于下载。和 ReadFile 是同一条路、同一套 L3 规则，只是上限更大；超过上限返回 ErrTooLarge。
func (k *Kernel) DownloadFile(ctx context.Context, actor Actor, host, p string) ([]byte, error) {
	data, err := k.readFile(ctx, actor, host, p, "file.download", DownloadLimit+1, 5*time.Minute)
	if err == nil && len(data) > DownloadLimit {
		return nil, ErrTooLarge
	}
	return data, err
}

// PreviewLimit 是页面上预览单个文件（图片、PDF、Office 文档）的上限。
const PreviewLimit = 20 << 20

// PreviewFile 读出整个文件用于在页面上预览。它就是“读文件”，审计里也记为读文件；同样的 L3 规则。
func (k *Kernel) PreviewFile(ctx context.Context, actor Actor, host, p string) ([]byte, error) {
	data, err := k.readFile(ctx, actor, host, p, "file.read", PreviewLimit+1, 2*time.Minute)
	if err == nil && len(data) > PreviewLimit {
		return nil, ErrTooLarge
	}
	return data, err
}

func (k *Kernel) readFile(ctx context.Context, actor Actor, host, p, typ string, limit int64, timeout time.Duration) ([]byte, error) {
	h, ex, err := k.readable(ctx, actor, host, "读文件")
	if err != nil {
		return nil, err
	}
	lvl, reason := risk.ReadPathLevel(p)
	entry := audit.Entry{Type: typ, Actor: actor.Name, ActorKind: string(actor.Kind), Entry: actor.Entry,
		Host: host, Command: "读取 " + p, Level: lvl.String()}
	if lvl >= risk.L3 {
		entry.Type, entry.Reasons = "request.rejected", []string{"L3 " + reason}
		k.audit.Append(ctx, entry)
		return nil, ErrForbidden
	}
	if _, err := k.audit.Append(ctx, entry); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return ex.ReadFile(cctx, h, p, limit)
}

func (k *Kernel) readable(ctx context.Context, actor Actor, host, what string) (inventory.Host, executor.Executor, error) {
	if err := k.refusePaused(ctx, actor, what); err != nil {
		return inventory.Host{}, nil, err
	}
	h, ok := k.Inventory().Host(host)
	if !ok {
		return h, nil, fmt.Errorf("主机 %s 不存在", host)
	}
	ex := k.execs[h.Executor]
	if ex == nil {
		return h, nil, fmt.Errorf("执行器 %q 不可用", h.Executor)
	}
	return h, ex, nil
}

// Probe 运行固定的只读采样命令。命令是编译期常量，启动时已确认是 L0；
// 采样本身不逐次写审计（否则每台主机每分钟都会刷出记录），采样器的启停会留痕。
func (k *Kernel) Probe(ctx context.Context, host string) (string, error) {
	return k.probe(ctx, host, executor.ProbeCommand)
}

// ProbeStream 持续运行固定的只读采样脚本，把它的输出原样交给 out，直到 ctx 结束、脚本退出或执行被暂停。
// 它和 Probe 是同一类例外：脚本是编译期常量，启动时已确认是 L0；不逐次写审计。
// “暂停全部执行”会让它立刻停下，暂停期间也启动不了。
func (k *Kernel) ProbeStream(ctx context.Context, host string, out io.Writer) error {
	if k.paused.Load() {
		return ErrPaused
	}
	h, ok := k.Inventory().Host(host)
	if !ok {
		return fmt.Errorf("主机 %s 不存在", host)
	}
	ex := k.execs[h.Executor]
	if ex == nil {
		return fmt.Errorf("执行器 %q 不可用", h.Executor)
	}
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var paused atomic.Bool
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-cctx.Done():
				return
			case <-t.C:
				if k.paused.Load() {
					paused.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	code, err := ex.Exec(cctx, h, executor.StreamProbeCommand, out)
	if paused.Load() {
		return ErrPaused
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("采样脚本退出了（退出码 %d）", code)
}

// ProbeContainers 运行固定的容器占用采样命令。
func (k *Kernel) ProbeContainers(ctx context.Context, host string) (string, error) {
	return k.probe(ctx, host, executor.ContainerProbeCommand)
}

// ProbeHardware 运行固定的硬件信息采集命令（CPU 型号、整机厂家型号、磁盘）。
// 和 Probe 是同一类例外：编译期常量，启动时已确认是 L0，不逐次写审计；采样器只在连上主机时和每 6 小时跑一次。
func (k *Kernel) ProbeHardware(ctx context.Context, host string) (string, error) {
	return k.probe(ctx, host, executor.HardwareProbeCommand)
}

func (k *Kernel) probe(ctx context.Context, host, command string) (string, error) {
	if k.paused.Load() {
		return "", ErrPaused
	}
	h, ok := k.Inventory().Host(host)
	if !ok {
		return "", fmt.Errorf("主机 %s 不存在", host)
	}
	ex := k.execs[h.Executor]
	if ex == nil {
		return "", fmt.Errorf("执行器 %q 不可用", h.Executor)
	}
	var buf bytes.Buffer
	code, err := ex.Exec(ctx, h, command, &limitWriter{w: &buf, n: 128 << 10})
	if err != nil {
		return "", err
	}
	if code != 0 && buf.Len() == 0 {
		return "", fmt.Errorf("采样命令退出码 %d", code)
	}
	return buf.String(), nil
}

type limitWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if room := l.n - l.w.Len(); room > 0 {
		if len(p) > room {
			l.w.Write(p[:room])
		} else {
			l.w.Write(p)
		}
	}
	return len(p), nil
}

// Reconnect 断开到这台主机的连接，下一个会话重新登录；采样器收到事件后立刻重连采样。
// 它不在主机上执行任何东西，只是让登录账号的新权限（比如刚加进 docker 组）生效。
// 只对人开放，写审计；断开时这台主机上正在跑的命令会中断。
func (k *Kernel) Reconnect(ctx context.Context, actor Actor, host string) error {
	if actor.Kind != Human {
		return fmt.Errorf("%w：只有人可以重新连接主机", ErrNotAllowed)
	}
	h, ex, err := k.readable(ctx, actor, host, "重新连接")
	if err != nil {
		return err
	}
	rc, ok := ex.(executor.Reconnecter)
	if !ok {
		return fmt.Errorf("%w：这台主机不需要重新连接", ErrNotAllowed)
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: "host.reconnected", Actor: actor.Name, ActorKind: string(actor.Kind), Entry: actor.Entry, Host: h.Name}); err != nil {
		return err
	}
	rc.Reconnect(h.Name)
	k.Publish(Event{Type: "reconnected", Data: h.Name})
	return nil
}
