// Package hostenv 检测一台主机的“AI 环境”：装了哪些 AI 命令行工具和运行环境、登录过没有，
// 以及从这台主机出去能不能用各家大模型的接口（有些服务按出口地区拒绝访问）。
//
// 另外查出口 IP 的归属和类型、常用网站和流媒体能不能打开、在哪个区（见 net.go）。
//
// 全部是只读查询，走内核的 Query：命令是这里的常量，判级必须是 L0（测试和判级表里都锁住）。
// 所以这里不执行任何被检测的程序（`xxx --version` 是 L1）：有没有装看 command -v，
// 版本从 npm 包的 package.json 里读，登录状态只看凭据文件在不在，不读内容。
package hostenv

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Tool 是一个被检测的命令。
type Tool struct {
	Name  string `json:"name"`  // 命令名
	Title string `json:"title"` // 显示名
	Kind  string `json:"kind"`  // ai | runtime
	// App 是应用目录里对应的应用名：没装时页面给一个“去安装”的入口。
	App string `json:"app,omitempty"`
	// Package 是 npm 包名：版本从它的 package.json 里读。
	Package string `json:"-"`
	// Auth 是登录或配置后会出现的文件（只看在不在）。
	Auth []string `json:"-"`
}

// Tools 是检测的命令，按显示顺序。
var Tools = []Tool{
	{Name: "claude", Title: "Claude Code", Kind: "ai", App: "claude-code", Package: "@anthropic-ai/claude-code",
		Auth: []string{"$HOME/.claude/.credentials.json", "$HOME/.hostower/apps/claude-code/config.json"}},
	{Name: "codex", Title: "Codex CLI", Kind: "ai", App: "codex", Package: "@openai/codex",
		Auth: []string{"$HOME/.codex/auth.json", "$HOME/.hostower/apps/codex/config.json"}},
	{Name: "gemini", Title: "Gemini CLI", Kind: "ai", App: "gemini-cli", Package: "@google/gemini-cli",
		Auth: []string{"$HOME/.gemini/oauth_creds.json", "$HOME/.gemini/google_accounts.json"}},
	{Name: "dsh", Title: "DeepSeek Harness", Kind: "ai", App: "deepseek-harness", Package: "@deepseek-ai/dsh",
		Auth: []string{"$HOME/.hostower/apps/deepseek-harness/config.json"}},
	{Name: "opencode", Title: "opencode", Kind: "ai"},
	{Name: "aider", Title: "Aider", Kind: "ai"},
	{Name: "ollama", Title: "Ollama", Kind: "ai"},
	{Name: "docker", Title: "Docker", Kind: "runtime"},
	{Name: "node", Title: "Node.js", Kind: "runtime"},
	{Name: "npm", Title: "npm", Kind: "runtime"},
	{Name: "python3", Title: "Python 3", Kind: "runtime"},
	{Name: "uv", Title: "uv", Kind: "runtime"},
	{Name: "go", Title: "Go", Kind: "runtime"},
	{Name: "git", Title: "Git", Kind: "runtime"},
	{Name: "curl", Title: "curl", Kind: "runtime"},
}

// npm 全局包可能在的几个地方：Hostower 自己装的、系统的、/usr/local 的。
var npmRoots = []string{"~/.hostower/npm/lib/node_modules", "/usr/lib/node_modules", "/usr/local/lib/node_modules"}

// ToolsCommand 是检测命令和登录状态的只读脚本。必须是 L0。
var ToolsCommand = buildToolsCommand()

func buildToolsCommand() string {
	var names, pkgs, auth []string
	for _, t := range Tools {
		names = append(names, t.Name)
		if t.Package != "" {
			for _, r := range npmRoots {
				pkgs = append(pkgs, r+"/"+t.Package+"/package.json")
			}
		}
		for _, f := range t.Auth {
			auth = append(auth, `"`+f+`"`)
		}
	}
	return `echo '== tools'; for c in ` + strings.Join(names, " ") + `; do p="$(command -v "$c" 2>/dev/null)" && echo "$c	$p"; done; ` +
		`echo '== versions'; grep -Hs -m1 '"version"' ` + strings.Join(pkgs, " ") + `; ` +
		`echo '== claude-native'; ls ~/.local/share/claude/versions 2>/dev/null; ` +
		`echo '== auth'; for f in ` + strings.Join(auth, " ") + `; do [ -s "$f" ] && echo "$f"; done; ` +
		`echo '== end'`
}

// Service 是一家大模型接口。检测办法：不带密钥请求它的模型列表，看它怎么拒绝——
// 正常是“缺少密钥”（说明这台主机的出口能用它），按地区拒绝时是另一种回答。
type Service struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Used 是用这家接口的工具，给页面提示。
	Used string `json:"used,omitempty"`
	URL  string `json:"-"`
	// OKCode 是“能用、只是没带密钥”时的状态码；OKText 是这时回答里会出现的文字（状态码不够区分时用）。
	OKCode int    `json:"-"`
	OKText string `json:"-"`
	// BlockedText 是按地区拒绝时回答里会出现的文字；状态码是 403 也算被拒绝。
	BlockedText []string `json:"-"`
}

