package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"github.com/tsienwan/hostower/internal/agentbin"
	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/metrics"
	"github.com/tsienwan/hostower/internal/secrets"
	"github.com/tsienwan/hostower/internal/snippet"
)

// ---------- 引导与认证 ----------

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	need, err := s.Auth.NeedsSetup()
	if err != nil {
		writeErr(w, 500, "数据库不可用")
		return
	}
	out := map[string]any{"version": s.Version, "dev": s.Dev, "setupRequired": need, "user": nil, "started": s.Started.UnixMilli(), "auth": s.loginInfo(),
		// 构建封签和许可证对所有人公开（登录页也显示）：AGPL 要求告诉通过网络使用它的人去哪里拿源代码。
		"build": s.Build, "license": "AGPL-3.0-only", "source": s.sourceURL(),
		// 用 IP 访问时浏览器不允许用 Passkey：页面据此把“添加 Passkey”置灰并说明原因。
		"passkey": s.Auth.PasskeyAvailable()}
	if s.OIDC != nil {
		if on, name := s.OIDC.Enabled(); on {
			out["oidc"] = map[string]any{"name": name}
		}
	}
	if c, err := r.Cookie(cookieName); err == nil {
		if sess, err := s.Auth.Lookup(c.Value); err == nil {
			out["user"] = sess.User
			out["devSession"] = sess.Dev
			out["login"] = sess.Method
			f := s.Auth.Factors(sess.User.ID)
			out["initialPassword"] = f.PasswordInitial
			// 已经有 Passkey 或动态码：提示里就不再要求“添加 Passkey 或动态码”。
			out["secondFactor"] = f.Passkeys > 0 || f.TOTP
			// 还在用默认的管理员用户名（admin）：页面上提醒改掉，别人就少猜对一半。
			out["defaultUsername"] = sess.User.Role == "owner" && sess.User.Name == auth.DefaultOwnerName
			out["paused"] = s.Kernel.Paused()
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) loginBegin(w http.ResponseWriter, r *http.Request) {
	id, opts, err := s.Auth.LoginBegin()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ceremony": id, "options": opts})
}

func (s *Server) loginFinish(w http.ResponseWriter, r *http.Request) {
	token, u, err := s.Auth.LoginFinish(r.URL.Query().Get("ceremony"), r.Body)
	if err != nil {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: err.Error()})
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.login", Actor: u.Name, Message: "Passkey 登录"})
	s.setSession(w, token)
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Token string }
	if !readJSON(w, r, &in) {
		return
	}
	token, u, err := s.Auth.DevLogin(strings.TrimSpace(in.Token))
	if err != nil {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: "开发模式令牌登录：" + err.Error()})
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.login", Actor: u.Name, Message: "开发模式令牌登录（未经 Passkey）"})
	s.setSession(w, token)
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.Auth.Logout(sessionOf(r))
	// 聊天账号的绑定也是一种“会话”：一起作废。
	s.bot.unbind(r.Context(), "吊销所有会话")
	s.clearSession(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) reauthBegin(w http.ResponseWriter, r *http.Request) {
	id, opts, err := s.Auth.ReauthBegin(sessionOf(r).User)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ceremony": id, "options": opts})
}

