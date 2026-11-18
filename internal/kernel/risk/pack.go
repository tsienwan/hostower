package risk

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// PackRule 是规则包里的一条规则。规则包只能加严：它只在等级高于内置判定时生效。
type PackRule struct {
	ID      string
	Command string
	Args    *regexp.Regexp
	Level   Level
	Reason  string
}

func (p PackRule) matches(name, joinedArgs string) bool {
	if p.Command != name {
		return false
	}
	return p.Args == nil || p.Args.MatchString(joinedArgs)
}

type packFile struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Name       string `yaml:"name"`
	Rules      []struct {
		ID      string `yaml:"id"`
		Command string `yaml:"command"`
		Args    string `yaml:"args"`
		Level   string `yaml:"level"`
		Reason  string `yaml:"reason"`
	} `yaml:"rules"`
}

// ParsePack 解析一个 RulePack 文件。
func ParsePack(data []byte) ([]PackRule, error) {
	var f packFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.APIVersion != "hostower/v1" || f.Kind != "RulePack" {
		return nil, fmt.Errorf("规则包需要 apiVersion: hostower/v1 与 kind: RulePack")
	}
	var out []PackRule
	for _, r := range f.Rules {
		lvl, err := ParseLevel(r.Level)
		if err != nil {
			return nil, err
		}
		if lvl == L0 {
			return nil, fmt.Errorf("规则 %s：规则包只能加严，不能声明 L0", r.ID)
		}
		if r.Command == "" || r.ID == "" {
			return nil, fmt.Errorf("规则包 %s：规则需要 id 和 command", f.Name)
		}
		pr := PackRule{ID: f.Name + "/" + r.ID, Command: r.Command, Level: lvl, Reason: r.Reason}
		if r.Args != "" {
			re, err := regexp.Compile(r.Args)
			if err != nil {
				return nil, fmt.Errorf("规则 %s：%w", r.ID, err)
			}
			pr.Args = re
		}
		out = append(out, pr)
	}
	return out, nil
}
