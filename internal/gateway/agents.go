package gateway

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/snippet"
)

// Agent 任务：把同一个任务交给若干台主机上的 AI 编程助手（Claude Code、Codex、Gemini CLI）各自去做，
// 再让其中一台上的助手把各台的结果汇总。都没有自己的执行通道：
//   - 派发是一个多主机的命令请求（每台主机上运行一次非交互的助手），判级、审批、审计照常；
//   - 汇总是一个单主机的命令请求，各台的输出先暂存成一个文件，从标准输入送给助手（和上传文件同一条路）。
// 助手按自己的权限设置做事：默认只读；勾选“允许改文件”时也只放开工作目录里的修改，
// 跳过权限确认或沙箱的开关这里不提供（判级规则把它们判成高危）。

// agentSpec 是一种助手的非交互调用方式。
type agentSpec struct {
	title string
	// run 渲染在工作目录里执行任务的命令；write 表示允许改工作目录里的文件。
	run func(prompt string, write bool) string
	// summary 表示能不能从标准输入读入材料做汇总。
	summary bool
}

// 优先用 Hostower 装的那一份（~/.local/bin），没有就用系统 PATH 里的。
const agentPath = `PATH="$HOME/.local/bin:$PATH" `

var agentSpecs = map[string]agentSpec{
	"claude": {title: "Claude Code", summary: true, run: func(p string, write bool) string {
		c := agentPath + "claude -p " + snippet.Quote(p)
		if write {
			c += " --permission-mode acceptEdits"
		}
		return c
	}},
	"codex": {title: "Codex CLI", run: func(p string, write bool) string {
		mode := "read-only"
		if write {
			mode = "workspace-write"
		}
		return agentPath + "codex exec --skip-git-repo-check --sandbox " + mode + " " + snippet.Quote(p)
	}},
	"gemini": {title: "Gemini CLI", summary: true, run: func(p string, write bool) string {
		c := agentPath + "gemini -p " + snippet.Quote(p)
		if write {
			c += " --approval-mode auto_edit"
		}
		return c
	}},
}

const (
	maxAgentPrompt  = 4000
	maxSummaryBytes = 512 << 10
)

func validPrompt(p string) bool {
	p = strings.TrimSpace(p)
	return p != "" && utf8.RuneCountInString(p) <= maxAgentPrompt && utf8.ValidString(p) && !strings.ContainsRune(p, 0)
}

// promptHead 是写进请求意图的任务开头（一行）。
func promptHead(p string) string {
	p = strings.Join(strings.Fields(p), " ")
	if r := []rune(p); len(r) > 60 {
		p = string(r[:60]) + "…"
	}
	return p
}

