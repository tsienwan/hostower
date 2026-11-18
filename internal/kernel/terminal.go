package kernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 交互终端。
//
// 终端里敲的命令没法逐条判级——塔台看到的只是字节流。所以内核管的是“开终端”这个动作：
//   - 固定按 L2（高危）处理：当场确认，正式环境要重新验证 Passkey；
//   - 只有人能开，AI、令牌、插件都不行；
//   - 整段输出录制下来，开始和结束都写审计；
//   - “暂停全部执行”会立刻断开所有终端。
// 它是内核里唯一不逐条判级的路径，这一点写在威胁模型里。

// TerminalInfo 是一次终端会话的记录。
type TerminalInfo struct {
	AttachedAt time.Time  `json:"attachedAt"`
	ClosedAt   *time.Time `json:"closedAt,omitempty"`
	Bytes      int64      `json:"bytes"`
	Recording  string     `json:"recording"`
	Truncated  bool       `json:"truncated,omitempty"`
}

const (
	terminalAttachTTL = 2 * time.Minute // 批准之后要在这段时间里连上
	recordingLimit    = 64 << 20        // 单次录像的上限
)

// SubmitTerminal 请求在一台主机上开交互终端。
func (k *Kernel) SubmitTerminal(ctx context.Context, actor Actor, host string) (*Request, error) {
	if err := k.refusePaused(ctx, actor, "打开终端"); err != nil {
		return nil, err
	}
	if actor.Kind != Human {
		return nil, fmt.Errorf("%w：交互终端只对人开放", ErrHumanOnly)
	}
	h, ok := k.Inventory().Host(host)
	if !ok {
		return nil, fmt.Errorf("主机 %s 不存在", host)
	}
	if _, ok := k.execs[h.Executor].(executor.ShellExecutor); !ok {
		return nil, executor.ErrNoShell
	}
	r := &Request{
		ID: newID(), Kind: "terminal", Intent: "打开交互终端", Actor: actor, CreatedAt: time.Now(), Level: risk.L2,
		Targets: []*Target{{Host: host, Sensitivity: h.Sensitivity, Command: "交互终端（整段录制）", Level: risk.L2, State: "waiting",
			Findings: []risk.Finding{{Level: risk.L2, Rule: "terminal.interactive", Reason: "交互终端里的命令无法逐条判级"}}}},
		Digest: digest("terminal", host),
	}
	return k.admit(ctx, r)
}

// TerminalSession 是一个已经连上的终端：读写都经过这里，输出被录下来。
type TerminalSession struct {
	k     *Kernel
	r     *Request
	sh    executor.ShellSession
	file  *os.File
	start time.Time
	mu    sync.Mutex
	bytes int64
	full  bool
	once  sync.Once
}

// AttachTerminal 连上一个已获批准的终端请求。只有发起人自己能连，而且只能连一次。
func (k *Kernel) AttachTerminal(ctx context.Context, id string, actor Actor, cols, rows int) (*TerminalSession, error) {
	if k.paused.Load() {
		return nil, ErrPaused
	}
	k.mu.Lock()
	r, ok := k.reqs[id]
	switch {
	case !ok || r.Kind != "terminal":
		k.mu.Unlock()
		return nil, ErrNotFound
	case actor.Kind != Human || actor.ID != r.Actor.ID:
		k.mu.Unlock()
		return nil, ErrNotAllowed
	case r.State != StateRunning || r.Terminal != nil:
		k.mu.Unlock()
		return nil, errors.New("这个终端请求已经用过或还没有批准")
	case r.ExpiresAt != nil && time.Now().After(*r.ExpiresAt):
		k.expire(r)
		k.mu.Unlock()
		return nil, ErrExpired
	}
	host := r.Targets[0].Host
	r.Terminal = &TerminalInfo{AttachedAt: time.Now(), Recording: r.ID + ".cast"}
	r.ExpiresAt = nil
	now := time.Now()
	r.Targets[0].State, r.Targets[0].StartedAt = "running", &now
	k.changed(r)
	k.mu.Unlock()

	fail := func(err error) (*TerminalSession, error) {
		k.finishTerminal(r, 0, false, err)
		return nil, err
	}
	h, ok := k.Inventory().Host(host)
	sx, okx := k.execs[h.Executor].(executor.ShellExecutor)
	if !ok || !okx {
		return fail(executor.ErrNoShell)
	}
	dir := filepath.Join(k.cfg.DataDir, "recordings")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fail(fmt.Errorf("无法创建录像目录，终端不开：%w", err))
	}
	f, err := os.OpenFile(filepath.Join(dir, r.Terminal.Recording), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return fail(fmt.Errorf("无法创建录像文件，终端不开：%w", err))
	}
	if _, err := k.audit.Append(ctx, audit.Entry{Type: "terminal.opened", Actor: actor.Name, ActorKind: string(actor.Kind), Entry: actor.Entry,
		RequestID: r.ID, Host: host, Level: "L2", Detail: map[string]any{"recording": r.Terminal.Recording}}); err != nil {
		f.Close()
		return fail(fmt.Errorf("写审计失败，终端不开：%w", err))
	}
	sh, err := sx.Shell(ctx, h, cols, rows)
	if err != nil {
		f.Close()
		return fail(err)
	}
	// 录像用 asciicast v2 格式：第一行是头，之后每行一个 [秒, "o", 输出]。
	head, _ := json.Marshal(map[string]any{"version": 2, "width": cols, "height": rows, "timestamp": time.Now().Unix(), "title": host})
	f.Write(append(head, '\n'))
	s := &TerminalSession{k: k, r: r, sh: sh, file: f, start: time.Now()}
	k.mu.Lock()
	k.cancels[r.ID] = func() { s.Close() }
	k.mu.Unlock()
	return s, nil
}

