// Package apps 是应用市场：一份份声明式的应用清单，以及把清单渲染成安装、卸载命令的逻辑。
//
// 这里不碰主机。渲染出来的命令交给网关，由网关照常走 kernel.SubmitCommand：判级、审批、执行、审计。
// 应用能拿到多大的权限不由清单自报，而由判级引擎看实际写进 compose 文件的内容决定（见 risk.ComposeFindings）。
//
// 主机上的布局：
//
//	~/.hostower/apps/<实例>/compose.yaml  容器应用的 compose 文件
//	~/.hostower/apps/<实例>/.env          应用密钥（只有登录账号可读，内容从标准输入写进去）
//	~/.hostower/apps/<实例>/app.json      装的是哪个应用、哪个版本；“已安装”列表就读它
//	~/.hostower/npm/                      命令行工具（npm 包）装在这里，不用 sudo
//	~/.local/bin/<命令>                    命令行工具的启动脚本
package apps

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/snippet"
)

//go:embed catalog/*.yaml catalog/icons/*.svg
var catalogFS embed.FS

// 应用的种类。
const (
	TypeCompose = "compose" // 用 docker compose 跑的容器应用
	TypeNpm     = "npm"     // 装进当前账号目录的 npm 命令行工具
)

// Categories 是应用分类，按显示顺序排列。
var Categories = []string{"AI 工具", "监控", "数据库", "网络", "运维"}

// Icons 是清单里能用的图标名（前端有对应的图标）。
var Icons = []string{"bot", "sparkles", "activity", "database", "network", "box", "shield", "terminal"}

// App 是一份应用清单。
type App struct {
	APIVersion string `yaml:"apiVersion" json:"-"`
	Kind       string `yaml:"kind" json:"-"`
	Name       string `yaml:"name" json:"name"`
	Version    string `yaml:"version" json:"version"`
	Title      string `yaml:"title" json:"title"`
	Category   string `yaml:"category" json:"category"`
	Icon       string `yaml:"icon" json:"icon"` // 没有官方图标时用的通用图标
	// Logo 是官方图标（catalog/icons/<name>.svg），以 data: 地址给页面显示。
	Logo        string  `yaml:"-" json:"logo,omitempty"`
	Summary     string  `yaml:"summary" json:"summary"`
	Description string  `yaml:"description" json:"description"`
	Preview     bool    `yaml:"preview" json:"preview,omitempty"` // 上游还是预览版
	Type        string  `yaml:"type" json:"type"`
	Params      []Param `yaml:"params" json:"params"`
	Compose     string  `yaml:"compose" json:"-"`
	Npm         *Npm    `yaml:"npm" json:"npm,omitempty"`
	// Upgrade 是升级说明：可以原地升级的给出提示，不能原地升级的（数据格式不兼容）列在 BlockFrom 里。
	Upgrade *Upgrade `yaml:"upgrade" json:"upgrade,omitempty"`
	// Configure 是 AI 命令行工具的配置方式（API Key、模型供应商、订阅登录），见 ai.go。
	Configure *Configure `yaml:"configure" json:"-"`
	AI        *AIView    `yaml:"-" json:"ai,omitempty"`
	Notes     string     `yaml:"notes" json:"notes,omitempty"`
}

// Upgrade 是升级说明。
type Upgrade struct {
	Note string `yaml:"note" json:"note,omitempty"`
	// BlockFrom 是不能原地升级到目录版本的旧版本前缀（例如 PostgreSQL 的 "17-"）：这些实例不提供一键更新。
	BlockFrom []string `yaml:"blockFrom" json:"blockFrom,omitempty"`
}

// UpgradeBlocked 报告从 from 版本能不能原地升级到目录里的版本。
func (a *App) UpgradeBlocked(from string) bool {
	if a.Upgrade == nil || from == a.Version {
		return false
	}
	for _, p := range a.Upgrade.BlockFrom {
		if strings.HasPrefix(from, p) {
			return true
		}
	}
	return false
}

func (a *App) blockedErr(from string) error {
	note := ""
	if a.Upgrade != nil {
		note = a.Upgrade.Note
	}
	return fmt.Errorf("%s 不能从 %s 直接升级到 %s：%s", a.Title, from, a.Version, note)
}

// Npm 描述一个 npm 命令行工具。
type Npm struct {
	Package string   `yaml:"package" json:"package"`
	Version string   `yaml:"version" json:"version"` // 默认装的版本；安装时可以换
	Bins    []string `yaml:"bins" json:"bins"`
	Node    int      `yaml:"node" json:"node"` // 需要的 Node.js 主版本下限
}

