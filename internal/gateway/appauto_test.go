package gateway

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/store"
)

const appInventory = `
apiVersion: hostower/v1
kind: Inventory
hosts:
  web-1: {addr: 192.0.2.10, executor: sim, tags: [docker]}
  web-2: {addr: 192.0.2.11, executor: sim, tags: [docker]}
`

var testOwner = kernel.Actor{ID: "u1", Name: "owner", Kind: kernel.Human, Entry: "web", MaxLevel: risk.L2, CanApprove: true}

func appSetup(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	inv, err := inventory.Parse([]byte(appInventory))
	if err != nil {
		t.Fatal(err)
	}
	k, err := kernel.New(kernel.Config{DataDir: dir}, db, risk.New(), audit.Open(db, nil), executor.Registry{"sim": executor.NewSim()}, inv)
	if err != nil {
		t.Fatal(err)
	}
	authSvc, err := auth.New(db, "http://localhost:8693", true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{PublicURL: "https://hostower.example.com", Kernel: k, Auth: authSvc, DB: db, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	// 测试不碰网络：镜像仓库和 npm 都指到一个连不上的本机地址（查不到就当没有更新）。
	s.appRegistry.RegistryBase = func(string) string { return "http://127.0.0.1:1" }
	s.appRegistry.Base = "http://127.0.0.1:1"
	return s
}

// runApproved 以人的身份提交并批准一条命令，等它执行完。
func runApproved(t *testing.T, s *Server, host, cmd string) {
	t.Helper()
	ctx := context.Background()
	r, err := s.Kernel.SubmitCommand(ctx, testOwner, kernel.CommandSpec{Hosts: []string{host}, Commands: map[string]string{host: cmd}})
	if err != nil {
		t.Fatal(err)
	}
	if r.State == kernel.StatePending {
		if _, err := s.Kernel.Approve(ctx, r.ID, testOwner, kernel.Proof{Digest: r.Digest, Reauthed: true, Typed: r.Confirm}); err != nil {
			t.Fatal(err)
		}
	}
	waitState(t, s, r.ID, kernel.StateDone)
}

func waitState(t *testing.T, s *Server, id string, want kernel.State) *kernel.Request {
	t.Helper()
	for i := 0; i < 200; i++ {
		r, err := s.Kernel.Get(id)
		if err == nil && r.State == want {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	r, _ := s.Kernel.Get(id)
	t.Fatalf("请求 %s 没有到 %s，现在是 %+v", id, want, r)
	return nil
}

// 装一个“旧版本”的 Redis：用真正的安装命令，只把记下的版本号改旧。
func installOld(t *testing.T, s *Server, host string) {
	t.Helper()
	a, _ := s.apps.Get("redis")
	vals, _ := a.Values(nil)
	in, err := a.PlanInstall("cache", vals, map[string]string{"password": "p"}, "")
	if err != nil {
		t.Fatal(err)
	}
	cmd := strings.Replace(strings.Replace(in.Command, `"version":"`+a.Version+`"`, `"version":"7.4-0"`, 1), "cat > .env && ", "", 1)
	runApproved(t, s, host, cmd)
}

func TestAppAutoUpdate(t *testing.T) {
	s := appSetup(t)
	ctx := context.Background()
	installOld(t, s, "web-1")
	installOld(t, s, "web-2")

	n, err := s.autoUpdateCheck(ctx)
	if err != nil || n != 1 {
		t.Fatalf("应该为两台主机合并提交 1 个更新请求，得到 %d，%v", n, err)
	}
	var req *kernel.Request
	for _, r := range s.Kernel.List(10) {
		if r.Actor.Kind == kernel.System {
			req = r
		}
	}
	if req == nil || req.State != kernel.StatePending || len(req.Targets) != 2 || req.Level != risk.L1 || !strings.Contains(req.Intent, "更新应用实例 cache") {
		t.Fatalf("自动发起的请求不对：%+v", req)
	}
	// 系统身份不能批准任何东西。
	if _, err := s.Kernel.Approve(ctx, req.ID, appAutoActor, kernel.Proof{Digest: req.Digest}); err == nil {
		t.Fatal("系统身份不应能批准")
	}
	// 还在等批准：再查一次不重复提交。
	if n, _ := s.autoUpdateCheck(ctx); n != 0 {
		t.Fatalf("等批准时不应重复提交，提交了 %d 个", n)
	}
	// 人拒绝之后：这个版本不再打扰。
	if _, err := s.Kernel.Deny(ctx, req.ID, testOwner, "先不更新"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.autoUpdateCheck(ctx); n != 0 {
		t.Fatalf("被拒绝的版本不应再提交，提交了 %d 个", n)
	}
	// 过期或失败的，隔一天再提交。
	for k, v := range s.appAuto.sent {
		v.At = v.At.Add(-25 * time.Hour)
		s.appAuto.sent[k] = v
	}
	if n, _ := s.autoUpdateCheck(ctx); n != 0 {
		t.Fatalf("拒绝过的版本过了一天也不应再提交，提交了 %d 个", n)
	}
	redis, _ := s.apps.Get("redis")
	s.appAuto.sent = map[string]appAutoSent{"web-1/cache@" + redis.Version: {Req: "rq_gone", At: time.Now().Add(-25 * time.Hour)}}
	n, err = s.autoUpdateCheck(ctx)
	if err != nil || n != 1 {
		t.Fatalf("找不到上次的请求、过了一天，应重新提交：%d %v", n, err)
	}
	// 批准后执行，主机上的版本变新，再查就没有更新了。
	for _, r := range s.Kernel.List(10) {
		if r.Actor.Kind == kernel.System && r.State == kernel.StatePending {
			if _, err := s.Kernel.Approve(ctx, r.ID, testOwner, kernel.Proof{Digest: r.Digest, Reauthed: true, Typed: r.Confirm}); err != nil {
				t.Fatal(err)
			}
			waitState(t, s, r.ID, kernel.StateDone)
		}
	}
	s.appAuto.sent = map[string]appAutoSent{}
	if n, _ := s.autoUpdateCheck(ctx); n != 0 {
		t.Fatalf("都更新过了，不应再提交，提交了 %d 个", n)
	}

	// 版本没变、但镜像仓库里同一个标签有了新构建：提交一次“更新”（重新拉取），等批准时不重复。
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/manifests/8-alpine") {
			w.Header().Set("Docker-Content-Digest", "sha256:fresh")
			return
		}
		w.WriteHeader(404)
	}))
	defer reg.Close()
	s.appRegistry.RegistryBase = func(string) string { return reg.URL }
	hs := s.installedOn(ctx, appAutoActor, []string{"web-1"})
	if len(hs[0].Installed) != 1 || !hs[0].Installed[0].Rebuilt || hs[0].Installed[0].Update != "" {
		t.Fatalf("应识别出镜像有新构建：%+v", hs[0].Installed)
	}
	if n, err := s.autoUpdateCheck(ctx); err != nil || n != 1 {
		t.Fatalf("新构建应自动发起 1 个更新请求，得到 %d %v", n, err)
	}
	if n, _ := s.autoUpdateCheck(ctx); n != 0 {
		t.Fatalf("同一个新构建在等批准时不应重复提交，提交了 %d 个", n)
	}
}