func (s *Server) reauthFinish(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.ReauthFinish(sessionOf(r), r.URL.Query().Get("ceremony"), r.Body); err != nil {
		s.audit(r, audit.Entry{Type: "auth.reauth.failed", Message: err.Error()})
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.verify", Message: "用 Passkey 验证了身份"})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) passkeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.Auth.Passkeys(sessionOf(r).User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"passkeys": list})
}

func (s *Server) passkeyBegin(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	id, opts, err := s.Auth.AddBegin(sessionOf(r).User)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ceremony": id, "options": opts})
}

func (s *Server) passkeyFinish(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	q := r.URL.Query()
	if err := s.Auth.AddFinish(sessionOf(r).User, q.Get("ceremony"), q.Get("name"), r.Body); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.passkey.added", Message: "新增 Passkey：" + q.Get("name")})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- 主机 ----------

type hostView struct {
	inventory.Host
	Status metrics.Status `json:"status"`
}

type groupView struct {
	inventory.Group
	Members []string `json:"members"`
}

func (s *Server) hosts(w http.ResponseWriter, r *http.Request) {
	inv := s.Kernel.Inventory()
	status := s.Sampler.All(true)
	hosts := []hostView{}
	for _, h := range inv.Hosts() {
		st, ok := status[h.Name]
		if !ok {
			st = metrics.Status{Host: h.Name}
		}
		h.Vars = nil // 主机变量可能含部署细节，列表里不返回
		hosts = append(hosts, hostView{Host: h, Status: st})
	}
	groups := []groupView{}
	for _, g := range inv.Groups() {
		gv := groupView{Group: g, Members: []string{}}
		for _, m := range inv.Members(g) {
			gv.Members = append(gv.Members, m.Name)
		}
		groups = append(groups, gv)
	}
	writeJSON(w, 200, map[string]any{"hosts": hosts, "groups": groups})
}

// hostMetrics 返回一台主机的指标历史（已经落盘的部分）。
func (s *Server) hostMetrics(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.Kernel.Inventory().Host(name); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	samples, err := s.Sampler.History(r.Context(), name, r.URL.Query().Get("range"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"samples": samples})
}

type hostBody struct {
	Name        string           `json:"name"`
	Addr        string           `json:"addr"`
	Executor    string           `json:"executor"`
	Credential  string           `json:"credential"`
	Tags        []string         `json:"tags"`
	Sensitivity risk.Sensitivity `json:"sensitivity"`
}

func (s *Server) saveHost(w http.ResponseWriter, r *http.Request, create bool) {
	if !ownerOnly(w, r) {
		return
	}
	var in hostBody
	if !readJSON(w, r, &in) {
		return
	}
	if !create {
		in.Name = r.PathValue("name")
	}
	h := inventory.Host{Name: strings.TrimSpace(in.Name), Addr: strings.TrimSpace(in.Addr), Executor: in.Executor,
		Credential: strings.TrimSpace(in.Credential), Tags: in.Tags, Sensitivity: in.Sensitivity}
	// 保留页面上改不到的主机变量。
	if cur, ok := s.Kernel.Inventory().Host(h.Name); ok {
		h.Vars = cur.Vars
	}
	if err := s.SaveHost(h, create); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	typ := "inventory.host.updated"
	if create {
		typ = "inventory.host.created"
	}
	s.audit(r, audit.Entry{Type: typ, Host: h.Name, Message: "地址 " + h.Addr + "，敏感度 " + string(h.Sensitivity),
		Detail: map[string]any{"executor": h.Executor, "credential": h.Credential, "tags": h.Tags}})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) createHost(w http.ResponseWriter, r *http.Request) { s.saveHost(w, r, true) }
func (s *Server) updateHost(w http.ResponseWriter, r *http.Request) { s.saveHost(w, r, false) }

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	if err := s.DeleteHost(name); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "inventory.host.deleted", Host: name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// resolve 把“选择器或主机列表”解析成主机。
func (s *Server) resolve(selector string, names []string) ([]inventory.Host, error) {
	inv := s.Kernel.Inventory()
	if len(names) > 0 {
		out := make([]inventory.Host, 0, len(names))
		for _, n := range names {
			h, ok := inv.Host(n)
			if !ok {
				return nil, errors.New("主机 " + n + " 不存在")
			}
			out = append(out, h)
		}
		return out, nil
	}
	if strings.TrimSpace(selector) == "" {
		return nil, errors.New("请选择目标主机")
	}
	hs, err := inv.Select(selector)
	if err != nil {
		return nil, err
	}
	if len(hs) == 0 {
		return nil, errors.New("选择器没有匹配到任何主机")
	}
	return hs, nil
}

func (s *Server) selectHosts(w http.ResponseWriter, r *http.Request) {
	hs, err := s.resolve(r.URL.Query().Get("selector"), nil)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	names := make([]string, len(hs))
	for i, h := range hs {
		names[i] = h.Name
	}
	writeJSON(w, 200, map[string]any{"hosts": names})
}

// ---------- 判级预览与提交 ----------

type targetPreview struct {
	Host     string         `json:"host"`
	Command  string         `json:"command"`
	Level    risk.Level     `json:"level"`
	Findings []risk.Finding `json:"findings"`
}

type submitBody struct {
	Command  string           `json:"command"`
	Snippet  string           `json:"snippet"`
	Params   map[string]any   `json:"params"`
	Selector string           `json:"selector"`
	Hosts    []string         `json:"hosts"`
	Intent   string           `json:"intent"`
	Strategy *kernel.Strategy `json:"strategy"`
	// Draft 是还没保存的 Snippet，只用于预览（提交时会被忽略）。
	Draft *snippet.Snippet `json:"draft"`
}

// plan 把请求体变成“每台主机上的实际命令”。预览和提交共用这一段，
// 所以批准人预览到的和内核收到的是同一份渲染结果。
func (s *Server) plan(in submitBody) (kernel.CommandSpec, []targetPreview, error) {
	var spec kernel.CommandSpec
	hs, err := s.resolve(in.Selector, in.Hosts)
	if err != nil {
		return spec, nil, err
	}
	spec.Intent = strings.TrimSpace(in.Intent)
	spec.Commands = map[string]string{}
	if in.Strategy != nil {
		spec.Strategy = *in.Strategy
	}
	var render func(inventory.Host) (string, error)
	if in.Snippet != "" || in.Draft != nil {
		sn, ok := s.Snippets()[in.Snippet]
		if in.Draft != nil {
			draft, _, err := snippet.FromSpec(*in.Draft)
			if err != nil {
				return spec, nil, err
			}
			sn, ok = draft, true
		}
		if !ok {
			return spec, nil, errors.New("Snippet " + in.Snippet + " 不存在")
		}
		params, err := sn.Validate(in.Params)
		if err != nil {
			return spec, nil, err
		}
		spec.Snippet = sn.Name + "@" + sn.Version
		spec.Declared = sn.Level
		if in.Strategy == nil {
			spec.Strategy = kernel.Strategy{Concurrency: sn.Strategy.Concurrency, Canary: sn.Strategy.Canary, StopOnError: sn.Strategy.StopOnError}
		}
		if spec.Intent == "" {
			spec.Intent = sn.Title
		}
		render = func(h inventory.Host) (string, error) { return sn.Render(params, h) }
	} else {
		cmd := strings.TrimSpace(in.Command)
		if cmd == "" {
			return spec, nil, errors.New("命令为空")
		}
		if len(cmd) > 16<<10 {
			return spec, nil, errors.New("命令过长")
		}
		render = func(inventory.Host) (string, error) { return cmd, nil }
	}
	previews := make([]targetPreview, 0, len(hs))
	for _, h := range hs {
		cmd, err := render(h)
		if err != nil {
			return spec, nil, err
		}
		spec.Hosts = append(spec.Hosts, h.Name)
		spec.Commands[h.Name] = cmd
		lvl, fs := s.Kernel.Assess(h, cmd, spec.Declared)
		previews = append(previews, targetPreview{Host: h.Name, Command: cmd, Level: lvl, Findings: fs})
	}
	return spec, previews, nil
}

func (s *Server) preview(w http.ResponseWriter, in submitBody) {
	_, previews, err := s.plan(in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	lvl := risk.L0
	for _, p := range previews {
		lvl = risk.Max(lvl, p.Level)
	}
	writeJSON(w, 200, map[string]any{"level": lvl, "targets": previews})
}

func (s *Server) classify(w http.ResponseWriter, r *http.Request) {
	var in submitBody
	if !readJSON(w, r, &in) {
		return
	}
	s.preview(w, in)
}

func (s *Server) renderSnippet(w http.ResponseWriter, r *http.Request) {
	var in submitBody
	if !readJSON(w, r, &in) {
		return
	}
	in.Snippet = r.PathValue("name")
	s.preview(w, in)
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	var in submitBody
	if !readJSON(w, r, &in) {
		return
	}
	in.Draft = nil // 只有保存过的 Snippet 才能执行
	spec, _, err := s.plan(in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), spec)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func (s *Server) listRequests(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}
	writeJSON(w, 200, map[string]any{"requests": s.Kernel.List(limit)})
}

func (s *Server) getRequest(w http.ResponseWriter, r *http.Request) {
	req, err := s.Kernel.Get(r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func (s *Server) approve(w http.ResponseWriter, r *http.Request) {
	var in struct{ Digest, Typed string }
	if !readJSON(w, r, &in) {
		return
	}
	sess := sessionOf(r)
	id := r.PathValue("id")
	proof := kernel.Proof{Digest: in.Digest, Typed: strings.TrimSpace(in.Typed)}
	// 只有 L2 才消耗二次验证；验证是一次性的。
	if cur, err := s.Kernel.Get(id); err == nil && cur.Level == risk.L2 && cur.State == kernel.StatePending {
		proof.Reauthed = s.Auth.ConsumeReauth(sess)
		proof.DevBypass = proof.Reauthed && sess.Dev
	}
	req, err := s.Kernel.Approve(r.Context(), id, actor(sess), proof)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func (s *Server) deny(w http.ResponseWriter, r *http.Request) {
	var in struct{ Reason string }
	if !readJSON(w, r, &in) {
		return
	}
	req, err := s.Kernel.Deny(r.Context(), r.PathValue("id"), actor(sessionOf(r)), strings.TrimSpace(in.Reason))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	req, err := s.Kernel.Cancel(r.Context(), r.PathValue("id"), actor(sessionOf(r)))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

// ---------- Snippet ----------

func (s *Server) snippets(w http.ResponseWriter, r *http.Request) {
	all := s.Snippets()
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]any, 0, len(names))
	for _, n := range names {
		out = append(out, all[n])
	}
	writeJSON(w, 200, map[string]any{"snippets": out})
}

func (s *Server) saveSnippet(w http.ResponseWriter, r *http.Request, create bool) {
	if !ownerOnly(w, r) {
		return
	}
	var in snippet.Snippet
	if !readJSON(w, r, &in) {
		return
	}
	if !create {
		in.Name = r.PathValue("name")
	}
	sn, err := s.SaveSnippet(in, create)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	typ := "snippet.updated"
	if create {
		typ = "snippet.created"
	}
	s.audit(r, audit.Entry{Type: typ, Snippet: sn.Name + "@" + sn.Version, Command: sn.Command, Level: sn.Level.String(), Message: sn.Title})
	writeJSON(w, 200, sn)
}

func (s *Server) createSnippet(w http.ResponseWriter, r *http.Request) { s.saveSnippet(w, r, true) }
func (s *Server) updateSnippet(w http.ResponseWriter, r *http.Request) { s.saveSnippet(w, r, false) }

func (s *Server) deleteSnippet(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	if err := s.DeleteSnippet(name); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "snippet.deleted", Snippet: name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- 文件 ----------

func cleanRemote(p string) (string, bool) {
	if !strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) {
		return "", false
	}
	return path.Clean(p), true
}

func (s *Server) listDir(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, ok := cleanRemote(q.Get("path"))
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	entries, err := s.Kernel.ListDir(r.Context(), actor(sessionOf(r)), q.Get("host"), p)
	if err != nil {
		if errors.Is(err, executor.ErrNotExist) {
			writeErr(w, 404, "目录不存在")
			return
		}
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"path": p, "entries": entries})
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p, ok := cleanRemote(q.Get("path"))
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	data, err := s.Kernel.ReadFile(r.Context(), actor(sessionOf(r)), q.Get("host"), p)
	if err != nil {
		if errors.Is(err, executor.ErrNotExist) {
			writeErr(w, 404, "文件不存在")
			return
		}
		fail(w, err)
		return
	}
	out := map[string]any{"path": p, "size": len(data), "truncated": len(data) >= 1<<20}
	if utf8.Valid(data) {
		out["content"] = string(data)
	} else {
		out["binary"] = true
	}
	writeJSON(w, 200, out)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	var in struct{ Host, Path, Content, Intent string }
	if !readJSON(w, r, &in) {
		return
	}
	p, ok := cleanRemote(in.Path)
	if !ok {
		writeErr(w, 400, "路径必须是绝对路径")
		return
	}
	req, err := s.Kernel.SubmitFileWrite(r.Context(), actor(sessionOf(r)), in.Host, p, []byte(in.Content), strings.TrimSpace(in.Intent))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

// ---------- 审计 ----------

var auditTypeRe = regexp.MustCompile(`^[a-z][a-z.]{1,48}$`)

func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("size"))
	if size <= 0 || size > 200 {
		size = 20
	}
	if page < 0 {
		page = 0
	}
	var types []string
	for _, t := range strings.Split(q.Get("types"), ",") {
		if auditTypeRe.MatchString(t) && len(types) < 40 {
			types = append(types, t)
		}
	}
	recs, total, err := s.Kernel.Audit().Page(r.Context(), audit.Filter{Text: q.Get("q"), Types: types, All: q.Get("scope") == "all", Limit: size, Offset: page * size})
	if err != nil {
		writeErr(w, 500, "读取审计失败")
		return
	}
	writeJSON(w, 200, map[string]any{"records": recs, "total": total})
}

func (s *Server) auditVerify(w http.ResponseWriter, r *http.Request) {
	res, err := s.Kernel.VerifyAudit(r.Context())
	if err != nil {
		writeErr(w, 500, "校验失败："+err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// ---------- 系统 ----------

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	inv := s.Kernel.Inventory()
	writeJSON(w, 200, map[string]any{
		"version": s.Version, "dev": s.Dev, "paused": s.Kernel.Paused(),
		"hosts": len(inv.Hosts()), "groups": len(inv.Groups()), "snippets": len(s.Snippets()),
		"secretsAvailable": s.Secrets.Available(), "publicURL": s.PublicURL, "executors": s.Executors(),
		"agentArchs": agentbin.Available(),
	})
}

func (s *Server) panel(w http.ResponseWriter, r *http.Request) {
	v, err := s.Panel()
	if err != nil {
		writeErr(w, 500, "读取设置失败")
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) savePanel(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in map[string]string
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	next, err := s.SavePanel(in)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// 会话时长立即对新会话生效；监听地址和 TLS 要重启。
	s.Auth.SetSessionTTL(time.Duration(next.SessionMinutes) * time.Minute)
	s.audit(r, audit.Entry{Type: "system.panel.updated", Message: "修改面板设置（监听地址、TLS 等重启后生效）", Detail: map[string]any{"values": in}})
	s.panel(w, r)
}

func ownerOnly(w http.ResponseWriter, r *http.Request) bool {
	if sessionOf(r).User.Role != "owner" {
		writeErr(w, http.StatusForbidden, "只有所有者可以执行这个操作")
		return false
	}
	return true
}

func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	if err := s.Kernel.SetPaused(r.Context(), actor(sessionOf(r)), true); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"paused": true})
}

// auditRoutine 读取或修改“日常查看要不要写审计”。
func (s *Server) auditRoutine(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		if !ownerOnly(w, r) {
			return
		}
		var in struct {
			Record bool `json:"record"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		if err := s.Kernel.SetAuditRoutine(r.Context(), actor(sessionOf(r)), in.Record); err != nil {
			fail(w, err)
			return
		}
	}
	writeJSON(w, 200, map[string]any{"record": s.Kernel.AuditRoutine()})
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	if err := s.Kernel.SetPaused(r.Context(), actor(sessionOf(r)), false); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"paused": false})
}

func (s *Server) revokeSessions(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	s.audit(r, audit.Entry{Type: "system.sessions.revoked", Message: "吊销所有会话"})
	if err := s.Auth.RevokeAll(); err != nil {
		writeErr(w, 500, "吊销失败")
		return
	}
	s.clearSession(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) reload(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	if err := s.Reload(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "system.config.reloaded", Message: "重新加载主机清单、Snippet 和规则包"})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- 凭据（只写不读） ----------

func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	all, err := s.Secrets.List()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	// 登录用的密钥（动态码）也在凭据存储里，但它们不是给主机用的凭据：不列出来。
	list := all[:0]
	for _, m := range all {
		if !strings.HasPrefix(m.Name, "auth/") {
			list = append(list, m)
		}
	}
	writeJSON(w, 200, map[string]any{"secrets": list, "available": s.Secrets.Available(), "autoKey": s.Secrets.AutoKey()})
}

func (s *Server) putSSHSecret(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct{ Name, User, PrivateKey, Passphrase string }
	if !readJSON(w, r, &in) {
		return
	}
	if in.User == "" || !strings.Contains(in.PrivateKey, "PRIVATE KEY") {
		writeErr(w, 400, "需要登录用户名和 PEM 格式的私钥")
		return
	}
	if secrets.Reserved(in.Name) {
		writeErr(w, 400, "这个名字是系统自己用的，换一个")
		return
	}
	if in.User == "root" {
		writeErr(w, 400, "不要用 root：请在被管机上建一个受限账号，配合 sudo 白名单")
		return
	}
	body, _ := json.Marshal(executor.SSHCredential{User: in.User, PrivateKey: in.PrivateKey, Passphrase: in.Passphrase})
	if err := s.Secrets.Put(in.Name, body); err != nil {
		code := 400
		if errors.Is(err, secrets.ErrNoMasterKey) {
			code = http.StatusServiceUnavailable
		}
		writeErr(w, code, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "secret.updated", Message: "写入凭据 " + in.Name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.URL.Query().Get("name")
	if err := s.Secrets.Delete(name); err != nil {
		writeErr(w, 500, "删除失败")
		return
	}
	s.audit(r, audit.Entry{Type: "secret.deleted", Message: "删除凭据 " + name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- 实时通道 ----------

// stream 是一条多路复用的 WebSocket：请求状态、流式输出、主机状态、系统事件。
// 握手时校验 Origin（由 websocket.Accept 完成：来源必须与 Host 一致）。
// streamBeat 是实时通道发心跳的间隔。页面超过它的三倍没收到任何消息就认为连接断了。
var streamBeat = 15 * time.Second

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := c.CloseRead(r.Context())
	events, unsubscribe := s.Kernel.Subscribe()
	defer unsubscribe()
	check := time.NewTicker(30 * time.Second)
	defer check.Stop()
	// 心跳：浏览器看不到协议层的 ping，网络悄悄断掉时它那头的连接会一直显示“已连接”。
	// 定时发一条页面看得见的消息，页面隔久了收不到就自己重连（见 web/src/store.ts 的 streamSilence）。
	beat := time.NewTicker(streamBeat)
	defer beat.Stop()
	token := ""
	if ck, err := r.Cookie(cookieName); err == nil {
		token = ck.Value
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-check.C:
			// 会话被吊销或过期后，实时通道也要断开。
			if cur, err := s.Auth.Lookup(token); err != nil || cur.User.ID != sess.User.ID {
				c.Close(websocket.StatusPolicyViolation, "会话已失效")
				return
			}
			if err := c.Ping(ctx); err != nil {
				return
			}
		case <-beat.C:
			wctx, cancel := contextWithTimeout(ctx, 10*time.Second)
			err := c.Write(wctx, websocket.MessageText, []byte(`{"type":"ping"}`))
			cancel()
			if err != nil {
				return
			}
		case ev := <-events:
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			wctx, cancel := contextWithTimeout(ctx, 10*time.Second)
			err = c.Write(wctx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

var _ = auth.ErrUnauthorized

var sourceRepoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// sourceURL 是源代码的地址：发布构建编译进了仓库名（owner/name）就指向它，否则为空（自己编译的由编译的人负责告知）。
func (s *Server) sourceURL() string {
	if !sourceRepoRe.MatchString(s.SourceRepo) {
		return ""
	}
	return "https://github.com/" + s.SourceRepo
}
