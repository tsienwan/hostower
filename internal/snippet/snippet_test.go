package snippet

import (
	"testing"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

const sample = `
apiVersion: hostower/v1
kind: Snippet
name: tail-log
title: 查看日志
level: L0
params:
  - {name: file, type: path, required: true}
  - {name: lines, type: int, default: 50, min: 1, max: 1000}
  - {name: keyword, type: string}
command: tail -n {{ .lines }} {{ .file }} | grep -- {{ .keyword }} ; ls {{ .host.vars.root }}
`

func TestRenderEscapes(t *testing.T) {
	s, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	h := inventory.Host{Name: "web-1", Addr: "192.0.2.10", Vars: map[string]string{"root": "/opt/my stack"}}

	// 注入尝试：值里带 shell 元字符，必须被当成一个单词。
	params, err := s.Validate(map[string]any{"file": "/var/log/syslog", "keyword": "x; rm -rf / #"})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := s.Render(params, h)
	if err != nil {
		t.Fatal(err)
	}
	want := `tail -n 50 /var/log/syslog | grep -- 'x; rm -rf / #' ; ls '/opt/my stack'`
	if cmd != want {
		t.Fatalf("渲染结果\n got: %s\nwant: %s", cmd, want)
	}
	if lvl := risk.New().Classify(cmd).Level; lvl != risk.L0 {
		t.Fatalf("转义后的注入不应改变判级，得到 %s", lvl)
	}

	params, _ = s.Validate(map[string]any{"file": "/var/log/syslog", "keyword": `a'$(reboot)'b`})
	cmd, _ = s.Render(params, h)
	if lvl := risk.New().Classify(cmd).Level; lvl != risk.L0 {
		t.Fatalf("单引号逃逸不应成功: %s → %s", cmd, lvl)
	}
}

func TestValidate(t *testing.T) {
	s, _ := Parse([]byte(sample))
	bad := []map[string]any{
		{},
		{"file": "relative/path"},
		{"file": "/var/../etc/shadow"},
		{"file": "/x", "lines": 5000},
		{"file": "/x", "lines": "abc"},
		{"file": "/x", "nope": 1},
		{"file": "/x\nrm -rf /"},
	}
	for _, in := range bad {
		if _, err := s.Validate(in); err == nil {
			t.Errorf("应拒绝 %v", in)
		}
	}
	if _, err := s.Render(map[string]string{"file": "/x", "lines": "1", "keyword": ""}, inventory.Host{Name: "h"}); err == nil {
		t.Error("缺少主机变量时应报错，而不是渲染出空值")
	}
}

func TestFromSpec(t *testing.T) {
	min, max := 1, 10
	spec := Snippet{Name: "scale", Title: "扩容", Level: risk.L1, Command: "cd {{ .host.vars.root }} && docker compose up -d --scale {{ .svc }}={{ .n }}",
		Params: []Param{{Name: "svc", Type: "enum", Values: []string{"web", "app"}, Required: true}, {Name: "n", Type: "int", Min: &min, Max: &max, Default: 2}}}
	s, data, err := FromSpec(spec)
	if err != nil {
		t.Fatal(err)
	}
	// 生成的 YAML 能被文件加载的同一个解析器读回来。
	back, err := Parse(data)
	if err != nil || back.Name != "scale" || len(back.Params) != 2 || back.Level != risk.L1 {
		t.Fatalf("YAML 往返失败：%v\n%s", err, data)
	}
	if _, err := s.Validate(map[string]any{"svc": "db"}); err == nil {
		t.Error("枚举之外的值应被拒绝")
	}
	bad := spec
	bad.Command = "echo {{ .nope }}"
	if _, _, err := FromSpec(bad); err == nil {
		t.Error("引用未定义参数的模板应被拒绝")
	}
	bad = spec
	bad.Selector = "tag:"
	if _, _, err := FromSpec(bad); err == nil {
		t.Error("无效的默认目标应被拒绝")
	}
}
