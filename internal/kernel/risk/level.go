// Package risk 是安全内核的判级引擎：把一段 shell 文本拆成单条命令，
// 逐条匹配规则，取最高等级。这里没有扩展点；行为由 testdata/table.txt 锁定。
package risk

import "fmt"

// Level 是风险等级。数值越大越危险，判级永远取最大值。
type Level int

const (
	L0 Level = iota // 只读：自动执行
	L1              // 变更：一人批准
	L2              // 高危：二次验证身份并输入主机名
	L3              // 禁止：任何角色、任何入口都不能放行
)

func (l Level) String() string {
	if l < L0 || l > L3 {
		return fmt.Sprintf("L?(%d)", int(l))
	}
	return [...]string{"L0", "L1", "L2", "L3"}[l]
}

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	v, err := ParseLevel(string(b))
	if err != nil {
		return err
	}
	*l = v
	return nil
}

// ParseLevel 解析 "L0".."L3"。
func ParseLevel(s string) (Level, error) {
	switch s {
	case "L0":
		return L0, nil
	case "L1":
		return L1, nil
	case "L2":
		return L2, nil
	case "L3":
		return L3, nil
	}
	return L0, fmt.Errorf("risk: 无效等级 %q", s)
}

// Max 返回两者中较高的等级。
func Max(a, b Level) Level {
	if a > b {
		return a
	}
	return b
}

// Finding 是一条判级理由：哪条命令、命中了哪条规则、为什么。
type Finding struct {
	Command string `json:"command"`
	Level   Level  `json:"level"`
	Rule    string `json:"rule"`
	Reason  string `json:"reason"`
}

// Verdict 是对一段 shell 文本的判级结果。
type Verdict struct {
	Level    Level     `json:"level"`
	Findings []Finding `json:"findings"`
}

func (v *Verdict) add(f Finding) {
	v.Findings = append(v.Findings, f)
	v.Level = Max(v.Level, f.Level)
}

// Sensitivity 是主机的固有敏感度，由管理员设定。
type Sensitivity string

const (
	SensLow    Sensitivity = "low"
	SensNormal Sensitivity = "normal"
	SensHigh   Sensitivity = "high"
)

// Final 计算最终等级 = max(命令判级, 声明等级, 主机敏感度加成)。
// 高敏感主机上的写操作（>= L1）升一级，但加成不会把操作变成 L3：
// L3 只由规则直接判定。
func Final(command Level, declared Level, sens Sensitivity) (Level, *Finding) {
	lvl := Max(command, declared)
	if sens == SensHigh && lvl == L1 {
		return L2, &Finding{Level: L2, Rule: "host.sensitivity", Reason: "高敏感主机上的写操作升一级"}
	}
	return lvl, nil
}