// Services 是检测的接口，按显示顺序。
var Services = []Service{
	{ID: "openai", Title: "OpenAI", Used: "Codex CLI", URL: "https://api.openai.com/v1/models", OKCode: 401, // hygiene:allow 供应商公开的接口地址
		BlockedText: []string{"unsupported_country", "not supported in your"}},
	{ID: "anthropic", Title: "Anthropic（Claude）", Used: "Claude Code", URL: "https://api.anthropic.com/v1/models", OKCode: 401}, // hygiene:allow 供应商公开的接口地址
	{ID: "gemini", Title: "Google Gemini", Used: "Gemini CLI", URL: "https://generativelanguage.googleapis.com/v1beta/models?key=x", OKCode: 400, OKText: "API key not valid", // hygiene:allow 供应商公开的接口地址
		BlockedText: []string{"location is not supported"}},
	{ID: "xai", Title: "xAI（Grok）", URL: "https://api.x.ai/v1/models", OKCode: 401},                                    // hygiene:allow 供应商公开的接口地址
	{ID: "deepseek", Title: "DeepSeek", Used: "DeepSeek Harness", URL: "https://api.deepseek.com/models", OKCode: 401}, // hygiene:allow 供应商公开的接口地址
}

// ToolStatus 是一个命令的检测结果。
type ToolStatus struct {
	Tool
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	// Signed 表示找到了登录凭据或面板写的配置（只对 AI 工具有意义）。
	Signed bool `json:"signed,omitempty"`
}

var (
	versionLineRe = regexp.MustCompile(`"version"\s*:\s*"([0-9A-Za-z.+-]{1,40})"`)
	nativeVerRe   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	pathRe        = regexp.MustCompile(`^/[A-Za-z0-9._/@+-]{1,200}$`)
)

// ParseTools 解析 ToolsCommand 的输出。
func ParseTools(out string) []ToolStatus {
	sec := sections(out)
	paths := map[string]string{}
	for _, l := range sec["tools"] {
		name, p, ok := strings.Cut(l, "\t")
		if ok && pathRe.MatchString(p) {
			paths[name] = p
		}
	}
	versions := map[string]string{}
	for _, l := range sec["versions"] {
		m := versionLineRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		for _, t := range Tools {
			// Hostower 自己装的排在最前面：同一个包在几个地方都有时，以先出现的为准。
			if t.Package != "" && strings.Contains(l, "/"+t.Package+"/package.json") && versions[t.Name] == "" {
				versions[t.Name] = m[1]
			}
		}
	}
	// Claude Code 的原生安装不在 npm 里：版本是 ~/.local/share/claude/versions 下的文件名，取最新的。
	var native []string
	for _, l := range sec["claude-native"] {
		if l = strings.TrimSpace(l); nativeVerRe.MatchString(l) {
			native = append(native, l)
		}
	}
	if len(native) > 0 && strings.Contains(paths["claude"], "/.local/bin/") {
		sort.Slice(native, func(i, j int) bool { return versionLess(native[i], native[j]) })
		versions["claude"] = native[len(native)-1]
	}
	signed := map[string]bool{}
	for _, l := range sec["auth"] {
		for _, t := range Tools {
			for _, f := range t.Auth {
				if strings.HasSuffix(l, strings.TrimPrefix(f, "$HOME")) {
					signed[t.Name] = true
				}
			}
		}
	}
	list := make([]ToolStatus, 0, len(Tools))
	for _, t := range Tools {
		st := ToolStatus{Tool: t, Path: paths[t.Name]}
		st.Installed = st.Path != ""
		if st.Installed {
			st.Version, st.Signed = versions[t.Name], signed[t.Name]
		}
		list = append(list, st)
	}
	return list
}

func versionLess(a, b string) bool {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(x) && i < len(y); i++ {
		m, _ := strconv.Atoi(x[i])
		n, _ := strconv.Atoi(y[i])
		if m != n {
			return m < n
		}
	}
	return len(x) < len(y)
}

func sections(out string) map[string][]string {
	sec := map[string][]string{}
	cur := ""
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.HasPrefix(l, "== ") {
			cur = strings.TrimPrefix(l, "== ")
			continue
		}
		if cur != "" && l != "" {
			sec[cur] = append(sec[cur], l)
		}
	}
	return sec
}

// 接口检测的结论。
const (
	StateOK          = "ok"          // 能用（只是没带密钥）
	StateBlocked     = "blocked"     // 按出口地区拒绝
	StateUnreachable = "unreachable" // 连不上（超时、解析失败、被重置）
	StateUnknown     = "unknown"     // 连上了，但回答不是预期的两种
)

// ServiceStatus 是一家接口的检测结果。
type ServiceStatus struct {
	Service
	State string `json:"state"`
	// Code 是 HTTP 状态码；连不上时是 0。
	Code int `json:"code"`
}

var codeRe = regexp.MustCompile(`\|code=([0-9]{3})\s*$`)

// ParseService 解析 NetCommand 里一家接口的那一行（应答的末尾加上 |code=状态码）。
func ParseService(s Service, out string) ServiceStatus {
	st := ServiceStatus{Service: s, State: StateUnreachable}
	m := codeRe.FindStringSubmatch(out)
	if m == nil {
		return st
	}
	st.Code, _ = strconv.Atoi(m[1])
	if st.Code == 0 {
		return st
	}
	body := strings.ToLower(out)
	for _, t := range s.BlockedText {
		if strings.Contains(body, strings.ToLower(t)) {
			st.State = StateBlocked
			return st
		}
	}
	switch {
	case st.Code == s.OKCode && (s.OKText == "" || strings.Contains(body, strings.ToLower(s.OKText))):
		st.State = StateOK
	case st.Code == 403:
		st.State = StateBlocked
	default:
		st.State = StateUnknown
	}
	return st
}
