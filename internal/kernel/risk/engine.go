package risk

import (
	"bytes"
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Engine 是判级引擎。内置规则随版本发布；规则包只能在其上加严。
type Engine struct {
	packs []PackRule
}

// New 创建判级引擎。packs 中的规则只参与取最大值，因此无法放宽任何内置判定。
func New(packs ...PackRule) *Engine {
	return &Engine{packs: packs}
}

const maxNesting = 6

// Classify 对一段 shell 文本判级：拆成单条命令逐条匹配，取最高等级。
// 解析失败或不认识的命令按 L1 处理，绝不降级。
func (e *Engine) Classify(src string) Verdict {
	return e.classify(src, 0)
}

func (e *Engine) classify(src string, depth int) Verdict {
	var v Verdict
	if strings.TrimSpace(src) == "" {
		v.add(Finding{Level: L0, Rule: "empty", Reason: "空命令"})
		return v
	}
	if depth > maxNesting {
		v.add(Finding{Command: clip(src), Level: L2, Rule: "nesting.depth", Reason: "嵌套层数过深，无法可靠判定"})
		return v
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		v.add(Finding{Command: clip(src), Level: L1, Rule: "parse.error", Reason: "无法解析，按变更处理"})
		return v
	}
	syntax.Walk(file, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Stmt:
			e.stmt(&v, x, depth)
		case *syntax.CallExpr:
			e.call(&v, x, depth)
		case *syntax.BinaryCmd:
			e.pipe(&v, x)
		case *syntax.FuncDecl:
			if x.Name != nil && callsName(x.Body, x.Name.Value) {
				v.add(Finding{Command: clip(nodeText(x)), Level: L3, Rule: "fork.bomb", Reason: "自我递归的函数（fork 炸弹）"})
			}
		}
		return true
	})
	if len(v.Findings) == 0 {
		v.add(Finding{Level: L0, Rule: "empty", Reason: "没有可执行的命令"})
	}
	return v
}

// word 是一个已渲染的 shell 单词。static 表示它不含任何运行时展开。
type word struct {
	text   string
	static bool
	glob   bool
	node   *syntax.Word
}

