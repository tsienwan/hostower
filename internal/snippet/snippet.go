// Package snippet 实现参数化的操作模板。参数有类型和校验规则，
// 渲染时所有值默认做 shell 转义；渲染结果仍然要过内核判级。
package snippet

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// Param 是一个参数的定义。页面表单和 AI 工具的参数定义都由它生成。
type Param struct {
	Name        string   `yaml:"name" json:"name"`
	Type        string   `yaml:"type" json:"type"` // string | int | bool | enum | path
	Title       string   `yaml:"title,omitempty" json:"title,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Required    bool     `yaml:"required,omitempty" json:"required"`
	Default     any      `yaml:"default,omitempty" json:"default,omitempty"`
	Values      []string `yaml:"values,omitempty" json:"values,omitempty"`
	Pattern     string   `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Min         *int     `yaml:"min,omitempty" json:"min,omitempty"`
	Max         *int     `yaml:"max,omitempty" json:"max,omitempty"`

	re *regexp.Regexp
}

// Strategy 是 Snippet 建议的批量执行方式。
type Strategy struct {
	Concurrency int  `yaml:"concurrency" json:"concurrency"`
	Canary      bool `yaml:"canary" json:"canary"`
	StopOnError bool `yaml:"stopOnError" json:"stopOnError"`
}

// Snippet 是一个参数化的操作模板。
type Snippet struct {
	APIVersion  string     `yaml:"apiVersion" json:"-"`
	Kind        string     `yaml:"kind" json:"-"`
	Name        string     `yaml:"name" json:"name"`
	Title       string     `yaml:"title" json:"title"`
	Description string     `yaml:"description,omitempty" json:"description,omitempty"`
	Version     string     `yaml:"version" json:"version"`
	Level       risk.Level `yaml:"level" json:"level"`
	Selector    string     `yaml:"selector,omitempty" json:"selector,omitempty"` // 建议的目标
	Params      []Param    `yaml:"params,omitempty" json:"params"`
	Command     string     `yaml:"command" json:"command"`
	Strategy    Strategy   `yaml:"strategy" json:"strategy"`
	// Source 标记来自配置文件（file）还是在页面上管理（managed）；YAML 是可以放进 Git 的文件内容。
	Source string `yaml:"-" json:"source"`
	YAML   string `yaml:"-" json:"yaml,omitempty"`

	tmpl *template.Template
}

// FromSpec 把页面上填的内容变成和文件相同格式的 YAML，并按同一套规则校验。
func FromSpec(spec Snippet) (*Snippet, []byte, error) {
	spec.APIVersion, spec.Kind = inventory.APIVersion, "Snippet"
	spec.Source, spec.YAML = "", ""
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Selector = strings.TrimSpace(spec.Selector)
	if spec.Selector != "" {
		if _, err := inventory.ParseSelector(spec.Selector); err != nil {
			return nil, nil, fmt.Errorf("默认目标：%w", err)
		}
	}
	if spec.Strategy.Concurrency < 0 || spec.Strategy.Concurrency > 50 {
		return nil, nil, fmt.Errorf("并发应在 0 到 50 之间")
	}
	if len(spec.Command) > 16<<10 || len(spec.Params) > 32 {
		return nil, nil, fmt.Errorf("命令模板过长或参数过多")
	}
	data, err := yaml.Marshal(&spec)
	if err != nil {
		return nil, nil, err
	}
	out, err := Parse(data)
	if err != nil {
		return nil, nil, err
	}
	// 用默认值试渲染一次：模板里引用了不存在的参数，现在就报出来，而不是等到执行时。
	probe := map[string]string{}
	for _, p := range out.Params {
		probe[p.Name] = "x"
	}
	if err := out.checkRefs(probe); err != nil {
		return nil, nil, err
	}
	return out, data, nil
}

// checkRefs 检查模板引用的参数是否都定义过（主机变量因主机而异，不在这里检查）。
func (s *Snippet) checkRefs(params map[string]string) error {
	vars := map[string]string{}
	for _, m := range hostVarRe.FindAllStringSubmatch(s.Command, -1) {
		vars[m[1]] = "x"
	}
	data := map[string]any{"host": map[string]any{"name": "h", "addr": "a", "vars": vars}}
	for k, v := range params {
		data[k] = v
	}
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, data); err != nil {
		msg := err.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		if m := regexp.MustCompile(`key "([^"]+)"`).FindStringSubmatch(msg); m != nil {
			msg = m[1]
		}
		return fmt.Errorf("命令模板引用了没有定义的参数：%s", msg)
	}
	return nil
}