// Param 是安装时要填的一项。
type Param struct {
	Name    string   `yaml:"name" json:"name"`
	Title   string   `yaml:"title" json:"title"`
	Type    string   `yaml:"type" json:"type"` // port | string | enum | secret
	Default string   `yaml:"default" json:"default,omitempty"`
	Pattern string   `yaml:"pattern" json:"-"`
	Options []string `yaml:"options" json:"options,omitempty"`
	Hint    string   `yaml:"hint" json:"hint,omitempty"`
	// Env 是 secret 写进 .env 的变量名，compose 文件里用 ${Env} 引用它。
	Env string `yaml:"env" json:"env,omitempty"`
	re  *regexp.Regexp
}

var (
	appNameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
	paramNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	envNameRe   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	// InstanceRe 是实例名：也是主机上的目录名和 compose 项目名。
	InstanceRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
	// SecretNameRe 和网关里应用密钥的名字规则一致。
	SecretNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
	npmPkgRe     = regexp.MustCompile(`^(@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
	binRe        = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	// VersionRe 是 npm 工具能换成的版本：具体版本号或 latest。
	VersionRe = regexp.MustCompile(`^(latest|[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?)$`)
	// 镜像必须带明确的标签：不接受隐含的 latest。
	imageTagRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+$`)
)

// Catalog 是内置的应用目录，随版本发布（被发布签名覆盖）。
type Catalog struct {
	apps []*App
	by   map[string]*App
}

// Builtin 读出编进可执行文件的应用目录。清单有错就是发布前的 bug，测试会拦住。
func Builtin() (*Catalog, error) {
	c := &Catalog{by: map[string]*App{}}
	files, _ := fs.Glob(catalogFS, "catalog/*.yaml")
	for _, f := range files {
		data, err := catalogFS.ReadFile(f)
		if err != nil {
			return nil, err
		}
		a, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", f, err)
		}
		if svg, err := catalogFS.ReadFile("catalog/icons/" + a.Name + ".svg"); err == nil {
			if err := checkSVG(svg); err != nil {
				return nil, fmt.Errorf("%s 的图标：%w", a.Name, err)
			}
			a.Logo = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(svg)
		}
		a.AI = a.aiView()
		if c.by[a.Name] != nil {
			return nil, fmt.Errorf("%s：应用 %s 重复", f, a.Name)
		}
		c.apps = append(c.apps, a)
		c.by[a.Name] = a
	}
	cat := map[string]int{}
	for i, x := range Categories {
		cat[x] = i
	}
	sort.SliceStable(c.apps, func(i, j int) bool {
		if cat[c.apps[i].Category] != cat[c.apps[j].Category] {
			return cat[c.apps[i].Category] < cat[c.apps[j].Category]
		}
		return c.apps[i].Name < c.apps[j].Name
	})
	return c, nil
}

// List 返回全部应用。
func (c *Catalog) List() []*App { return c.apps }

// Get 按名字找应用。
func (c *Catalog) Get(name string) (*App, bool) {
	a, ok := c.by[name]
	return a, ok
}

var svgEventRe = regexp.MustCompile(`\son[a-z]+\s*=`)