func renderWord(w *syntax.Word) word {
	out := word{static: true, node: w}
	if w == nil {
		return out
	}
	var sb strings.Builder
	var walk func(parts []syntax.WordPart, quoted bool)
	walk = func(parts []syntax.WordPart, quoted bool) {
		for _, p := range parts {
			switch x := p.(type) {
			case *syntax.Lit:
				if quoted {
					sb.WriteString(x.Value)
				} else {
					val := unescape(x.Value)
					if strings.ContainsAny(x.Value, "*?[") {
						out.glob = true
					}
					sb.WriteString(val)
				}
			case *syntax.SglQuoted:
				sb.WriteString(x.Value)
			case *syntax.DblQuoted:
				walk(x.Parts, true)
			case *syntax.ParamExp:
				out.static = false
				sb.WriteString("$")
				if x.Param != nil {
					sb.WriteString(x.Param.Value)
				}
			case *syntax.CmdSubst:
				out.static = false
				sb.WriteString("$(…)")
			default:
				out.static = false
				sb.WriteString("$(…)")
			}
		}
	}
	walk(w.Parts, false)
	out.text = sb.String()
	return out
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func nodeText(n syntax.Node) string {
	var buf bytes.Buffer
	_ = syntax.NewPrinter().Print(&buf, n)
	return strings.TrimSpace(buf.String())
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

func callsName(n syntax.Node, name string) bool {
	found := false
	if n == nil {
		return false
	}
	syntax.Walk(n, func(n syntax.Node) bool {
		if c, ok := n.(*syntax.CallExpr); ok && len(c.Args) > 0 {
			if w := renderWord(c.Args[0]); w.static && w.text == name {
				found = true
			}
		}
		return !found
	})
	return found
}

// resolved 是剥掉 sudo / env / xargs 等包装之后真正要执行的命令。
type resolved struct {
	name string
	// full 是命令名的原样写法（带路径）。绝大多数规则只看去掉路径的名字，个别规则要认准某个固定位置。
	full      string
	args      []word
	dynamic   bool // 命令名由运行时决定
	bare      bool // 只有包装、没有内层命令
	wrapper   string
	wrapFlags []word
	stdinArgs bool // 参数来自标准输入（xargs、find -exec）
}

type wrapSpec struct {
	valueFlags string // 以空格分隔、需要跟一个值的选项
	positional int    // 选项之后要跳过的位置参数个数
	assigns    bool   // 跳过 NAME=VALUE
}

var wrappers = map[string]wrapSpec{
	"sudo":    {valueFlags: "-u -g -h -p -C -D -r -t -U -T -R", assigns: true},
	"doas":    {valueFlags: "-u -C"},
	"env":     {valueFlags: "-u -C", assigns: true},
	"nohup":   {},
	"setsid":  {},
	"exec":    {valueFlags: "-a"},
	"builtin": {},
	"nice":    {valueFlags: "-n"},
	"ionice":  {valueFlags: "-c -n -p"},
	"time":    {valueFlags: "-f -o"},
	"timeout": {valueFlags: "-s -k", positional: 1},
	"stdbuf":  {valueFlags: "-i -o -e"},
	"watch":   {valueFlags: "-n -d"},
	"xargs":   {valueFlags: "-I -n -P -L -d -E -s -a"},
}

func resolve(args []word) resolved {
	var r resolved
	for i := 0; i < 8; i++ {
		if len(args) == 0 {
			r.bare = true
			return r
		}
		if !args[0].static {
			r.dynamic = true
			r.args = args
			return r
		}
		name := path.Base(args[0].text)
		spec, ok := wrappers[name]
		if !ok {
			r.name, r.full, r.args = name, args[0].text, args[1:]
			return r
		}
		r.wrapper = name
		if name == "xargs" {
			r.stdinArgs = true
		}
		rest := args[1:]
		j := 0
		r.wrapFlags = nil
		for j < len(rest) {
			a := rest[j]
			if !a.static {
				break
			}
			if a.text == "--" {
				j++
				break
			}
			if len(a.text) > 1 && a.text[0] == '-' {
				r.wrapFlags = append(r.wrapFlags, a)
				if hasField(spec.valueFlags, a.text) {
					j++
				}
				j++
				continue
			}
			if spec.assigns && isAssign(a.text) {
				j++
				continue
			}
			break
		}
		for k := 0; k < spec.positional && j < len(rest); k++ {
			j++
		}
		args = rest[j:]
	}
	r.dynamic = true
	return r
}

func hasField(fields, s string) bool {
	for _, f := range strings.Fields(fields) {
		if f == s {
			return true
		}
	}
	return false
}

var assignRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func isAssign(s string) bool { return assignRe.MatchString(s) }

func renderArgs(ws []*syntax.Word) []word {
	out := make([]word, len(ws))
	for i, w := range ws {
		out[i] = renderWord(w)
	}
	return out
}

var shells = set("sh bash zsh dash ksh ash fish su")
var interpreters = set("python python2 python3 perl ruby node nodejs php lua")
var fetchers = set("curl wget nc ncat netcat fetch aria2c socat")

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

// containsFetch 报告节点里是否有从网络取内容的命令。
func containsFetch(n syntax.Node) bool {
	found := false
	if n == nil {
		return false
	}
	syntax.Walk(n, func(n syntax.Node) bool {
		if c, ok := n.(*syntax.CallExpr); ok && len(c.Args) > 0 {
			if r := resolve(renderArgs(c.Args)); fetchers[r.name] {
				found = true
			}
		}
		return !found
	})
	return found
}

// scriptArg 找出 `sh -c <脚本>` 里的脚本；ok=false 表示没有 -c。
func scriptArg(args []word) (script word, ok bool, hasFile bool) {
	for i, a := range args {
		if !a.static {
			hasFile = true
			continue
		}
		if strings.HasPrefix(a.text, "-") && !strings.HasPrefix(a.text, "--") && strings.Contains(a.text, "c") {
			if i+1 < len(args) {
				return args[i+1], true, false
			}
			return word{}, false, false
		}
		if a.text == "--command" && i+1 < len(args) {
			return args[i+1], true, false
		}
		if !strings.HasPrefix(a.text, "-") {
			hasFile = true
		}
	}
	return word{}, false, hasFile
}

func (e *Engine) stmt(v *Verdict, s *syntax.Stmt, depth int) {
	var target resolved
	if c, ok := s.Cmd.(*syntax.CallExpr); ok && len(c.Args) > 0 {
		target = resolve(renderArgs(c.Args))
	}
	// 这条命令是不是在写 compose 文件：重定向到 compose 文件，或者 tee compose 文件。
	compose := false
	for _, r := range s.Redirs {
		switch r.Op {
		case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.AppClob, syntax.RdrAll, syntax.AppAll, syntax.RdrAllClob, syntax.AppAllClob:
			if w := renderWord(r.Word); isComposeFile(w.text) {
				compose = true
			}
		}
	}
	if target.name == "tee" {
		for _, a := range target.args {
			if isComposeFile(a.text) {
				compose = true
			}
		}
	}
	for _, r := range s.Redirs {
		switch r.Op {
		case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.AppClob,
			syntax.RdrAll, syntax.AppAll, syntax.RdrAllClob, syntax.AppAllClob, syntax.RdrInOut:
			w := renderWord(r.Word)
			text := ">" + " " + w.text
			switch {
			case harmlessSink(w.text):
			case isBlockDevice(w.text):
				v.add(Finding{Command: text, Level: L3, Rule: "redirect.blockdev", Reason: "写块设备"})
			case !w.static:
				v.add(Finding{Command: text, Level: L1, Rule: "redirect.dynamic", Reason: "重定向到运行时才确定的路径"})
			default:
				lvl, reason := WritePathLevel(w.text)
				v.add(Finding{Command: text, Level: lvl, Rule: "redirect.write", Reason: reason})
			}
		case syntax.RdrIn:
			w := renderWord(r.Word)
			if isSecretPath(w.text) {
				v.add(Finding{Command: "< " + w.text, Level: L3, Rule: "secret.read", Reason: "读取私钥或凭据文件"})
			}
		case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			if compose {
				body := r.Hdoc
				if r.Op == syntax.WordHdoc {
					body = r.Word
				}
				if w := renderWord(body); !w.static {
					v.add(Finding{Command: clip(nodeText(s)), Level: L2, Rule: "compose.dynamic", Reason: "写进 compose 文件的内容有运行时展开，无法静态判定"})
				} else {
					for _, f := range ComposeFindings(w.text) {
						v.add(f)
					}
				}
				continue
			}
			if !shells[target.name] && !interpreters[target.name] {
				continue
			}
			body := r.Hdoc
			if r.Op == syntax.WordHdoc {
				body = r.Word
			}
			w := renderWord(body)
			if shells[target.name] && w.static {
				inner := e.classify(w.text, depth+1)
				for _, f := range inner.Findings {
					v.add(f)
				}
			} else {
				v.add(Finding{Command: clip(nodeText(s)), Level: L2, Rule: "shell.stdin", Reason: "交给解释器执行的内容无法静态判定"})
			}
		}
	}
}

func (e *Engine) pipe(v *Verdict, b *syntax.BinaryCmd) {
	if b.Op != syntax.Pipe && b.Op != syntax.PipeAll {
		return
	}
	c, ok := b.Y.Cmd.(*syntax.CallExpr)
	if !ok || len(c.Args) == 0 {
		return
	}
	r := resolve(renderArgs(c.Args))
	if !shells[r.name] && !interpreters[r.name] {
		return
	}
	if _, hasScript, hasFile := scriptArg(r.args); hasScript || hasFile {
		return
	}
	text := clip(nodeText(b))
	if containsFetch(b.X) {
		v.add(Finding{Command: text, Level: L3, Rule: "pipe.remote-shell", Reason: "把下载的内容直接交给解释器执行"})
		return
	}
	v.add(Finding{Command: text, Level: L2, Rule: "pipe.shell", Reason: "管道内容交给解释器执行，无法静态判定"})
}

func (e *Engine) call(v *Verdict, c *syntax.CallExpr, depth int) {
	if len(c.Args) == 0 {
		return // 只有赋值；右侧的命令替换会被单独遍历
	}
	args := renderArgs(c.Args)
	text := clip(nodeText(c))
	f := e.classifyCall(v, args, text, depth, false)
	if f != nil {
		v.add(*f)
	}
}

// classifyCall 判定一条简单命令。可能直接向 v 追加（递归脚本），也可能返回一条 Finding。
func (e *Engine) classifyCall(v *Verdict, args []word, text string, depth int, stdinArgs bool) *Finding {
	r := resolve(args)
	r.stdinArgs = r.stdinArgs || stdinArgs
	switch {
	case r.dynamic:
		return &Finding{Command: text, Level: L1, Rule: "command.dynamic", Reason: "命令名由运行时决定，按变更处理"}
	case r.bare:
		return bareWrapper(r, text)
	}

	// shell / eval：能看到脚本就递归判级，看不到就按高危处理。
	if shells[r.name] || r.name == "eval" {
		for _, a := range r.args {
			if a.node != nil && containsFetch(a.node) {
				return &Finding{Command: text, Level: L3, Rule: "shell.remote", Reason: "把下载的内容直接交给解释器执行"}
			}
		}
		if r.name == "eval" {
			if allStatic(r.args) {
				e.mergeNested(v, joinWords(r.args), depth)
				return nil
			}
			return &Finding{Command: text, Level: L2, Rule: "eval.dynamic", Reason: "eval 的内容无法静态判定"}
		}
		script, ok, _ := scriptArg(r.args)
		if ok {
			if script.static {
				e.mergeNested(v, script.text, depth)
				return nil
			}
			return &Finding{Command: text, Level: L2, Rule: "shell.dynamic", Reason: "交给 shell 的脚本无法静态判定"}
		}
		return &Finding{Command: text, Level: L1, Rule: "shell.script", Reason: "运行脚本文件或交互 shell，内容未知"}
	}

	cmd := &command{name: r.name, full: r.full, args: r.args, stdinArgs: r.stdinArgs}
	lvl, rule, reason := builtin(e, v, cmd, depth)
	f := &Finding{Command: text, Level: lvl, Rule: rule, Reason: reason}

	// 私钥与凭据文件：除了只看元数据的命令，任何引用都禁止。
	if !metadataOnly(cmd) {
		for i, a := range cmd.args {
			if i > 0 && cmd.args[i-1].text == "-i" && usesIdentity[cmd.name] {
				continue
			}
			for _, cand := range pathCandidates(a.text) {
				if isSecretPath(cand) || isSSHDirWhole(cand, a.glob) {
					return &Finding{Command: text, Level: L3, Rule: "secret.read", Reason: "读取或外传私钥、凭据文件"}
				}
			}
		}
	}
	// 修改登录、提权、防火墙相关配置：变更升为高危。
	if f.Level == L1 {
		for _, a := range cmd.args {
			for _, cand := range pathCandidates(a.text) {
				if isCriticalConfig(cand) {
					f.Level, f.Rule, f.Reason = L2, "config.critical", "修改登录、提权或防火墙相关配置"
				}
			}
		}
	}
	// 读内容的命令如果参数要到运行时才确定，无法保证它不读敏感文件。
	if f.Level == L0 && contentReaders[cmd.name] && (cmd.stdinArgs || !allStatic(cmd.args)) {
		f.Level, f.Rule, f.Reason = L1, "args.dynamic", "读取的路径由运行时决定，无法静态判定"
	}
	// 规则包：只能加严。
	joined := joinWords(cmd.args)
	for _, p := range e.packs {
		if p.Level > f.Level && p.matches(cmd.name, joined) {
			f.Level, f.Rule, f.Reason = p.Level, "pack."+p.ID, p.Reason
		}
	}
	return f
}

func (e *Engine) mergeNested(v *Verdict, script string, depth int) {
	inner := e.classify(script, depth+1)
	for _, f := range inner.Findings {
		v.add(f)
	}
}

func bareWrapper(r resolved, text string) *Finding {
	if r.wrapper == "sudo" || r.wrapper == "doas" {
		for _, f := range r.wrapFlags {
			if f.text == "-l" || f.text == "-v" || f.text == "-k" || f.text == "-V" || f.text == "--list" {
				return &Finding{Command: text, Level: L0, Rule: "sudo.query", Reason: "只读"}
			}
		}
		return &Finding{Command: text, Level: L1, Rule: "sudo.shell", Reason: "提权 shell，内容未知"}
	}
	return &Finding{Command: text, Level: L0, Rule: "wrapper.bare", Reason: "只读"}
}

func allStatic(ws []word) bool {
	for _, w := range ws {
		if !w.static {
			return false
		}
	}
	return true
}

func joinWords(ws []word) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = w.text
	}
	return strings.Join(parts, " ")
}

// pathCandidates 把 `of=/dev/sda`、`--file=/etc/shadow` 这类参数拆出路径部分。
func pathCandidates(s string) []string {
	out := []string{s}
	if i := strings.IndexByte(s, '='); i >= 0 && i+1 < len(s) {
		out = append(out, s[i+1:])
	}
	return out
}
