// Package kernel 是安全内核：判级 → 审批 → 执行 → 审计。
// 它是整个系统里唯一能触达主机的路径。这里没有扩展点，也没有后门接口：
// 任何入口（网页、API、聊天、MCP、插件、定时任务）的任何操作都从 Submit* 进来。
package kernel

import (
	"errors"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// ActorKind 区分发起人的种类。只有 human 能批准。
type ActorKind string

const (
	Human  ActorKind = "human"
	AI     ActorKind = "ai"
	Token  ActorKind = "token"
	Plugin ActorKind = "plugin"
	System ActorKind = "system"
)

// Actor 是一次操作的发起人或批准人。它由网关在认证之后构造，调用方无法自报。
type Actor struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Kind     ActorKind  `json:"kind"`
	Entry    string     `json:"entry"` // web | api | cli | chat | mcp | plugin | schedule
	MaxLevel risk.Level `json:"maxLevel"`
	// CanApprove 为 false 的角色（操作员）只能发起，不能批准。
	CanApprove bool `json:"canApprove"`
}

// State 是请求的状态。
type State string

const (
	StatePending     State = "pending"     // 等待批准
	StateRunning     State = "running"     // 执行中
	StateDone        State = "done"        // 全部成功
	StateFailed      State = "failed"      // 有主机失败
	StateDenied      State = "denied"      // 被人拒绝
	StateRejected    State = "rejected"    // L3，被内核拒绝
	StateExpired     State = "expired"     // 超时无人批准
	StateCancelled   State = "cancelled"   // 被取消
	StateInterrupted State = "interrupted" // 塔台重启时中断
)

func (s State) terminal() bool {
	return s != StatePending && s != StateRunning
}

// Target 是请求在一台主机上的那一份：渲染后的实际命令、判级和执行结果。
type Target struct {
	Host        string           `json:"host"`
	Sensitivity risk.Sensitivity `json:"sensitivity"`
	Command     string           `json:"command"`
	Level       risk.Level       `json:"level"`
	Findings    []risk.Finding   `json:"findings"`

	State       string     `json:"state"` // waiting | running | ok | failed | error | skipped
	ExitCode    *int       `json:"exitCode,omitempty"`
	Error       string     `json:"error,omitempty"`
	Output      string     `json:"output"`
	OutputBytes int64      `json:"outputBytes"`
	Truncated   bool       `json:"truncated,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	AuditSeq    int64      `json:"auditSeq,omitempty"`
	Backup      string     `json:"backup,omitempty"`
}

// Strategy 是批量执行的方式。
type Strategy struct {
	Concurrency int  `json:"concurrency"`
	Canary      bool `json:"canary"`      // 先在第一台预演，失败就停
	StopOnError bool `json:"stopOnError"` // 遇错停止
}

// FileWrite 是“写文件”请求的内容。
type FileWrite struct {
	Path      string `json:"path"`
	Content   []byte `json:"content"`
	Old       []byte `json:"old,omitempty"`
	OldExists bool   `json:"oldExists"`
}

// Approval 是一次批准。
type Approval struct {
	By     string    `json:"by"`
	ByID   string    `json:"byId"`
	At     time.Time `json:"at"`
	Reauth bool      `json:"reauth"`
	// DevBypass 表示这次批准发生在开发模式的令牌会话里，没有经过 Passkey 二次验证。
	DevBypass bool `json:"devBypass,omitempty"`
}

// Request 是一次操作请求，从提交到结束的完整状态。
type Request struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"` // command | file.write
	Intent   string     `json:"intent"`
	Actor    Actor      `json:"actor"`
	Snippet  string     `json:"snippet,omitempty"`
	Declared risk.Level `json:"declared"`
	Level    risk.Level `json:"level"`
	// Digest 绑定“批准的到底是什么”：命令或文件内容变了，批准作废。
	Digest   string     `json:"digest"`
	Strategy Strategy   `json:"strategy"`
	Targets  []*Target  `json:"targets"`
	File     *FileWrite `json:"file,omitempty"`
	// Input 是随命令送进标准输入的凭据或上传文件（只有引用，没有内容）。
	Input *Input `json:"input,omitempty"`
	// Terminal 是交互终端会话的记录（只有 kind 是 terminal 的请求才有）。
	Terminal *TerminalInfo `json:"terminal,omitempty"`
	// Confirm 是 L2 批准时需要手动输入的内容。
	Confirm string `json:"confirm,omitempty"`

	State      State      `json:"state"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	DecidedAt  *time.Time `json:"decidedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Approvals  []Approval `json:"approvals,omitempty"`
	DeniedBy   string     `json:"deniedBy,omitempty"`
	DenyReason string     `json:"denyReason,omitempty"`
	AuditSeq   int64      `json:"auditSeq,omitempty"`
}

// Proof 是批准时提交的证明。
type Proof struct {
	// Digest 是批准人在界面上看到的那份内容的摘要。
	Digest string
	// Reauthed 表示网关刚刚当场验证过批准人的身份（Passkey 或动态码；密码不算）。
	Reauthed bool
	// Typed 是批准人手动输入的确认内容。
	Typed string
	// DevBypass 表示 Reauthed 来自开发模式的放行，而不是真正的验证。只用于留痕。
	DevBypass bool
}

var (
	ErrPaused     = errors.New("管理员已暂停全部执行")
	ErrNotFound   = errors.New("请求不存在")
	ErrNotPending = errors.New("请求不在等待批准的状态")
	ErrExpired    = errors.New("请求已过期，请重新提交")
	ErrForbidden  = errors.New("禁止的操作（L3），任何角色、任何入口都不能放行")
	ErrNotAllowed = errors.New("没有权限")
	// ErrTooLarge 表示文件超过了下载上限。
	ErrTooLarge     = errors.New("文件太大")
	ErrDigest       = errors.New("请求内容已变化，批准作废")
	ErrReauth       = errors.New("高危操作需要当场重新验证身份")
	ErrConfirm      = errors.New("确认内容与目标不一致")
	ErrHumanOnly    = errors.New("只有人能批准")
	ErrL2WebOnly    = errors.New("高危操作只能在网页上批准")
	ErrSelfApproval = errors.New("双人批准：批准人不能是发起人")
)

// Event 是内核对外广播的事件。
type Event struct {
	Type string `json:"type"` // request | output | system
	Data any    `json:"data"`
}

// OutputChunk 是一段流式输出。
type OutputChunk struct {
	ID    string `json:"id"`
	Host  string `json:"host"`
	Chunk string `json:"chunk"`
}
