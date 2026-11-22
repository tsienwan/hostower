package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/tsienwan/hostower/internal/apps"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 应用市场没有自己的执行通道：目录是编进可执行文件的清单，“已安装”是内核的只读查询，
// 安装、启停、卸载都渲染成每台主机上的一条命令，走和手敲命令完全相同的判级、审批、审计。
// 批量操作是一个请求（每台主机的命令可以不同，摘要覆盖每一条），一次审批。

const maxAppTargets = 50

type appView struct {
	*apps.App
	Permissions []risk.Finding `json:"permissions"`
}

func (s *Server) appCatalog(w http.ResponseWriter, r *http.Request) {
	list := []appView{}
	for _, a := range s.apps.List() {
		p := a.Permissions()
		if p == nil {
			p = []risk.Finding{}
		}
		list = append(list, appView{App: a, Permissions: p})
	}
	writeJSON(w, 200, map[string]any{"apps": list, "categories": apps.Categories})
}

type hostApps struct {
	Host      string          `json:"host"`
	Installed []installedView `json:"installed"`
	Error     string          `json:"error,omitempty"`
}

type installedView struct {
	apps.Installed
	// Update 是可以更新到的版本；没有更新时为空。
	Update string `json:"update,omitempty"`
	// Known 表示目录里有这个应用（没有的不能更新，只能启停和卸载）。
	Known bool `json:"known"`
	// Blocked 是有新版本、但不能原地升级过去（数据格式不兼容）时的那个版本；这时 Update 为空。
	Blocked string `json:"blocked,omitempty"`
	// Rebuilt 表示版本没变、但镜像标签在仓库里指向了新的构建（比如 :2 从 2.5.10 换成了 2.5.11）：更新就是重新拉取。
	Rebuilt bool `json:"rebuilt,omitempty"`
	// Remote 是仓库里的摘要（拼起来）：自动发起更新据此区分“同一个新构建”有没有提醒过。
	Remote string `json:"-"`
	// Upstream 是上游出现、但目录还没收录的大版本标签，只提示，不提供一键更新。
	Upstream string `json:"upstream,omitempty"`
	// Guide 是按这个实例的参数渲染好的上手指南（访问入口、初始账号、接下来几步）。
	Guide *apps.GuideView `json:"guide,omitempty"`
}

// installedOn 在几台主机上并行查询已安装的实例。
func (s *Server) installedOn(ctx context.Context, a kernel.Actor, hosts []string) []hostApps {
	out := make([]hostApps, len(hosts))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, h := range hosts {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = hostApps{Host: h, Installed: []installedView{}}
			res, _, err := s.Kernel.Query(ctx, a, h, apps.ListCommand)
			if err != nil {
				out[i].Error = err.Error()
				return
			}
			list, digests := apps.ParseListDigests(res)
			for _, x := range list {
				v := installedView{Installed: x}
				if app, ok := s.apps.Get(x.App); ok && app.Type == x.Type {
					v.Known = true
					// 主机上记的参数不合法时渲染不出来：不显示指南，别的照常。
					v.Guide, _ = app.GuideFor(x.Record)
					if t := s.appRegistry.Target(ctx, app); apps.Compare(t, x.Version) > 0 {
						if app.UpgradeBlocked(x.Version) {
							v.Blocked = t
						} else {
							v.Update = t
						}
					} else if x.Version == app.Version && app.Type == apps.TypeCompose {
						// 版本和目录一样：看镜像仓库里同一个标签有没有新构建、上游有没有新大版本。
						n := s.appRegistry.CheckImages(ctx, app.Images(), digests)
						v.Rebuilt, v.Remote, v.Upstream = n.Rebuilt, n.Remote, n.Major
					}
				}
				out[i].Installed = append(out[i].Installed, v)
			}
		}(i, h)
	}
	wg.Wait()
	return out
}