// Read 读终端输出，并把它记进录像。
func (s *TerminalSession) Read(p []byte) (int, error) {
	n, err := s.sh.Read(p)
	if n > 0 {
		s.mu.Lock()
		s.bytes += int64(n)
		if !s.full {
			line, _ := json.Marshal([]any{time.Since(s.start).Seconds(), "o", string(p[:n])})
			if s.bytes > recordingLimit {
				s.full = true
				line, _ = json.Marshal([]any{time.Since(s.start).Seconds(), "o", "\r\n[录像达到上限，之后的输出没有录制]\r\n"})
			}
			s.file.Write(append(line, '\n'))
		}
		s.mu.Unlock()
	}
	return n, err
}

// Write 把键盘输入送进终端。输入本身不单独录制（它可能含口令）；回显会出现在输出里。
func (s *TerminalSession) Write(p []byte) (int, error) { return s.sh.Write(p) }

// Resize 调整终端大小。
func (s *TerminalSession) Resize(cols, rows int) error { return s.sh.Resize(cols, rows) }

// Close 结束会话：关终端、收尾录像、写审计。可以重复调用。
func (s *TerminalSession) Close() error {
	s.once.Do(func() {
		s.sh.Close()
		s.mu.Lock()
		s.file.Close()
		bytes, full := s.bytes, s.full
		s.mu.Unlock()
		s.k.finishTerminal(s.r, bytes, full, nil)
	})
	return nil
}

func (k *Kernel) finishTerminal(r *Request, bytes int64, truncated bool, cause error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.cancels, r.ID)
	now := time.Now()
	t := r.Targets[0]
	t.EndedAt, r.FinishedAt = &now, &now
	r.Terminal.ClosedAt, r.Terminal.Bytes, r.Terminal.Truncated = &now, bytes, truncated
	entry := audit.Entry{Type: "terminal.closed", Actor: r.Actor.Name, RequestID: r.ID, Host: t.Host, Level: "L2",
		Detail: map[string]any{"seconds": int(now.Sub(r.Terminal.AttachedAt).Seconds()), "bytes": bytes, "recording": r.Terminal.Recording}}
	if cause != nil {
		t.State, t.Error, r.State = "error", cause.Error(), StateFailed
		entry.Message = cause.Error()
	} else {
		t.State, r.State = "ok", StateDone
	}
	seq, _ := k.audit.Append(context.Background(), entry)
	t.AuditSeq = seq
	k.changed(r)
}

// RecordingPath 返回一次终端会话的录像文件位置。
func (k *Kernel) RecordingPath(id string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, ok := k.reqs[id]
	if !ok || r.Kind != "terminal" || r.Terminal == nil {
		return "", ErrNotFound
	}
	return filepath.Join(k.cfg.DataDir, "recordings", r.Terminal.Recording), nil
}
