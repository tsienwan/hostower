package risk

import (
	"os"
	"strings"
	"testing"
)

// TestTable 用 testdata/table.txt 锁定判级行为。
func TestTable(t *testing.T) {
	data, err := os.ReadFile("testdata/table.txt")
	if err != nil {
		t.Fatal(err)
	}
	e := New()
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	n := 0
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "# ") || line == "#" {
			continue
		}
		lvlText, cmd, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("第 %d 行格式错误: %q", i+1, line)
		}
		want, err := ParseLevel(lvlText)
		if err != nil {
			t.Fatalf("第 %d 行: %v", i+1, err)
		}
		// heredoc：把后续行并进来，直到结束标记。
		if idx := strings.Index(cmd, "<<"); idx >= 0 && !strings.Contains(cmd, "<<<") {
			// 结束标记是 << 后面的第一个词（去掉 - 和引号），后面还可以接别的命令。
			tag := strings.TrimLeft(cmd[idx+2:], "- ")
			if f := strings.Fields(tag); len(f) > 0 {
				tag = strings.Trim(f[0], `'"`)
			}
			for i+1 < len(lines) {
				i++
				cmd += "\n" + lines[i]
				if lines[i] == tag {
					break
				}
			}
		}
		n++
		got := e.Classify(cmd)
		if got.Level != want {
			t.Errorf("第 %d 行 %q: 期望 %s，得到 %s，理由 %+v", i+1, cmd, want, got.Level, got.Findings)
		}
	}
	if n < 150 {
		t.Fatalf("测试表只有 %d 条，太少", n)
	}
}

func TestFinal(t *testing.T) {
	cases := []struct {
		cmd, declared Level
		sens          Sensitivity
		want          Level
	}{
		{L0, L0, SensHigh, L0},
		{L1, L0, SensHigh, L2},
		{L1, L0, SensNormal, L1},
		{L0, L1, SensHigh, L2},
		{L2, L0, SensHigh, L2},
		{L3, L0, SensLow, L3},
		{L0, L2, SensNormal, L2},
	}
	for _, c := range cases {
		if got, _ := Final(c.cmd, c.declared, c.sens); got != c.want {
			t.Errorf("Final(%s,%s,%s)=%s 期望 %s", c.cmd, c.declared, c.sens, got, c.want)
		}
	}
}

func TestPackOnlyTightens(t *testing.T) {
	pack, err := ParsePack([]byte(`
apiVersion: hostower/v1
kind: RulePack
name: demo
rules:
  - {id: restart, command: systemctl, args: "restart", level: L2, reason: 重启要二次确认}
  - {id: df, command: rm, level: L1, reason: 试图放宽}
`))
	if err != nil {
		t.Fatal(err)
	}
	e := New(pack...)
	if got := e.Classify("systemctl restart nginx").Level; got != L2 {
		t.Errorf("规则包应加严到 L2，得到 %s", got)
	}
	if got := e.Classify("rm -rf /opt/data").Level; got != L2 {
		t.Errorf("规则包不能放宽，得到 %s", got)
	}
	if got := e.Classify("rm -rf /").Level; got != L3 {
		t.Errorf("L3 不可放宽，得到 %s", got)
	}
	if _, err := ParsePack([]byte("apiVersion: hostower/v1\nkind: RulePack\nname: x\nrules:\n  - {id: a, command: rm, level: L0}\n")); err == nil {
		t.Error("声明 L0 的规则包应被拒绝")
	}
}

func BenchmarkClassify(b *testing.B) {
	e := New()
	for i := 0; i < b.N; i++ {
		e.Classify("sudo systemctl restart nginx && docker ps | grep web > /tmp/out")
	}
}