func (s *Server) appInstalled(w http.ResponseWriter, r *http.Request) {
	var hosts []string
	seen := map[string]bool{}
	for _, h := range strings.Split(r.URL.Query().Get("hosts"), ",") {
		if h = strings.TrimSpace(h); h != "" && !seen[h] {
			seen[h] = true
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		for _, h := range s.Kernel.Inventory().Hosts() {
			hosts = append(hosts, h.Name)
		}
	}
	if len(hosts) > maxAppTargets {
		writeErr(w, 400, "一次最多查询 "+strconv.Itoa(maxAppTargets)+" 台主机")
		return
	}
	writeJSON(w, 200, map[string]any{"hosts": s.installedOn(r.Context(), actor(sessionOf(r)), hosts)})
}

// appReveal 读一个实例的初始账号：清单里写好的一条只读命令（比如从容器日志里找第一次启动时生成的密码）。
// 实例是什么应用、什么参数，以主机上的 app.json 为准；命令照常过判级，不是 L0 就被内核拒绝。
func (s *Server) appReveal(w http.ResponseWriter, r *http.Request) {
	host, inst := r.URL.Query().Get("host"), r.URL.Query().Get("instance")
	if !apps.InstanceRe.MatchString(inst) {
		writeErr(w, 400, "实例名不合法")
		return
	}
	if _, ok := s.Kernel.Inventory().Host(host); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	a := actor(sessionOf(r))
	ha := s.installedOn(r.Context(), a, []string{host})[0]
	if ha.Error != "" {
		writeErr(w, 400, ha.Error)
		return
	}
	for _, x := range ha.Installed {
		if x.Instance != inst {
			continue
		}
		app, ok := s.apps.Get(x.App)
		if !ok || app.Type != x.Type {
			break
		}
		cmd, err := app.RevealCommand(x.Record)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		out, _, err := s.Kernel.Query(r.Context(), a, host, cmd)
		if err != nil {
			fail(w, err)
			return
		}
		if len(out) > 4096 {
			out = out[len(out)-4096:]
		}
		writeJSON(w, 200, map[string]any{"output": strings.TrimSpace(out)})
		return
	}
	writeErr(w, 404, "这台主机上没有这个实例，或者它没有可以读取的初始信息")
}

// ---------- 安装 ----------

type appInstallTarget struct {
	Host     string            `json:"host"`
	Instance string            `json:"instance"`
	Params   map[string]string `json:"params"`
	Secrets  map[string]string `json:"secrets"` // 参数名 → 应用密钥名
	Version  string            `json:"version"`
	// AI 是 AI 命令行工具装完顺带做的配置（API Key、供应商、模型）；为空表示只装不配。
	AI *apps.AIConfig `json:"ai"`
}

type appInstallBody struct {
	App      string             `json:"app"`
	Targets  []appInstallTarget `json:"targets"`
	Preview  bool               `json:"preview"`
	Strategy kernel.Strategy    `json:"strategy"`
}

// plannedTarget 是一台主机上渲染好的命令。
type plannedTarget struct {
	host, command, compose string
	input                  *kernel.Input
	group                  string // 同一组的主机用同一份标准输入，可以并成一个请求
}

func (s *Server) appInstall(w http.ResponseWriter, r *http.Request) {
	var in appInstallBody
	if !readJSON(w, r, &in) {
		return
	}
	a, ok := s.apps.Get(in.App)
	if !ok {
		writeErr(w, 404, "应用不存在")
		return
	}
	if len(in.Targets) == 0 || len(in.Targets) > maxAppTargets {
		writeErr(w, 400, "请选择 1 到 "+strconv.Itoa(maxAppTargets)+" 台主机")
		return
	}
	var plan []plannedTarget
	seen := map[string]bool{}
	version := ""
	for _, t := range in.Targets {
		if seen[t.Host] {
			writeErr(w, 400, "主机 "+t.Host+" 选了两次")
			return
		}
		seen[t.Host] = true
		inst := t.Instance
		if a.Type == apps.TypeNpm || inst == "" {
			inst = a.Name
		}
		values, err := a.Values(t.Params)
		if err != nil {
			writeErr(w, 400, t.Host+"："+err.Error())
			return
		}
		p, err := a.PlanInstall(inst, values, t.Secrets, t.Version)
		if err != nil {
			writeErr(w, 400, t.Host+"："+err.Error())
			return
		}
		version = p.Version
		pt := plannedTarget{host: t.Host, command: p.Command, compose: p.Compose}
		if err := s.attachSecrets(&pt, inst, p.Secrets); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if t.AI != nil && a.Configure != nil {
			// 装完接着配置：安装脚本失败会直接退出，配置不会执行。
			rec := apps.Record{App: a.Name, Type: a.Type, Instance: inst, Version: p.Version, Bins: a.Npm.Bins}
			ap, err := a.PlanAIConfig(rec, *t.AI)
			if err != nil {
				writeErr(w, 400, t.Host+"："+err.Error())
				return
			}
			pt.command += "; " + ap.Command
			if err := s.attachAIKey(&pt, inst, ap); err != nil {
				writeErr(w, 400, err.Error())
				return
			}
		}
		plan = append(plan, pt)
	}
	intent := func(n int) string {
		if n == 1 {
			return "安装应用 " + a.Title + "（" + version + "）"
		}
		return "在 " + strconv.Itoa(n) + " 台主机上安装应用 " + a.Title + "（" + version + "）"
	}
	s.submitPlan(w, r, plan, in.Preview, in.Strategy, intent, "app:"+a.Name+"@"+a.Version)
}

// submitPlan 预览或提交渲染好的命令。标准输入相同的主机并成一个请求。
func (s *Server) submitPlan(w http.ResponseWriter, r *http.Request, plan []plannedTarget, preview bool, st kernel.Strategy, intent func(n int) string, source string) {
	if preview {
		type target struct {
			Host     string         `json:"host"`
			Command  string         `json:"command"`
			Compose  string         `json:"compose,omitempty"`
			Level    risk.Level     `json:"level"`
			Findings []risk.Finding `json:"findings"`
		}
		inv := s.Kernel.Inventory()
		lvl := risk.L0
		var out []target
		for _, p := range plan {
			h, ok := inv.Host(p.host)
			if !ok {
				writeErr(w, 400, "主机 "+p.host+" 不存在")
				return
			}
			l, fs := s.Kernel.Assess(h, p.command, risk.L0)
			lvl = risk.Max(lvl, l)
			out = append(out, target{Host: p.host, Command: p.command, Compose: p.compose, Level: l, Findings: fs})
		}
		writeJSON(w, 200, map[string]any{"level": lvl, "targets": out, "requests": len(groups(plan))})
		return
	}
	reqs, err := s.submitPlanned(r.Context(), actor(sessionOf(r)), plan, st, intent, source)
	switch {
	case err != nil && len(reqs) == 0:
		fail(w, err)
	case err != nil:
		// 前面的请求已经提交了：把它们和这次的错误一起告诉页面。
		writeJSON(w, 200, map[string]any{"requests": reqs, "error": err.Error()})
	default:
		writeJSON(w, 200, map[string]any{"requests": reqs})
	}
}

// submitPlanned 把渲染好的命令提交给内核：标准输入相同的主机并成一个请求。出错时返回已经提交的那些。
func (s *Server) submitPlanned(ctx context.Context, a kernel.Actor, plan []plannedTarget, st kernel.Strategy, intent func(n int) string, source string) ([]*kernel.Request, error) {
	if st.Concurrency <= 0 {
		st.Concurrency = 4
	}
	var reqs []*kernel.Request
	for _, g := range groups(plan) {
		spec := kernel.CommandSpec{Commands: map[string]string{}, Intent: intent(len(g)), Snippet: source, Strategy: st}
		for _, p := range g {
			spec.Hosts = append(spec.Hosts, p.host)
			spec.Commands[p.host] = p.command
			spec.Input = p.input
		}
		req, err := s.Kernel.SubmitCommand(ctx, a, spec)
		if err != nil {
			return reqs, err
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

func groups(plan []plannedTarget) [][]plannedTarget {
	var order []string
	by := map[string][]plannedTarget{}
	for _, p := range plan {
		if _, ok := by[p.group]; !ok {
			order = append(order, p.group)
		}
		by[p.group] = append(by[p.group], p)
	}
	out := make([][]plannedTarget, 0, len(order))
	for _, k := range order {
		out = append(out, by[k])
	}
	return out
}

// ---------- 启停、更新、卸载 ----------

type appTarget struct {
	Host     string `json:"host"`
	Instance string `json:"instance"`
}

type appActionBody struct {
	Action   string          `json:"action"`
	Version  string          `json:"version"` // 只对更新命令行工具有效：空表示更新到最新
	Targets  []appTarget     `json:"targets"`
	Preview  bool            `json:"preview"`
	Strategy kernel.Strategy `json:"strategy"`
}

const actUpdate = "update"

var appActionNames = map[string]string{
	apps.ActStart: "启动", apps.ActStop: "停止", apps.ActRestart: "重启", actUpdate: "更新",
	apps.ActUninstall: "卸载", apps.ActPurge: "卸载并删除数据：",
}

// actionPlan 是一次对若干实例的操作，渲染好了每台主机上的命令。
type actionPlan struct {
	plan   []plannedTarget
	intent func(n int) string
}

// planAppAction 渲染对若干实例的操作。实例是什么应用、什么类型，以主机上的 app.json 为准（只读查询），不信调用方传来的。
func (s *Server) planAppAction(ctx context.Context, a kernel.Actor, action, version string, targets []appTarget) (*actionPlan, error) {
	verb, ok := appActionNames[action]
	if !ok {
		return nil, errors.New("未知的操作")
	}
	if len(targets) == 0 || len(targets) > maxAppTargets {
		return nil, errors.New("请选择 1 到 " + strconv.Itoa(maxAppTargets) + " 个实例")
	}
	var hosts []string
	want := map[string][]string{}
	for _, t := range targets {
		if !apps.InstanceRe.MatchString(t.Instance) {
			return nil, errors.New("实例名不合法")
		}
		if _, ok := want[t.Host]; !ok {
			hosts = append(hosts, t.Host)
		}
		want[t.Host] = append(want[t.Host], t.Instance)
	}
	var plan []plannedTarget
	names := map[string]bool{}
	for _, ha := range s.installedOn(ctx, a, hosts) {
		if ha.Error != "" {
			return nil, errors.New(ha.Host + "：" + ha.Error)
		}
		var cmds []string
		for _, inst := range want[ha.Host] {
			var rec *apps.Record
			for i := range ha.Installed {
				if ha.Installed[i].Instance == inst {
					rec = &ha.Installed[i].Record
				}
			}
			if rec == nil {
				return nil, fmt.Errorf("%s 上没有实例 %s", ha.Host, inst)
			}
			app, known := s.apps.Get(rec.App)
			var c string
			if action == actUpdate {
				if !known || app.Type != rec.Type {
					return nil, errors.New(inst + "：目录里没有这个应用，不能更新")
				}
				v := version
				if v == "" {
					v = s.appRegistry.Target(ctx, app)
				}
				p, err := app.PlanUpdate(*rec, v)
				if err != nil {
					return nil, errors.New(inst + "：" + err.Error())
				}
				c = p.Command
				names[inst+" → "+p.Version] = true
			} else {
				pkg := ""
				if known && app.Npm != nil {
					pkg = app.Npm.Package
				}
				var err error
				if c, err = apps.PlanAction(*rec, pkg, action); err != nil {
					return nil, errors.New(inst + "：" + err.Error())
				}
				names[inst] = true
			}
			cmds = append(cmds, c)
		}
		// 同一台主机上的几个实例依次执行，前一个失败就停。
		// 右括号单独一行：命令里可能有 heredoc，结束标记那一行后面不能再接别的。
		cmd := cmds[0]
		if len(cmds) > 1 {
			parts := make([]string, len(cmds))
			for i, c := range cmds {
				parts[i] = "{ " + c + "\n}"
			}
			cmd = strings.Join(parts, " && ")
		}
		plan = append(plan, plannedTarget{host: ha.Host, command: cmd})
	}
	list := make([]string, 0, len(names))
	for n := range names {
		list = append(list, n)
	}
	sort.Strings(list)
	return &actionPlan{plan: plan, intent: func(n int) string {
		s := verb + "应用实例 " + strings.Join(list, "、")
		if n > 1 {
			s += "（" + strconv.Itoa(n) + " 台主机）"
		}
		return s
	}}, nil
}

func (s *Server) appAction(w http.ResponseWriter, r *http.Request) {
	var in appActionBody
	if !readJSON(w, r, &in) {
		return
	}
	p, err := s.planAppAction(r.Context(), actor(sessionOf(r)), in.Action, in.Version, in.Targets)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.submitPlan(w, r, p.plan, in.Preview, in.Strategy, p.intent, "")
}

// attachSecrets 把“变量名 → 应用密钥名”变成送进标准输入的 .env（只有引用，没有值），并按密钥分组。
func (s *Server) attachSecrets(pt *plannedTarget, inst string, secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}
	input := &kernel.Input{Kind: "dotenv", Label: inst + "/.env"}
	envs := make([]string, 0, len(secrets))
	for env := range secrets {
		envs = append(envs, env)
	}
	sort.Strings(envs)
	for _, env := range envs {
		name := secrets[env]
		if _, err := s.Secrets.Get("app/" + name); err != nil {
			return errors.New("应用密钥 " + name + " 不存在或读不出来")
		}
		input.Env = append(input.Env, kernel.EnvRef{Env: env, Ref: "app/" + name})
		pt.group += env + "=" + name + ";"
	}
	pt.input = input
	return nil
}

// ---------- 修改实例设置 ----------

type appConfigureBody struct {
	Host     string            `json:"host"`
	Instance string            `json:"instance"`
	Params   map[string]string `json:"params"`
	Secrets  map[string]string `json:"secrets"` // 参数名 → 应用密钥名；为空表示密钥不变
	// AI 是 AI 命令行工具的配置（命令行工具用这个，不用 Params）。
	AI      *apps.AIConfig `json:"ai"`
	Preview bool           `json:"preview"`
}

// appConfigure 修改一个容器应用实例的设置：按新参数重新生成 compose 文件，docker compose up -d 让它生效。
// 实例的现状以主机上的 app.json 为准（只读查询）；改动照常走判级和审批。
func (s *Server) appConfigure(w http.ResponseWriter, r *http.Request) {
	var in appConfigureBody
	if !readJSON(w, r, &in) {
		return
	}
	if !apps.InstanceRe.MatchString(in.Instance) {
		writeErr(w, 400, "实例名不合法")
		return
	}
	ha := s.installedOn(r.Context(), actor(sessionOf(r)), []string{in.Host})[0]
	if ha.Error != "" {
		writeErr(w, 400, ha.Host+"："+ha.Error)
		return
	}
	var rec *apps.Record
	for i := range ha.Installed {
		if ha.Installed[i].Instance == in.Instance {
			rec = &ha.Installed[i].Record
		}
	}
	if rec == nil {
		writeErr(w, 400, fmt.Sprintf("%s 上没有实例 %s", in.Host, in.Instance))
		return
	}
	a, ok := s.apps.Get(rec.App)
	if !ok {
		writeErr(w, 400, in.Instance+"：目录里没有这个应用，不能修改设置")
		return
	}
	var pt plannedTarget
	if rec.Type == apps.TypeNpm {
		if in.AI == nil {
			writeErr(w, 400, "缺少配置")
			return
		}
		ap, err := a.PlanAIConfig(*rec, *in.AI)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		pt = plannedTarget{host: in.Host, command: ap.Command}
		if err := s.attachAIKey(&pt, in.Instance, ap); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	} else {
		p, err := a.PlanConfigure(*rec, in.Params, in.Secrets)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		pt = plannedTarget{host: in.Host, command: p.Command, compose: p.Compose}
		if err := s.attachSecrets(&pt, in.Instance, p.Secrets); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	intent := func(int) string { return "修改应用实例 " + in.Instance + " 的设置" }
	s.submitPlan(w, r, []plannedTarget{pt}, in.Preview, kernel.Strategy{}, intent, "app:"+a.Name+"@"+a.Version)
}

// attachAIKey 把 AI 工具的 API Key 接到标准输入上：写进 secret.env 的一行，或者原样交给 codex login。只有引用，没有值。
func (s *Server) attachAIKey(pt *plannedTarget, inst string, ap *apps.AIPlan) error {
	if ap.KeyRef == "" {
		return nil
	}
	if _, err := s.Secrets.Get(ap.KeyRef); err != nil {
		return errors.New("应用密钥 " + strings.TrimPrefix(ap.KeyRef, "app/") + " 不存在或读不出来")
	}
	if ap.Dotenv {
		pt.input = &kernel.Input{Kind: "dotenv", Label: inst + "/secret.env", Env: []kernel.EnvRef{{Env: ap.KeyEnv, Ref: ap.KeyRef}}}
	} else {
		pt.input = &kernel.Input{Kind: "secret", Label: inst + " 的 API Key", Ref: ap.KeyRef}
	}
	pt.group += "ai:" + ap.KeyEnv + "=" + ap.KeyRef + ";"
	return nil
}