var hostVarRe = regexp.MustCompile(`\.host\.vars\.([A-Za-z_][A-Za-z0-9_]*)`)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var paramRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// Parse 解析并校验一个 Snippet 文件。
func Parse(data []byte) (*Snippet, error) {
	var s Snippet
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.APIVersion != inventory.APIVersion || s.Kind != "Snippet" {
		return nil, fmt.Errorf("需要 apiVersion: %s 与 kind: Snippet", inventory.APIVersion)
	}
	if !nameRe.MatchString(s.Name) {
		return nil, fmt.Errorf("name %q 不合法（小写字母、数字、连字符）", s.Name)
	}
	if s.Title == "" {
		s.Title = s.Name
	}
	if s.Version == "" {
		s.Version = "0"
	}
	if strings.TrimSpace(s.Command) == "" {
		return nil, fmt.Errorf("%s：缺少 command", s.Name)
	}
	seen := map[string]bool{"host": true}
	for i := range s.Params {
		p := &s.Params[i]
		if !paramRe.MatchString(p.Name) || seen[p.Name] {
			return nil, fmt.Errorf("%s：参数名 %q 不合法或重复", s.Name, p.Name)
		}
		seen[p.Name] = true
		switch p.Type {
		case "string", "int", "bool", "path":
		case "enum":
			if len(p.Values) == 0 {
				return nil, fmt.Errorf("%s：enum 参数 %s 需要 values", s.Name, p.Name)
			}
		default:
			return nil, fmt.Errorf("%s：参数 %s 的类型 %q 暂不支持", s.Name, p.Name, p.Type)
		}
		if p.Pattern != "" {
			re, err := regexp.Compile(`^(?:` + p.Pattern + `)$`)
			if err != nil {
				return nil, fmt.Errorf("%s：参数 %s 的 pattern 无效：%w", s.Name, p.Name, err)
			}
			p.re = re
		}
	}
	tmpl, err := template.New(s.Name).Option("missingkey=error").Parse(s.Command)
	if err != nil {
		return nil, fmt.Errorf("%s：command 模板无效：%w", s.Name, err)
	}
	s.tmpl = tmpl
	return &s, nil
}

// LoadDir 加载目录下的全部 Snippet；目录不存在时返回空。
func LoadDir(dir string) (map[string]*Snippet, error) {
	out := map[string]*Snippet{}
	files, _ := filepath.Glob(filepath.Join(dir, "*.yaml"))
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		s, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", filepath.Base(f), err)
		}
		if _, dup := out[s.Name]; dup {
			return nil, fmt.Errorf("Snippet %s 重复定义", s.Name)
		}
		out[s.Name] = s
	}
	return out, nil
}

// Validate 校验并规整输入的参数，返回每个参数的字符串值。
func (s *Snippet) Validate(in map[string]any) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range s.Params {
		v, ok := in[p.Name]
		if !ok || v == nil || v == "" {
			v = p.Default
		}
		if v == nil || v == "" {
			if p.Required {
				return nil, fmt.Errorf("缺少参数 %s", p.label())
			}
			if p.Type == "bool" {
				out[p.Name] = "false"
			} else {
				out[p.Name] = ""
			}
			continue
		}
		str := strings.TrimSpace(fmt.Sprint(v))
		if f, isFloat := v.(float64); isFloat && f == float64(int64(f)) {
			str = strconv.FormatInt(int64(f), 10)
		}
		if len(str) > 4096 || strings.ContainsAny(str, "\x00\n\r") {
			return nil, fmt.Errorf("参数 %s 过长或含换行", p.label())
		}
		switch p.Type {
		case "int":
			n, err := strconv.Atoi(str)
			if err != nil {
				return nil, fmt.Errorf("参数 %s 必须是整数", p.label())
			}
			if p.Min != nil && n < *p.Min || p.Max != nil && n > *p.Max {
				return nil, fmt.Errorf("参数 %s 超出范围", p.label())
			}
			str = strconv.Itoa(n)
		case "bool":
			if str != "true" && str != "false" {
				return nil, fmt.Errorf("参数 %s 必须是 true 或 false", p.label())
			}
		case "enum":
			found := false
			for _, val := range p.Values {
				found = found || val == str
			}
			if !found {
				return nil, fmt.Errorf("参数 %s 只能是：%s", p.label(), strings.Join(p.Values, "、"))
			}
		case "path":
			if !strings.HasPrefix(str, "/") || strings.Contains(str, "..") {
				return nil, fmt.Errorf("参数 %s 必须是不含 .. 的绝对路径", p.label())
			}
		}
		if p.re != nil && !p.re.MatchString(str) {
			return nil, fmt.Errorf("参数 %s 不符合格式 %s", p.label(), p.Pattern)
		}
		out[p.Name] = str
	}
	for name := range in {
		if _, ok := out[name]; !ok {
			return nil, fmt.Errorf("未知参数 %s", name)
		}
	}
	return out, nil
}

func (p Param) label() string {
	if p.Title != "" {
		return p.Title
	}
	return p.Name
}

// Render 按“参数 + 主机变量”渲染出这台主机上的实际命令。所有值都做 shell 转义。
func (s *Snippet) Render(params map[string]string, h inventory.Host) (string, error) {
	data := map[string]any{}
	for k, v := range params {
		// 留空的可选参数渲染成“什么都没有”，而不是一个空字符串参数。
		if v == "" {
			data[k] = ""
			continue
		}
		data[k] = Quote(v)
	}
	vars := map[string]string{}
	for k, v := range h.Vars {
		vars[k] = Quote(v)
	}
	data["host"] = map[string]any{"name": Quote(h.Name), "addr": Quote(h.Addr), "vars": vars}
	var buf bytes.Buffer
	if err := s.tmpl.Execute(&buf, data); err != nil {
		msg := err.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		return "", fmt.Errorf("主机 %s 渲染失败：%s", h.Name, msg)
	}
	return strings.TrimSpace(buf.String()), nil
}

var safeRe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// Quote 把一个值转义成单个 shell 单词。
func Quote(s string) string {
	if safeRe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