// agentTask 把一个任务派给若干台主机上的同一种助手。
func (s *Server) agentTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Agent    string          `json:"agent"`
		Hosts    []string        `json:"hosts"`
		Dir      string          `json:"dir"`
		Prompt   string          `json:"prompt"`
		Write    bool            `json:"write"`
		Preview  bool            `json:"preview"`
		Strategy kernel.Strategy `json:"strategy"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	spec, ok := agentSpecs[in.Agent]
	if !ok {
		writeErr(w, 400, "不认识的 AI 编程助手")
		return
	}
	hosts, err := s.syncHosts(in.Hosts)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	dir, ok := cleanRemote(in.Dir)
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	if !validPrompt(in.Prompt) {
		writeErr(w, 400, "任务不能为空，最长 "+strconv.Itoa(maxAgentPrompt)+" 个字")
		return
	}
	cmd := "cd " + snippet.Quote(dir) + " && " + spec.run(strings.TrimSpace(in.Prompt), in.Write)
	var plan []plannedTarget
	for _, h := range hosts {
		plan = append(plan, plannedTarget{host: h, command: cmd})
	}
	head := promptHead(in.Prompt)
	intent := func(n int) string {
		s := "Agent 任务（" + spec.title + "）：" + head
		if n > 1 {
			s += "（" + strconv.Itoa(n) + " 台主机）"
		}
		return s
	}
	s.submitPlan(w, r, plan, in.Preview, in.Strategy, intent, "agent:"+in.Agent)
}

// stageBytes 把一段内容暂存成上传文件（和页面上传的文件一样，命令执行时从标准输入送出去）。
func (s *Server) stageBytes(r *http.Request, name string, data []byte) (uploadInfo, error) {
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	p, err := s.Kernel.UploadPath(id)
	if err != nil {
		return uploadInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return uploadInfo{}, errors.New("无法创建上传目录")
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return uploadInfo{}, errors.New("无法保存上传的文件")
	}
	sum := sha256.Sum256(data)
	info := uploadInfo{ID: id, Name: name, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}
	meta, _ := json.Marshal(info)
	os.WriteFile(p+".json", meta, 0o600)
	if err := s.auditErr(r, audit.Entry{Type: "upload.stored", Message: "暂存上传的文件 " + name, Detail: map[string]any{"size": info.Size, "sha256": info.SHA256}}); err != nil {
		os.Remove(p)
		os.Remove(p + ".json")
		return uploadInfo{}, errors.New("写审计失败，没有暂存")
	}
	return info, nil
}

// agentSummary 让一台主机上的助手汇总一次 Agent 任务在各台主机上的输出。
func (s *Server) agentSummary(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From    string `json:"from"`
		Host    string `json:"host"`
		Agent   string `json:"agent"`
		Prompt  string `json:"prompt"`
		Preview bool   `json:"preview"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	spec, ok := agentSpecs[in.Agent]
	if !ok || !spec.summary {
		writeErr(w, 400, "这个 AI 编程助手不能用来汇总（要能从标准输入读材料）")
		return
	}
	if _, ok := s.Kernel.Inventory().Host(in.Host); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	if !validPrompt(in.Prompt) {
		writeErr(w, 400, "任务不能为空，最长 "+strconv.Itoa(maxAgentPrompt)+" 个字")
		return
	}
	src, err := s.Kernel.Get(in.From)
	if err != nil {
		fail(w, err)
		return
	}
	if !strings.HasPrefix(src.Snippet, "agent:") {
		writeErr(w, 400, "只能汇总 Agent 任务的结果")
		return
	}
	if src.State != kernel.StateDone && src.State != kernel.StateFailed {
		writeErr(w, 400, "这个任务还没有执行完")
		return
	}
	// 各台主机的输出按主机分段，拼成一份材料。
	var b strings.Builder
	b.WriteString("# " + src.Intent + "\n")
	for _, t := range src.Targets {
		b.WriteString("\n## " + t.Host + "（" + string(t.State) + "）\n\n")
		b.WriteString(t.Output)
		if !strings.HasSuffix(t.Output, "\n") {
			b.WriteString("\n")
		}
	}
	data := []byte(b.String())
	if len(data) > maxSummaryBytes {
		data = append(data[:maxSummaryBytes], []byte("\n…（后面的输出太长，省略了）\n")...)
	}
	cmd := spec.run(strings.TrimSpace(in.Prompt), false)
	if in.Preview {
		// 预览只看命令和判级，不暂存文件。
		s.submitPlan(w, r, []plannedTarget{{host: in.Host, command: cmd}}, true, kernel.Strategy{}, func(int) string { return "" }, "")
		return
	}
	info, err := s.stageBytes(r, "agent-results.md", data)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), kernel.CommandSpec{
		Hosts:    []string{in.Host},
		Commands: map[string]string{in.Host: cmd},
		Intent:   "汇总 " + src.ID + " 的结果（" + spec.title + "）",
		Snippet:  "agent-summary:" + in.Agent,
		Input:    &kernel.Input{Kind: "upload", Label: "agent-results.md", Upload: info.ID, Size: info.Size, SHA256: info.SHA256},
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"requests": []*kernel.Request{req}})
}
