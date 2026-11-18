// Package inventory 加载声明式的主机清单，并用选择器把分组解析成主机列表。
package inventory

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// APIVersion 是清单文件当前的版本。
const APIVersion = "hostower/v1"

// Host 是一台被管主机。
type Host struct {
	Name        string            `yaml:"-" json:"name"`
	Addr        string            `yaml:"addr" json:"addr"`
	Executor    string            `yaml:"executor" json:"executor"`
	Credential  string            `yaml:"credential" json:"credential,omitempty"`
	Tags        []string          `yaml:"tags" json:"tags"`
	Sensitivity risk.Sensitivity  `yaml:"sensitivity" json:"sensitivity"`
	Vars        map[string]string `yaml:"vars" json:"vars,omitempty"`
	Manual      string            `yaml:"manual" json:"manual,omitempty"`
	// Source 标记主机来自配置文件（file）还是在页面上管理（managed）。
	Source string `yaml:"-" json:"source"`
}

// HasTag 报告主机是否带某个标签。
func (h Host) HasTag(tag string) bool {
	for _, t := range h.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Group 是一个分组：静态列表或选择器。
type Group struct {
	Name     string   `yaml:"-" json:"name"`
	Selector string   `yaml:"selector" json:"selector,omitempty"`
	Hosts    []string `yaml:"hosts" json:"hosts,omitempty"`
}

type file struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Hosts      map[string]Host  `yaml:"hosts"`
	Groups     map[string]Group `yaml:"groups"`
}

// Inventory 是加载后的不可变清单。
type Inventory struct {
	hosts  map[string]Host
	names  []string
	groups map[string]Group
	gnames []string
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,62}$`)

// Empty 返回一个空清单。
func Empty() *Inventory {
	return &Inventory{hosts: map[string]Host{}, groups: map[string]Group{}}
}

// Load 从文件加载清单；文件不存在时返回空清单。
func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse 解析清单内容。
func Parse(data []byte) (*Inventory, error) {
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("清单：%w", err)
	}
	if f.APIVersion != APIVersion || f.Kind != "Inventory" {
		return nil, fmt.Errorf("清单需要 apiVersion: %s 与 kind: Inventory（可用 hostower migrate config 改写旧文件）", APIVersion)
	}
	inv := Empty()
	for name, h := range f.Hosts {
		if !nameRe.MatchString(name) {
			return nil, fmt.Errorf("主机名 %q 不合法", name)
		}
		h.Name = name
		h.Source = "file"
		if err := Normalize(&h); err != nil {
			return nil, err
		}
		inv.hosts[name] = h
		inv.names = append(inv.names, name)
	}
	sort.Strings(inv.names)
	for name, g := range f.Groups {
		if !nameRe.MatchString(name) {
			return nil, fmt.Errorf("分组名 %q 不合法", name)
		}
		g.Name = name
		if g.Selector != "" {
			if _, err := ParseSelector(g.Selector); err != nil {
				return nil, fmt.Errorf("分组 %s：%w", name, err)
			}
		}
		for _, h := range g.Hosts {
			if _, ok := inv.hosts[h]; !ok {
				return nil, fmt.Errorf("分组 %s：主机 %s 不存在", name, h)
			}
		}
		inv.groups[name] = g
		inv.gnames = append(inv.gnames, name)
	}
	sort.Strings(inv.gnames)
	return inv, nil
}

var tagRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,31}$`)
var addrRe = regexp.MustCompile(`^[a-zA-Z0-9\[\]:._-]{1,255}$`)

// Normalize 校验一台主机并补上默认值。
func Normalize(h *Host) error {
	if !nameRe.MatchString(h.Name) {
		return fmt.Errorf("主机名 %q 不合法（字母、数字、点、下划线、连字符）", h.Name)
	}
	if h.Executor == "" {
		h.Executor = "ssh"
	}
	switch h.Sensitivity {
	case "":
		h.Sensitivity = risk.SensNormal
	case risk.SensLow, risk.SensNormal, risk.SensHigh:
	default:
		return fmt.Errorf("主机 %s：sensitivity 只能是 low、normal、high", h.Name)
	}
	if h.Addr == "" {
		return fmt.Errorf("主机 %s：缺少 addr", h.Name)
	}
	if !addrRe.MatchString(h.Addr) {
		return fmt.Errorf("主机 %s：地址 %q 不合法", h.Name, h.Addr)
	}
	if h.Tags == nil {
		h.Tags = []string{}
	}
	for _, t := range h.Tags {
		if !tagRe.MatchString(t) {
			return fmt.Errorf("主机 %s：标签 %q 不合法", h.Name, t)
		}
	}
	return nil
}

// LocalName 是内置的“本机”主机的名字。
const LocalName = "local"

// Builtin 返回内置的“本机”主机：Hostower 自己所在的这台机器。
// 标签、敏感度、变量可以在页面上改（见 app.SaveHost）；名字、地址、连接方式是固定的，也不能删除。
func Builtin() Host {
	return Host{Name: LocalName, Addr: "localhost", Executor: "local", Tags: []string{"local"},
		Sensitivity: risk.SensNormal, Source: "builtin"}
}

// WithManaged 返回一份合并了“页面管理的主机”的新清单。
// 名字冲突时配置文件优先：文件是声明式的真相来源。
// extra 里已经带来源的条目（内置的本机）保留原来的来源。
func (inv *Inventory) WithManaged(extra []Host) *Inventory {
	out := Empty()
	for n, h := range inv.hosts {
		out.hosts[n] = h
	}
	for n, g := range inv.groups {
		out.groups[n] = g
	}
	out.gnames = append(out.gnames, inv.gnames...)
	for _, h := range extra {
		if _, clash := out.hosts[h.Name]; clash {
			continue
		}
		if h.Source == "" {
			h.Source = "managed"
		}
		out.hosts[h.Name] = h
	}
	for n := range out.hosts {
		out.names = append(out.names, n)
	}
	sort.Strings(out.names)
	return out
}

// Host 按名字取主机。
func (inv *Inventory) Host(name string) (Host, bool) {
	h, ok := inv.hosts[name]
	return h, ok
}

// Hosts 返回全部主机，按名字排序。
func (inv *Inventory) Hosts() []Host {
	out := make([]Host, 0, len(inv.names))
	for _, n := range inv.names {
		out = append(out, inv.hosts[n])
	}
	return out
}

// Groups 返回全部分组。
func (inv *Inventory) Groups() []Group {
	out := make([]Group, 0, len(inv.gnames))
	for _, n := range inv.gnames {
		out = append(out, inv.groups[n])
	}
	return out
}

// Members 返回分组当前的成员。
func (inv *Inventory) Members(g Group) []Host {
	if g.Selector != "" {
		hs, _ := inv.Select(g.Selector)
		return hs
	}
	var out []Host
	for _, n := range g.Hosts {
		out = append(out, inv.hosts[n])
	}
	return out
}

// Select 用选择器挑出主机。`group:<名字>` 引用一个分组。
func (inv *Inventory) Select(selector string) ([]Host, error) {
	sel, err := ParseSelector(selector)
	if err != nil {
		return nil, err
	}
	var out []Host
	for _, n := range inv.names {
		h := inv.hosts[n]
		if sel.match(h, inv, 0) {
			out = append(out, h)
		}
	}
	return out, nil
}
