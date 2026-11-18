package kernel_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/store"
)

// captureExec 记下命令和标准输入，用来确认密钥走的是哪条路。
type captureExec struct {
	*executor.Sim
	command, stdin string
}

func (c *captureExec) ExecStdin(ctx context.Context, h inventory.Host, command string, stdin io.Reader, out io.Writer) (int, error) {
	b, _ := io.ReadAll(stdin)
	c.command, c.stdin = command, string(b)
	return 0, nil
}

func TestEnvFileSecretsOnlyTravelOnStdin(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	inv, _ := inventory.Parse([]byte(testInventory))
	ex := &captureExec{Sim: executor.NewSim()}
	secrets := map[string]string{"app/db-password": "s3cr3t-value", "app/api-key": "k-12345"}
	k, err := kernel.New(kernel.Config{DataDir: dir, Secret: func(name string) ([]byte, error) { return []byte(secrets[name]), nil }},
		db, risk.New(), audit.Open(db, nil), executor.Registry{"sim": ex}, inv)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	command := "umask 077 && mkdir -p ~/.hostower/env && cat > ~/.hostower/env/web.env && docker run -d --name web --env-file ~/.hostower/env/web.env nginx"
	spec := kernel.CommandSpec{Hosts: []string{"web-1"}, Commands: map[string]string{"web-1": command},
		Input: &kernel.Input{Kind: "envfile", Label: "web.env", Env: []kernel.EnvRef{{Env: "DB_PASSWORD", Ref: "app/db-password"}, {Env: "API_KEY", Ref: "app/api-key"}}}}
	r, err := k.SubmitCommand(ctx, owner, spec)
	if err != nil {
		t.Fatal(err)
	}
	// 换一条密钥引用，摘要必须不同：批准绑定到“用了哪些密钥”。
	other := spec
	other.Input = &kernel.Input{Kind: "envfile", Label: "web.env", Env: []kernel.EnvRef{{Env: "DB_PASSWORD", Ref: "app/api-key"}}}
	if r2, _ := k.SubmitCommand(ctx, owner, other); r2.Digest == r.Digest {
		t.Fatal("换了密钥引用，摘要应该变")
	}
	if _, err := k.Approve(ctx, r.ID, owner, kernel.Proof{Digest: r.Digest}); err != nil {
		t.Fatal(err)
	}
	got := wait(t, k, r.ID)
	if got.State != kernel.StateDone {
		t.Fatalf("应执行成功: %s %+v", got.State, got.Targets[0])
	}
	if ex.stdin != "DB_PASSWORD=s3cr3t-value\nAPI_KEY=k-12345\n" {
		t.Fatalf("标准输入应是环境文件内容: %q", ex.stdin)
	}
	if strings.Contains(ex.command, "s3cr3t") {
		t.Fatal("值不应出现在命令里")
	}
	// 请求和审计里都不能有值。
	req, _ := json.Marshal(got)
	recs, _ := k.Audit().List(ctx, 0, 100)
	all, _ := json.Marshal(recs)
	for _, leak := range []string{"s3cr3t-value", "k-12345"} {
		if strings.Contains(string(req), leak) || strings.Contains(string(all), leak) {
			t.Fatalf("密钥的值 %q 泄露到了请求或审计里", leak)
		}
	}
}
