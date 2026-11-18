package gateway

import (
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

func TestContainerUpdates(t *testing.T) {
	if got := risk.New().Classify(ctrUpdatesCommand).Level; got != risk.L0 {
		t.Fatalf("容器更新检测的查询必须是 L0，得到 %s", got)
	}
	for in, want := range map[string]string{
		"nginx":                         "nginx:latest",
		"nginx:1.27":                    "nginx:1.27",
		"ghcr.io/mhsanaei/3x-ui:v3.9.0": "ghcr.io/mhsanaei/3x-ui:v3.9.0",
		"registry.example.com:5000/a/b": "registry.example.com:5000/a/b:latest",
		"redis@sha256:abc":              "",
		"sha256:0123":                   "",
	} {
		if got := normalizeImage(in); got != want {
			t.Errorf("normalizeImage(%q) = %q，期望 %q", in, got, want)
		}
	}
}