// checkSVG：图标只能是简单的矢量图，不能带脚本、外部引用或嵌入的 HTML。
func checkSVG(b []byte) error {
	s := strings.ToLower(string(b))
	if len(b) > 8<<10 || !strings.HasPrefix(s, "<svg ") {
		return errors.New("要是不超过 8 KB 的 SVG")
	}
	for _, bad := range []string{"<script", "href", "foreignobject", "javascript:", "<iframe", "<image", "@import", "url("} {
		if strings.Contains(s, bad) {
			return errors.New("不能含有 " + bad)
		}
	}
	if svgEventRe.MatchString(s) {
		return errors.New("不能含有事件属性")
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Parse 解析并校验一份应用清单。
func Parse(data []byte) (*App, error) {
	var a App
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&a); err != nil {
		return nil, err
	}
	if a.APIVersion != "hostower/v1" || a.Kind != "App" {
		return nil, errors.New("应用清单需要 apiVersion: hostower/v1 与 kind: App")
	}
	if !appNameRe.MatchString(a.Name) || a.Version == "" || a.Title == "" || a.Summary == "" {
		return nil, errors.New("应用清单需要 name、version、title、summary")
	}
	if !contains(Categories, a.Category) {
		return nil, fmt.Errorf("未知的分类 %q", a.Category)
	}
	if !contains(Icons, a.Icon) {
		return nil, fmt.Errorf("未知的图标 %q", a.Icon)
	}
	seen := map[string]bool{}
	for i := range a.Params {
		p := &a.Params[i]
		if !paramNameRe.MatchString(p.Name) || seen[p.Name] || p.Title == "" {
			return nil, fmt.Errorf("参数 %q 不合法或重复", p.Name)
		}
		seen[p.Name] = true
		switch p.Type {
		case "port":
			if p.Default != "" && !validPort(p.Default) {
				return nil, fmt.Errorf("参数 %s 的默认端口不合法", p.Name)
			}
		case "enum":
			if len(p.Options) == 0 || p.Default != "" && !contains(p.Options, p.Default) {
				return nil, fmt.Errorf("参数 %s 需要选项", p.Name)
			}
			for _, o := range p.Options {
				if !plainValue(o) {
					return nil, fmt.Errorf("参数 %s 的选项 %q 不合法", p.Name, o)
				}
			}
		case "string":
			if p.Pattern == "" {
				return nil, fmt.Errorf("参数 %s 需要 pattern", p.Name)
			}
			re, err := regexp.Compile("^(?:" + p.Pattern + ")$")
			if err != nil {
				return nil, fmt.Errorf("参数 %s：%w", p.Name, err)
			}
			p.re = re
		case "secret":
			if a.Type != TypeCompose || !envNameRe.MatchString(p.Env) || p.Default != "" {
				return nil, fmt.Errorf("密钥参数 %s 需要 env，不能有默认值，只能用在容器应用里", p.Name)
			}
		default:
			return nil, fmt.Errorf("参数 %s 的类型 %q 不认识", p.Name, p.Type)
		}
	}
	switch a.Type {
	case TypeCompose:
		if a.Compose == "" || a.Npm != nil {
			return nil, errors.New("容器应用需要 compose，不能有 npm")
		}
		if _, err := a.render(a.defaults(), "check"); err != nil {
			return nil, err
		}
	case TypeNpm:
		n := a.Npm
		if n == nil || a.Compose != "" || !npmPkgRe.MatchString(n.Package) || !VersionRe.MatchString(n.Version) || n.Version == "latest" || n.Node < 16 || n.Node > NodeMajor || len(n.Bins) == 0 {
			return nil, errors.New("命令行工具需要 npm.package、固定的 npm.version、npm.node（16 到内置 Node 的主版本）和 npm.bins")
		}
		for _, b := range n.Bins {
			if !binRe.MatchString(b) {
				return nil, fmt.Errorf("命令名 %q 不合法", b)
			}
		}
		if len(a.Params) > 0 {
			return nil, errors.New("命令行工具不接受参数")
		}
		if a.Configure != nil && aiProviders[a.Configure.Kind] == nil {
			return nil, fmt.Errorf("不认识的配置方式 %q", a.Configure.Kind)
		}
	default:
		return nil, fmt.Errorf("未知的应用类型 %q", a.Type)
	}
	return &a, nil
}

func validPort(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 1 && n <= 65535 && strconv.Itoa(n) == s
}

// plainValue：能原样放进 YAML 而不改变结构的值。
var plainRe = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]{1,128}$`)

func plainValue(s string) bool { return plainRe.MatchString(s) }

func (a *App) defaults() map[string]string {
	v := map[string]string{}
	for _, p := range a.Params {
		switch p.Type {
		case "secret":
			v[p.Name] = "${" + p.Env + "}"
		case "string":
			v[p.Name] = p.Default
			if v[p.Name] == "" {
				v[p.Name] = "x"
			}
		default:
			v[p.Name] = p.Default
		}
	}
	return v
}

// Values 校验安装时填的参数（不含密钥）。缺省的用默认值。
func (a *App) Values(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range a.Params {
		if p.Type == "secret" {
			continue
		}
		v := strings.TrimSpace(in[p.Name])
		if v == "" {
			v = p.Default
		}
		switch p.Type {
		case "port":
			if !validPort(v) {
				return nil, fmt.Errorf("%s：端口要在 1 到 65535 之间", p.Title)
			}
		case "enum":
			if !contains(p.Options, v) {
				return nil, fmt.Errorf("%s：不是可选的值", p.Title)
			}
		case "string":
			if !p.re.MatchString(v) || !plainValue(v) {
				return nil, fmt.Errorf("%s：格式不对", p.Title)
			}
		}
		out[p.Name] = v
	}
	return out, nil
}

// render 把 compose 模板渲染成最终的文件：填参数、给每个服务打上 com.hostower.app 标签、定好容器名。
// 容器名不用 compose 默认的“项目名-服务名-序号”（实例名和服务名常常一样，会变成 3x-ui-3x-ui-1）：
// 只有一个服务时就叫实例名，多个服务时叫“实例名-服务名”。
func (a *App) render(values map[string]string, instance string) (string, error) {
	tpl, err := template.New(a.Name).Option("missingkey=error").Parse(a.Compose)
	if err != nil {
		return "", fmt.Errorf("compose 模板：%w", err)
	}
	vals := map[string]string{"instance": instance}
	for k, v := range values {
		vals[k] = v
	}
	for _, p := range a.Params {
		if p.Type == "secret" {
			vals[p.Name] = "${" + p.Env + "}"
		}
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vals); err != nil {
		return "", fmt.Errorf("compose 模板：%w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(buf.Bytes(), &doc); err != nil || len(doc.Content) == 0 {
		return "", errors.New("compose 模板渲染后不是合法的 YAML")
	}
	root := doc.Content[0]
	services := mapGet(root, "services")
	if services == nil || services.Kind != yaml.MappingNode || len(services.Content) == 0 {
		return "", errors.New("compose 文件里没有服务")
	}
	single := len(services.Content) == 2
	for i := 0; i+1 < len(services.Content); i += 2 {
		svc := services.Content[i+1]
		name := services.Content[i].Value
		if mapGet(svc, "build") != nil {
			return "", fmt.Errorf("服务 %s：只能用现成的镜像，不能现场构建", name)
		}
		img := mapGet(svc, "image")
		if img == nil || !imageTagRe.MatchString(img.Value) || strings.HasSuffix(img.Value, ":latest") {
			return "", fmt.Errorf("服务 %s：镜像要写明版本标签", name)
		}
		if mapGet(svc, "labels") != nil || mapGet(svc, "container_name") != nil {
			return "", fmt.Errorf("服务 %s：labels 和 container_name 由 Hostower 管理，清单里不要写", name)
		}
		ctr := instance
		if !single {
			ctr = instance + "-" + name
		}
		svc.Content = append(svc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "container_name"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: ctr, Style: yaml.DoubleQuotedStyle},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "labels"},
			&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: LabelKey},
				{Kind: yaml.ScalarNode, Value: instance, Style: yaml.DoubleQuotedStyle},
			}})
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", err
	}
	s := out.String()
	if strings.Contains(s, heredocTag) {
		return "", errors.New("compose 文件里不能出现结束标记")
	}
	return s, nil
}

func mapGet(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// Permissions 是这个应用（按默认参数）会拿到的宿主机级权限，给目录页提前展示。
// 真正的判级在提交时由判级引擎对实际内容再做一次。
func (a *App) Permissions() []risk.Finding {
	if a.Type != TypeCompose {
		return nil
	}
	s, err := a.render(a.defaults(), a.Name)
	if err != nil {
		return []risk.Finding{{Level: risk.L2, Rule: "compose.escape", Reason: err.Error()}}
	}
	return risk.ComposeFindings(s)
}

// ---------- 渲染成命令 ----------

// LabelKey 是打在每个容器上的标签，值是实例名。
const LabelKey = "com.hostower.app"

const heredocTag = "HOSTOWER_EOF"

// 命令行工具缺 Node.js 时装的固定版本（官方发布包，校验 SHA-256）。
const (
	NodeVersion = "24.22.3"
	NodeMajor   = 24
)

var nodeBuilds = []struct{ uname, arch, sha256 string }{
	{"x86_64|amd64", "x64", "061ec3e3062115705b56b51fa9cc330ab50d34b6969387d4e4b1a50c6809acc2"},
	{"aarch64|arm64", "arm64", "261af2af250dfd7e9f25cf151d1b2713a0adf8f10a9ad116c4947f68627affe8"},
}

// Install 是渲染好的安装请求。
type Install struct {
	Instance string
	Command  string
	Compose  string            // 容器应用最终写进主机的 compose 文件
	Secrets  map[string]string // 变量名 → 应用密钥名
	Version  string            // 实际装的版本
}

// Record 是写在主机上的 app.json。
type Record struct {
	App      string            `json:"app"`
	Version  string            `json:"version"`
	Type     string            `json:"type"`
	Instance string            `json:"instance"`
	Params   map[string]string `json:"params,omitempty"`
	Bins     []string          `json:"bins,omitempty"`
	// Envs 是写进 .env 的变量名（只有名字）。更新时据此知道主机上已经有哪些密钥。
	Envs []string `json:"envs,omitempty"`
	// Config 是 AI 命令行工具的配置（供应商、接口地址、模型、用的哪个应用密钥），记在实例目录的 config.json 里。
	Config map[string]string `json:"config,omitempty"`
}

func instanceDir(inst string) string { return "~/.hostower/apps/" + inst }

// purgeHelperImage 是删除实例目录时借用的临时容器镜像（只在当前账号和 sudo 都删不掉时才用）。
const purgeHelperImage = "busybox:1.37"

// PlanInstall 渲染安装命令。values 是 Values 校验过的参数，secrets 是“参数名 → 应用密钥名”，
// version 只对命令行工具有效（空表示清单里的默认版本）。
func (a *App) PlanInstall(instance string, values, secrets map[string]string, version string) (*Install, error) {
	if !InstanceRe.MatchString(instance) {
		return nil, errors.New("实例名只能用小写字母、数字和连字符，最长 31 个字符")
	}
	in := &Install{Instance: instance, Secrets: map[string]string{}}
	rec := Record{App: a.Name, Type: a.Type, Instance: instance}
	switch a.Type {
	case TypeCompose:
		for _, p := range a.Params {
			if p.Type != "secret" {
				continue
			}
			s := secrets[p.Name]
			if !SecretNameRe.MatchString(s) {
				return nil, fmt.Errorf("%s：请选择一个应用密钥", p.Title)
			}
			in.Secrets[p.Env] = s
			rec.Envs = append(rec.Envs, p.Env)
		}
		compose, err := a.render(values, instance)
		if err != nil {
			return nil, err
		}
		in.Compose, in.Version = compose, a.Version
		rec.Version, rec.Params = a.Version, values
		dir := instanceDir(instance)
		var b strings.Builder
		b.WriteString("docker compose version >/dev/null 2>&1 || { echo 'hostower: 这台主机没有 Docker Compose v2（docker compose 命令）'; exit 3; }; ")
		b.WriteString("umask 077 && mkdir -p " + dir + " && cd " + dir + " && ")
		if len(in.Secrets) > 0 {
			b.WriteString("cat > .env && ")
		}
		b.WriteString("printf '%s\\n' " + snippet.Quote(recordJSON(rec)) + " > app.json && ")
		b.WriteString("cat > compose.yaml <<'" + heredocTag + "' && docker compose -p " + instance + " up -d\n")
		b.WriteString(compose)
		b.WriteString(heredocTag)
		in.Command = b.String()
	case TypeNpm:
		if instance != a.Name {
			return nil, errors.New("命令行工具的实例名就是应用名")
		}
		v := version
		if v == "" {
			v = a.Npm.Version
		}
		if !VersionRe.MatchString(v) {
			return nil, errors.New("版本号的格式不对（例如 1.2.3 或 latest）")
		}
		in.Version = v
		rec.Version, rec.Bins = v, a.Npm.Bins
		in.Command = npmInstall(a, instance, v, rec)
	}
	return in, nil
}

// PlanUpdate 把一个已安装的实例更新到目录里的版本（命令行工具可以指定版本）。
// 容器应用沿用主机上记下的参数重新生成 compose 文件，拉取新镜像后重建容器；.env 里的密钥不动。
func (a *App) PlanUpdate(rec Record, version string) (*Install, error) {
	if rec.App != a.Name || rec.Type != a.Type {
		return nil, errors.New("实例和应用对不上")
	}
	if a.Type == TypeNpm {
		return a.PlanInstall(rec.Instance, nil, nil, version)
	}
	if a.UpgradeBlocked(rec.Version) {
		return nil, a.blockedErr(rec.Version)
	}
	values, err := a.Values(rec.Params)
	if err != nil {
		return nil, fmt.Errorf("主机上记下的参数在新版本里不合法（%w），请重新安装", err)
	}
	return a.planCompose(rec, values, nil, true)
}

// PlanConfigure 修改一个容器应用实例的设置：按新的参数重新生成 compose 文件，再 docker compose up -d
// （只有设置变了的容器会被重建，数据目录不动）。secrets 为空表示密钥不变；要换就得把每个密钥参数都选上，
// 因为 .env 是整份重写的，而主机上只记了变量名、没记用的是哪个应用密钥。
// 目录里的版本比实例新时，会顺带更新到目录里的版本（先拉取新镜像）。
func (a *App) PlanConfigure(rec Record, params, secrets map[string]string) (*Install, error) {
	if rec.App != a.Name || rec.Type != a.Type {
		return nil, errors.New("实例和应用对不上")
	}
	if a.Type != TypeCompose {
		return nil, errors.New("这个应用没有可以修改的设置")
	}
	// 改设置会按目录里的版本重新生成 compose 文件；不能原地升级的旧实例就不能这样改。
	if a.UpgradeBlocked(rec.Version) {
		return nil, a.blockedErr(rec.Version)
	}
	merged := map[string]string{}
	for k, v := range rec.Params {
		merged[k] = v
	}
	for k, v := range params {
		merged[k] = v
	}
	values, err := a.Values(merged)
	if err != nil {
		return nil, err
	}
	return a.planCompose(rec, values, secrets, a.Version != rec.Version)
}

// planCompose 重新生成一个容器应用实例的 compose 文件并让它生效。
func (a *App) planCompose(rec Record, values, secrets map[string]string, pull bool) (*Install, error) {
	if !InstanceRe.MatchString(rec.Instance) {
		return nil, errors.New("实例名不合法")
	}
	in := &Install{Instance: rec.Instance, Version: a.Version, Secrets: map[string]string{}}
	nrec := Record{App: a.Name, Type: a.Type, Instance: rec.Instance, Version: a.Version, Params: values}
	for _, p := range a.Params {
		if p.Type != "secret" {
			continue
		}
		if len(secrets) > 0 {
			s := secrets[p.Name]
			if !SecretNameRe.MatchString(s) {
				return nil, fmt.Errorf("%s：请选择一个应用密钥", p.Title)
			}
			in.Secrets[p.Env] = s
		} else if !contains(rec.Envs, p.Env) {
			return nil, fmt.Errorf("新版本需要新的密钥（%s），请重新安装", p.Title)
		}
		nrec.Envs = append(nrec.Envs, p.Env)
	}
	compose, err := a.render(values, rec.Instance)
	if err != nil {
		return nil, err
	}
	in.Compose = compose
	dir, p := instanceDir(rec.Instance), " -p "+rec.Instance
	var b strings.Builder
	b.WriteString("docker compose version >/dev/null 2>&1 || { echo 'hostower: 这台主机没有 Docker Compose v2（docker compose 命令）'; exit 3; }; ")
	b.WriteString("umask 077 && cd " + dir + " && ")
	if len(in.Secrets) > 0 {
		b.WriteString("cat > .env && ")
	}
	b.WriteString("printf '%s\\n' " + snippet.Quote(recordJSON(nrec)) + " > app.json && ")
	b.WriteString("cat > compose.yaml <<'" + heredocTag + "' && ")
	if pull {
		b.WriteString("docker compose" + p + " pull && ")
	}
	b.WriteString("docker compose" + p + " up -d\n")
	b.WriteString(compose)
	b.WriteString(heredocTag)
	in.Command = b.String()
	return in, nil
}

func recordJSON(r Record) string {
	b, _ := json.Marshal(r)
	return string(b)
}

func npmInstall(a *App, inst, version string, rec Record) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s) }
	w(`umask 022; d="$HOME/.hostower"; nv=` + NodeVersion + `; nb=; `)
	// 系统里的 Node.js 够新就用它；不够就用（必要时下载）固定版本的官方发布包。
	w(`v="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null)"; `)
	w(`if [ -z "$v" ] || [ "$v" -lt ` + strconv.Itoa(a.Npm.Node) + ` ] 2>/dev/null; then `)
	w(`case "$(uname -m)" in `)
	for _, nb := range nodeBuilds {
		w(nb.uname + `) a=` + nb.arch + `; h=` + nb.sha256 + `;; `)
	}
	w(`*) echo 'hostower: 不支持这种 CPU 架构'; exit 3;; esac; `)
	w(`nb="$d/node-v$nv-linux-$a/bin"; `)
	w(`if [ ! -x "$nb/node" ]; then `)
	w(`command -v curl >/dev/null 2>&1 || { echo 'hostower: 需要 curl 来下载 Node.js'; exit 3; }; `)
	w(`command -v sha256sum >/dev/null 2>&1 || { echo 'hostower: 需要 sha256sum 来校验 Node.js'; exit 3; }; `)
	w(`echo "hostower: 系统里没有 Node.js ` + strconv.Itoa(a.Npm.Node) + ` 以上的版本，下载 Node.js $nv"; `)
	w(`mkdir -p "$d/cache" && f="$d/cache/node-v$nv-linux-$a.tar.gz" && `)
	w(`curl -fsSL -m 600 -o "$f" "https://nodejs.org/dist/v$nv/node-v$nv-linux-$a.tar.gz" || { echo 'hostower: 下载 Node.js 失败'; exit 3; }; `)
	w(`[ "$(sha256sum "$f" | cut -d ' ' -f 1)" = "$h" ] || { echo 'hostower: Node.js 安装包校验失败'; exit 3; }; `)
	w(`tar -xzf "$f" -C "$d" || exit 3; : > "$f"; fi; fi; `)
	w(`P="$PATH"; [ -z "$nb" ] || P="$nb:$PATH"; `)
	// 标准输入可能带着 API Key（安装时顺带配置）：npm 和它的安装脚本一律不读标准输入。
	w(`PATH="$P" npm install -g --prefix "$d/npm" --no-fund --no-audit ` + snippet.Quote(a.Npm.Package+"@"+version) + ` </dev/null || exit 1; `)
	w(`mkdir -p "$d/apps/` + inst + `" || exit 1; printf '%s\n' "$nb" > "$d/apps/` + inst + `/nodebin"; `)
	w(wrapperScript(inst, a.Npm.Bins))
	// 记下实际装上的版本（装的是 latest 时尤其要紧：检测更新靠它）。
	rec.Version = "@@VERSION@@"
	format := strings.ReplaceAll(strings.ReplaceAll(recordJSON(rec), "%", "%%"), "@@VERSION@@", "%s") + `\n`
	w(`iv="$(PATH="$P" node -p 'require(process.argv[1]).version' "$d/npm/lib/node_modules/` + a.Npm.Package + `/package.json" 2>/dev/null </dev/null)"; `)
	w(`[ -n "$iv" ] || iv=` + snippet.Quote(version) + `; `)
	w(`printf ` + snippet.Quote(format) + ` "$iv" > "$d/apps/` + inst + `/app.json"; `)
	w(`echo "hostower: 已安装 ` + a.Npm.Package + ` $iv"; `)
	w(`case ":$PATH:" in *":$HOME/.local/bin:"*) ;; *) echo 'hostower: ~/.local/bin 不在 PATH 里：重新登录，或执行 export PATH="$HOME/.local/bin:$PATH"';; esac`)
	return b.String()
}

// Action 是对已安装实例的操作。
const (
	ActStart     = "start"
	ActStop      = "stop"
	ActRestart   = "restart"
	ActUninstall = "uninstall" // 停掉并移除，数据留在主机上
	ActPurge     = "purge"     // 连数据一起删（L2）
)

// PlanAction 渲染对一个实例的操作。typ 和 bins 来自主机上的 app.json，pkg 是清单里的 npm 包名。
func PlanAction(rec Record, pkg, action string) (string, error) {
	if !InstanceRe.MatchString(rec.Instance) {
		return "", errors.New("实例名不合法")
	}
	dir := instanceDir(rec.Instance)
	p := " -p " + rec.Instance
	switch rec.Type {
	case TypeCompose:
		switch action {
		case ActStart, ActStop, ActRestart:
			return "cd " + dir + " && docker compose" + p + " " + action, nil
		case ActUninstall:
			return "cd " + dir + " && docker compose" + p + " down && mv -f app.json removed.json", nil
		case ActPurge:
			// 容器写出来的数据常常属于 root：当前账号删不掉时先试 sudo；sudo 也用不了时
			// （没配 sudo，或者塔台的服务设置了 NoNewPrivileges），借一个临时容器把目录里的东西删掉。
			// 能跑 docker compose 就能跑 docker，所以最后这一步总能成功。整条命令仍是 L2。
			abs := `"$HOME/.hostower/apps/` + rec.Instance + `:/d"`
			return "cd " + dir + " && docker compose" + p + " down -v && cd && { rm -rf " + dir + " 2>/dev/null || sudo -n rm -rf " + dir + " 2>/dev/null || " +
				"{ docker run --rm -v " + abs + " " + purgeHelperImage + " sh -c 'rm -rf /d/* /d/.[!.]* /d/..?*' && rm -rf " + dir + "; }; }", nil
		}
	case TypeNpm:
		if action != ActUninstall || !npmPkgRe.MatchString(pkg) {
			break
		}
		for _, b := range rec.Bins {
			if !binRe.MatchString(b) {
				return "", errors.New("命令名不合法")
			}
		}
		var b strings.Builder
		b.WriteString(`d="$HOME/.hostower"; `)
		if len(rec.Bins) > 0 {
			b.WriteString(`for b in ` + strings.Join(rec.Bins, " ") + `; do f="$HOME/.local/bin/$b"; if grep -q 'hostower-app ` + rec.Instance + `' "$f" 2>/dev/null; then mv -f "$f" "$d/apps/` + rec.Instance + `/$b.removed"; fi; done; `)
		}
		b.WriteString(`n="$(ls -d "$d"/node-v*/bin 2>/dev/null | head -n 1)"; P="$PATH"; [ -z "$n" ] || P="$n:$PATH"; `)
		b.WriteString(`PATH="$P" npm uninstall -g --prefix "$d/npm" ` + snippet.Quote(pkg) + ` || echo 'hostower: npm uninstall 失败，程序文件还留在 ~/.hostower/npm'; `)
		b.WriteString(`mv -f "$d/apps/` + rec.Instance + `/app.json" "$d/apps/` + rec.Instance + `/removed.json"`)
		return b.String(), nil
	}
	return "", errors.New("这个应用不支持这个操作")
}

// ListCommand 是“已安装”列表的只读查询：读每个实例的 app.json，再列出带应用标签的容器。
// 第三段是本机镜像的摘要（仓库:标签 → 摘要），用来和镜像仓库比对，看同一个标签有没有新构建。
const ListCommand = `cat ~/.hostower/apps/*/app.json 2>/dev/null; cat ~/.hostower/apps/*/config.json 2>/dev/null; echo '---'; docker ps -a --filter label=` + LabelKey +
	` --format '{{.Label "` + LabelKey + `"}}	{{.Names}}	{{.State}}	{{.Status}}	{{.Ports}}' 2>/dev/null; echo '---'; docker images --digests --format '{{.Repository}}:{{.Tag}}	{{.Digest}}' 2>/dev/null`

// Container 是实例里的一个容器。
type Container struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}

// Installed 是主机上的一个实例。
type Installed struct {
	Record
	Containers []Container `json:"containers"`
}

// ParseList 解析 ListCommand 的输出。
func ParseList(out string) []Installed {
	list, _ := ParseListDigests(out)
	return list
}

// ParseListDigests 解析 ListCommand 的输出，另外返回本机镜像的摘要（“仓库:标签” → sha256:…）。
func ParseListDigests(out string) ([]Installed, map[string]string) {
	var secs [3][]string
	n := 0
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if line == "---" {
			if n < 2 {
				n++
			}
			continue
		}
		secs[n] = append(secs[n], line)
	}
	head, tail := strings.Join(secs[0], "\n"), strings.Join(secs[1], "\n")
	digests := map[string]string{}
	for _, line := range secs[2] {
		img, d, ok := strings.Cut(line, "\t")
		if ok && strings.HasPrefix(d, "sha256:") && !strings.Contains(img, "<none>") {
			digests[img] = d
		}
	}
	var list []Installed
	idx := map[string]int{}
	for _, line := range strings.Split(head, "\n") {
		var r Record
		if json.Unmarshal([]byte(strings.TrimSpace(line)), &r) != nil || !InstanceRe.MatchString(r.Instance) {
			continue
		}
		if r.App == "" {
			// config.json 的一行：只有实例名和配置，并进 app.json 那一项（app.json 都排在前面）。
			if i := idx[r.Instance]; i > 0 && r.Config != nil {
				list[i-1].Config = r.Config
			}
			continue
		}
		if idx[r.Instance] > 0 {
			continue
		}
		r.Config = nil
		list = append(list, Installed{Record: r, Containers: []Container{}})
		idx[r.Instance] = len(list)
	}
	for _, line := range strings.Split(tail, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 4 {
			continue
		}
		i := idx[f[0]]
		if i == 0 {
			continue
		}
		c := Container{Name: f[1], State: f[2], Status: f[3]}
		if len(f) > 4 {
			c.Ports = f[4]
		}
		list[i-1].Containers = append(list[i-1].Containers, c)
	}
	return list, digests
}
