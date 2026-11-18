package inventory

import (
	"fmt"
	"path"
	"strings"
)

// Selector 是解析后的选择器表达式。
//
//	表达式 := 项 { "or" 项 }
//	项     := 因子 { "and" 因子 }
//	因子   := "not" 因子 | "(" 表达式 ")" | 原子
//	原子   := "*" | tag:<标签> | sensitivity:<级别> | executor:<名字> | group:<分组> | name:<通配> | <主机名或通配>
type Selector struct {
	op   string // and | or | not | atom
	l, r *Selector
	key  string
	val  string
}

// ParseSelector 解析选择器。
func ParseSelector(s string) (*Selector, error) {
	p := &selParser{toks: tokenize(s)}
	if len(p.toks) == 0 {
		return nil, fmt.Errorf("选择器为空")
	}
	sel, err := p.expr()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.toks) {
		return nil, fmt.Errorf("选择器在 %q 附近有多余内容", p.toks[p.pos])
	}
	return sel, nil
}

func tokenize(s string) []string {
	s = strings.NewReplacer("(", " ( ", ")", " ) ", ",", " or ").Replace(s)
	return strings.Fields(s)
}

type selParser struct {
	toks []string
	pos  int
}

func (p *selParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos]
	}
	return ""
}

func (p *selParser) expr() (*Selector, error) {
	l, err := p.term()
	if err != nil {
		return nil, err
	}
	for p.peek() == "or" {
		p.pos++
		r, err := p.term()
		if err != nil {
			return nil, err
		}
		l = &Selector{op: "or", l: l, r: r}
	}
	return l, nil
}

func (p *selParser) term() (*Selector, error) {
	l, err := p.factor()
	if err != nil {
		return nil, err
	}
	for p.peek() == "and" {
		p.pos++
		r, err := p.factor()
		if err != nil {
			return nil, err
		}
		l = &Selector{op: "and", l: l, r: r}
	}
	return l, nil
}

func (p *selParser) factor() (*Selector, error) {
	t := p.peek()
	switch t {
	case "":
		return nil, fmt.Errorf("选择器不完整")
	case "and", "or", ")":
		return nil, fmt.Errorf("选择器在 %q 处缺少条件", t)
	case "not":
		p.pos++
		inner, err := p.factor()
		if err != nil {
			return nil, err
		}
		return &Selector{op: "not", l: inner}, nil
	case "(":
		p.pos++
		inner, err := p.expr()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("选择器缺少右括号")
		}
		p.pos++
		return inner, nil
	}
	p.pos++
	key, val, ok := strings.Cut(t, ":")
	if !ok {
		return &Selector{op: "atom", key: "name", val: t}, nil
	}
	switch key {
	case "tag", "sensitivity", "executor", "group", "name":
		if val == "" {
			return nil, fmt.Errorf("选择器 %q 缺少值", t)
		}
		return &Selector{op: "atom", key: key, val: val}, nil
	}
	return nil, fmt.Errorf("选择器不认识 %q（可用：tag: sensitivity: executor: group: name:）", key)
}

func (s *Selector) match(h Host, inv *Inventory, depth int) bool {
	switch s.op {
	case "and":
		return s.l.match(h, inv, depth) && s.r.match(h, inv, depth)
	case "or":
		return s.l.match(h, inv, depth) || s.r.match(h, inv, depth)
	case "not":
		return !s.l.match(h, inv, depth)
	}
	switch s.key {
	case "tag":
		return h.HasTag(s.val)
	case "sensitivity":
		return string(h.Sensitivity) == s.val
	case "executor":
		return h.Executor == s.val
	case "group":
		g, ok := inv.groups[s.val]
		if !ok || depth > 8 {
			return false
		}
		if g.Selector != "" {
			sub, err := ParseSelector(g.Selector)
			return err == nil && sub.match(h, inv, depth+1)
		}
		for _, n := range g.Hosts {
			if n == h.Name {
				return true
			}
		}
		return false
	default:
		ok, _ := path.Match(s.val, h.Name)
		return ok
	}
}
