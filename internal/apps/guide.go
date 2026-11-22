package apps

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// 上手指南：应用装好以后，页面上直接告诉人“从哪里打开、初始账号是什么、接下来做哪几步”，
// 不用再登录主机翻日志。指南写在清单的 guide 里，和清单一起随版本发布。
//
// 有些应用的初始账号是第一次启动时随机生成、只在日志里出现一次的（比如 3x-ui）：
// 清单里可以给一条只读命令（reveal），页面上点一下就由内核的只读查询去读。
// 这条命令和别的查询一样要过判级，不是 L0 就拒绝（测试里也锁住每个内置应用的 reveal 都是 L0）。

// Guide 是清单里的上手指南。文字里可以引用安装参数（{{ .port }}）和实例名（{{ .instance }}）。
type Guide struct {
	Open    []GuideLink  `yaml:"open"`
	Account string       `yaml:"account"`
	Steps   []string     `yaml:"steps"`
	Reveal  *GuideReveal `yaml:"reveal"`
}

// GuideLink 是一个访问入口。
type GuideLink struct {
	Title  string `yaml:"title"`
	Scheme string `yaml:"scheme"` // http（默认）| https
	Port   string `yaml:"port"`   // 可以引用参数
	Path   string `yaml:"path"`
	// Bind 是监听地址（一般写 {{ .bind }}）；渲染出来是 127.0.0.1 时，页面提示只能在主机本机访问。
	Bind string `yaml:"bind"`
}

// GuideReveal 是一条从主机上读初始账号的只读命令。
type GuideReveal struct {
	Title   string `yaml:"title"`
	Command string `yaml:"command"`
	Hint    string `yaml:"hint"`
}

// GuideView 是按一个实例的参数渲染好的指南，给页面用。
type GuideView struct {
	Open    []OpenView  `json:"open"`
	Account string      `json:"account,omitempty"`
	Steps   []string    `json:"steps"`
	Reveal  *RevealView `json:"reveal,omitempty"`
}

// OpenView 是一个渲染好的访问入口。主机地址由页面按清单里的主机补上。
type OpenView struct {
	Title  string `json:"title"`
	Scheme string `json:"scheme"`
	Port   string `json:"port"`
	Path   string `json:"path"`
	// Local 表示只在主机本机监听：要配反向代理或 SSH 隧道才能从外面打开。
	Local bool `json:"local,omitempty"`
}

// RevealView 是“读取初始账号”按钮上的文字；命令本身不给页面。
type RevealView struct {
	Title string `json:"title"`
	Hint  string `json:"hint,omitempty"`
}

var guidePathRe = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,127}$`)

// guideValues 是渲染指南用的值：实例名加上重新校验过的参数（主机上记的参数不直接信）。
func (a *App) guideValues(rec Record) (map[string]string, error) {
	if !InstanceRe.MatchString(rec.Instance) {
		return nil, errors.New("实例名不合法")
	}
	vals, err := a.Values(rec.Params)
	if err != nil {
		return nil, err
	}
	vals["instance"] = rec.Instance
	return vals, nil
}

func guideText(name, text string, vals map[string]string) (string, error) {
	if !strings.Contains(text, "{{") {
		return text, nil
	}
	tpl, err := template.New(name).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", fmt.Errorf("上手指南的模板：%w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vals); err != nil {
		return "", fmt.Errorf("上手指南的模板：%w", err)
	}
	return buf.String(), nil
}

// GuideFor 按一个实例的参数渲染上手指南；清单里没有指南时返回 nil。
func (a *App) GuideFor(rec Record) (*GuideView, error) {
	g := a.Guide
	if g == nil {
		return nil, nil
	}
	vals, err := a.guideValues(rec)
	if err != nil {
		return nil, err
	}
	v := &GuideView{Open: []OpenView{}, Steps: []string{}}
	for _, l := range g.Open {
		port, err := guideText(a.Name, l.Port, vals)
		if err != nil {
			return nil, err
		}
		bind, err := guideText(a.Name, l.Bind, vals)
		if err != nil {
			return nil, err
		}
		if !validPort(port) {
			return nil, errors.New("上手指南里的端口不合法")
		}
		scheme := l.Scheme
		if scheme == "" {
			scheme = "http"
		}
		path := l.Path
		if path == "" {
			path = "/"
		}
		v.Open = append(v.Open, OpenView{Title: l.Title, Scheme: scheme, Port: port, Path: path, Local: bind == "127.0.0.1" || bind == "localhost" || bind == "::1"})
	}
	if v.Account, err = guideText(a.Name, g.Account, vals); err != nil {
		return nil, err
	}
	for _, s := range g.Steps {
		t, err := guideText(a.Name, s, vals)
		if err != nil {
			return nil, err
		}
		v.Steps = append(v.Steps, t)
	}
	if g.Reveal != nil {
		v.Reveal = &RevealView{Title: g.Reveal.Title, Hint: g.Reveal.Hint}
	}
	return v, nil
}

// RevealCommand 渲染“读取初始账号”的只读命令。
func (a *App) RevealCommand(rec Record) (string, error) {
	if a.Guide == nil || a.Guide.Reveal == nil {
		return "", errors.New("这个应用没有可以读取的初始信息")
	}
	vals, err := a.guideValues(rec)
	if err != nil {
		return "", err
	}
	return guideText(a.Name, a.Guide.Reveal.Command, vals)
}

// checkGuide 在加载清单时校验指南：按默认参数能渲染出来，入口的端口、路径、协议合法。
func (a *App) checkGuide() error {
	g := a.Guide
	if g == nil {
		return nil
	}
	if len(g.Open) == 0 && g.Account == "" && len(g.Steps) == 0 && g.Reveal == nil {
		return errors.New("上手指南是空的")
	}
	if a.Type != TypeCompose && len(g.Open) > 0 {
		return errors.New("只有容器应用的上手指南能写访问入口")
	}
	for _, l := range g.Open {
		if l.Title == "" || l.Port == "" {
			return errors.New("上手指南的访问入口需要 title 和 port")
		}
		if l.Scheme != "" && l.Scheme != "http" && l.Scheme != "https" {
			return fmt.Errorf("上手指南的协议 %q 不认识", l.Scheme)
		}
		if l.Path != "" && !guidePathRe.MatchString(l.Path) {
			return fmt.Errorf("上手指南的路径 %q 不合法", l.Path)
		}
	}
	if r := g.Reveal; r != nil && (r.Title == "" || strings.TrimSpace(r.Command) == "" || strings.ContainsAny(r.Command, "\n\r")) {
		return errors.New("上手指南的 reveal 需要 title 和单行的 command")
	}
	vals := a.defaults()
	for _, p := range a.Params {
		if p.Type == "secret" {
			delete(vals, p.Name)
		}
	}
	rec := Record{Instance: a.Name, Params: vals}
	if _, err := a.GuideFor(rec); err != nil {
		return err
	}
	if g.Reveal != nil {
		if _, err := a.RevealCommand(rec); err != nil {
			return err
		}
	}
	return nil
}
