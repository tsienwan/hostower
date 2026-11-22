package apps

import (
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

func catalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.List()) < 8 {
		t.Fatalf("内置应用只有 %d 个", len(c.List()))
	}
	return c
}

// 每个内置应用按默认参数生成的安装、卸载命令，交给真正的判级引擎判级：等级必须符合预期。
func TestBuiltinLevels(t *testing.T) {
	c := catalog(t)
	e := risk.New()
	want := map[string]risk.Level{"portainer": risk.L2, "3x-ui": risk.L2}
	for _, a := range c.List() {
		values, err := a.Values(nil)
		if err != nil {
			t.Fatalf("%s：%v", a.Name, err)
		}
		secrets := map[string]string{}
		for _, p := range a.Params {
			if p.Type == "secret" {
				secrets[p.Name] = a.Name + "-" + p.Name
			}
		}
		in, err := a.PlanInstall(a.Name, values, secrets, "")
		if err != nil {
			t.Fatalf("%s：%v", a.Name, err)
		}
		lvl := risk.L1
		if w, ok := want[a.Name]; ok {
			lvl = w
		}
		if got := e.Classify(in.Command); got.Level != lvl {
			t.Errorf("%s 的安装命令：期望 %s，得到 %s：%+v", a.Name, lvl, got.Level, got.Findings)
		}
		if (len(a.Permissions()) > 0) != (lvl == risk.L2) {
			t.Errorf("%s：目录页展示的权限和判级不一致：%+v", a.Name, a.Permissions())
		}
		rec := Record{App: a.Name, Type: a.Type, Instance: a.Name}
		if a.Npm != nil {
			rec.Bins = a.Npm.Bins
		}
		pkg := ""
		if a.Npm != nil {
			pkg = a.Npm.Package
		}
		un, err := PlanAction(rec, pkg, ActUninstall)
		if err != nil {
			t.Fatalf("%s：%v", a.Name, err)
		}
		if got := e.Classify(un).Level; got != risk.L1 {
			t.Errorf("%s 的卸载命令应是 L1，得到 %s", a.Name, got)
		}
		if a.Type == TypeCompose {
			for _, act := range []string{ActStart, ActStop, ActRestart} {
				cmd, _ := PlanAction(rec, "", act)
				if got := e.Classify(cmd).Level; got != risk.L1 {
					t.Errorf("%s %s 应是 L1，得到 %s", a.Name, act, got)
				}
			}
			cmd, _ := PlanAction(rec, "", ActPurge)
			if got := e.Classify(cmd).Level; got != risk.L2 {
				t.Errorf("%s 连数据删除应是 L2，得到 %s", a.Name, got)
			}
		}
	}
	if got := e.Classify(ListCommand).Level; got != risk.L0 {
		t.Errorf("已安装列表的查询必须是 L0，得到 %s", got)
	}
}

func TestRender(t *testing.T) {
	c := catalog(t)
	a, _ := c.Get("redis")
	values, err := a.Values(map[string]string{"port": "6380", "bind": "0.0.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	in, err := a.PlanInstall("cache", values, map[string]string{"password": "redis-pass"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`0.0.0.0:6380:6379`, `com.hostower.app: "cache"`, `container_name: "cache"`, `${REDIS_PASSWORD}`} {
		if !strings.Contains(in.Compose, s) {
			t.Errorf("compose 里缺少 %q：\n%s", s, in.Compose)
		}
	}
	if in.Secrets["REDIS_PASSWORD"] != "redis-pass" || !strings.Contains(in.Command, "cat > .env") || !strings.Contains(in.Command, "-p cache up -d") {
		t.Errorf("命令或密钥不对：%v\n%s", in.Secrets, in.Command)
	}
	if _, err := a.Values(map[string]string{"port": "70000"}); err == nil {
		t.Error("端口越界应被拒绝")
	}
	if _, err := a.Values(map[string]string{"bind": "192.0.2.1"}); err == nil {
		t.Error("不在选项里的值应被拒绝")
	}
	if _, err := a.PlanInstall("cache", values, nil, ""); err == nil {
		t.Error("没选密钥应被拒绝")
	}
	if _, err := a.PlanInstall("../x", values, map[string]string{"password": "p"}, ""); err == nil {
		t.Error("不合法的实例名应被拒绝")
	}
	pg, _ := c.Get("postgres")
	if _, err := pg.Values(map[string]string{"database": "a'; rm -rf /"}); err == nil {
		t.Error("不符合格式的字符串参数应被拒绝")
	}
	cx, _ := c.Get("codex")
	if _, err := cx.PlanInstall("codex", nil, nil, "1.0.0; reboot"); err == nil {
		t.Error("不合法的版本号应被拒绝")
	}
	in, err = cx.PlanInstall("codex", nil, nil, "latest")
	if err != nil || !strings.Contains(in.Command, "@openai/codex@latest ") {
		t.Errorf("换版本失败：%v %s", err, in.Command)
	}
}

func TestParseRejects(t *testing.T) {
	head := "apiVersion: hostower/v1\nkind: App\nname: demo\nversion: \"1\"\ntitle: Demo\ncategory: 运维\nicon: box\nsummary: x\ntype: compose\n"
	for name, body := range map[string]string{
		"隐含 latest": "compose: |\n  services:\n    a:\n      image: nginx\n",
		"明写 latest": "compose: |\n  services:\n    a:\n      image: nginx:latest\n",
		"现场构建":      "compose: |\n  services:\n    a:\n      build: .\n      image: x:1\n",
		"自带 labels": "compose: |\n  services:\n    a:\n      image: x:1\n      labels: {a: b}\n",
		"未知字段":      "unknown: 1\ncompose: |\n  services:\n    a:\n      image: x:1\n",
		"字符串参数没有格式": "params:\n  - {name: p, title: P, type: string}\ncompose: |\n  services:\n    a:\n      image: x:1\n",
		"密钥有默认值":    "params:\n  - {name: p, title: P, type: secret, env: P, default: x}\ncompose: |\n  services:\n    a:\n      image: x:1\n",
		"模板引用未知参数":  "compose: |\n  services:\n    a:\n      image: x:{{ .nope }}\n",
	} {
		if _, err := Parse([]byte(head + body)); err == nil {
			t.Errorf("%s：应被拒绝", name)
		}
	}
}

func TestParseList(t *testing.T) {
	out := `{"app":"redis","version":"7.4-1","type":"compose","instance":"cache","params":{"port":"6379"}}
{"app":"codex","version":"0.169.0","type":"npm","instance":"codex","bins":["codex"]}
not json
---
cache	cache-redis-1	running	Up 2 hours	127.0.0.1:6379->6379/tcp
ghost	ghost-1	exited	Exited (0)
`
	list := ParseList(out)
	if len(list) != 2 || list[0].Instance != "cache" || len(list[0].Containers) != 1 || list[0].Containers[0].State != "running" || list[1].Type != TypeNpm {
		t.Fatalf("解析结果不对：%+v", list)
	}
	if got := ParseList("---\n"); len(got) != 0 {
		t.Fatalf("空列表解析出了 %d 项", len(got))
	}
}

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.169.0", "0.168.9", 1}, {"0.169.0", "0.169.0", 0}, {"1.10.0", "1.9.9", 1},
		{"0.2.2-rc.1", "0.2.2", -1}, {"0.2.2-rc.2", "0.2.2-rc.1", 1}, {"0.2.3-rc.1", "0.2.2", 1},
		{"7.4-2", "7.4-1", 1}, {"17-1", "16-3", 1}, {"lts-2", "lts-1", 1}, {"2.1.326", "2.1.400", -1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d，期望 %d", c.a, c.b, got, c.want)
		}
	}
}

func TestUpdate(t *testing.T) {
	c := catalog(t)
	e := risk.New()
	r, _ := c.Get("redis")
	rec := Record{App: "redis", Type: TypeCompose, Instance: "cache", Version: "7.4-0", Params: map[string]string{"port": "6380", "bind": "127.0.0.1"}, Envs: []string{"REDIS_PASSWORD"}}
	up, err := r.PlanUpdate(rec, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(up.Compose, "127.0.0.1:6380:6379") || strings.Contains(up.Command, "cat > .env") || !strings.Contains(up.Command, "-p cache pull && docker compose -p cache up -d") {
		t.Errorf("更新要沿用参数、不动 .env、先拉取再重建：\n%s", up.Command)
	}
	if got := e.Classify(up.Command).Level; got != risk.L1 {
		t.Errorf("更新 redis 应是 L1，得到 %s", got)
	}
	rec.Envs = nil
	if _, err := r.PlanUpdate(rec, ""); err == nil {
		t.Error("主机上没有需要的密钥时应要求重新安装")
	}
	x, _ := c.Get("3x-ui")
	up, err = x.PlanUpdate(Record{App: "3x-ui", Type: TypeCompose, Instance: "3x-ui"}, "")
	if err != nil || e.Classify(up.Command).Level != risk.L2 {
		t.Errorf("更新 3x-ui 也要按内容判成 L2：%v", err)
	}
	cx, _ := c.Get("codex")
	up, err = cx.PlanUpdate(Record{App: "codex", Type: TypeNpm, Instance: "codex"}, "0.200.0")
	if err != nil || !strings.Contains(up.Command, "@openai/codex@0.200.0") {
		t.Errorf("命令行工具更新到指定版本失败：%v", err)
	}
	logos := 0
	for _, a := range c.List() {
		if a.Logo != "" {
			logos++
		}
	}
	if logos < 7 {
		t.Errorf("只有 %d 个应用有官方图标", logos)
	}
}

func TestCheckSVG(t *testing.T) {
	for _, bad := range []string{`<svg onload="x"><script>alert(1)</script></svg>`, `<svg ><a href="https://example.com"/></svg>`, `<svg ><foreignObject/></svg>`, `<html></html>`} {
		if checkSVG([]byte(bad)) == nil {
			t.Errorf("应拒绝 %q", bad)
		}
	}
}

func TestConfigure(t *testing.T) {
	c := catalog(t)
	e := risk.New()
	r, _ := c.Get("redis")
	rec := Record{App: "redis", Type: TypeCompose, Instance: "cache", Version: r.Version, Params: map[string]string{"port": "6379", "bind": "127.0.0.1"}, Envs: []string{"REDIS_PASSWORD"}}
	// 只改端口：密钥不动，不拉取镜像（版本没变），重建生效。
	in, err := r.PlanConfigure(rec, map[string]string{"port": "6390"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(in.Compose, "127.0.0.1:6390:6379") || strings.Contains(in.Command, "cat > .env") || strings.Contains(in.Command, " pull ") ||
		!strings.Contains(in.Command, "-p cache up -d\n") || !strings.Contains(in.Command, `"port":"6390"`) {
		t.Errorf("改端口的命令不对：\n%s", in.Command)
	}
	if got := e.Classify(in.Command).Level; got != risk.L1 {
		t.Errorf("改设置应是 L1，得到 %s", got)
	}
	// 换密钥：重写 .env。
	in, err = r.PlanConfigure(rec, nil, map[string]string{"password": "new-pass"})
	if err != nil || in.Secrets["REDIS_PASSWORD"] != "new-pass" || !strings.Contains(in.Command, "cat > .env && ") {
		t.Errorf("换密钥不对：%v %+v", err, in)
	}
	// 不合法的参数照样拒绝。
	if _, err := r.PlanConfigure(rec, map[string]string{"port": "99999"}, nil); err == nil {
		t.Error("越界端口应被拒绝")
	}
	// 实例版本比目录旧：改设置时顺带拉取新镜像。
	rec.Version = "7.4-0"
	if in, _ := r.PlanConfigure(rec, nil, nil); !strings.Contains(in.Command, "-p cache pull && ") {
		t.Errorf("版本较旧时应先拉取：\n%s", in.Command)
	}
	cx, _ := c.Get("codex")
	if _, err := cx.PlanConfigure(Record{App: "codex", Type: TypeNpm, Instance: "codex"}, nil, nil); err == nil {
		t.Error("命令行工具没有这种设置")
	}
}

func TestUpgradeBlocked(t *testing.T) {
	c := catalog(t)
	pg, _ := c.Get("postgres")
	old := Record{App: "postgres", Type: TypeCompose, Instance: "db", Version: "17-1", Params: map[string]string{"port": "5432", "bind": "127.0.0.1", "database": "app"}, Envs: []string{"POSTGRES_PASSWORD"}}
	if !pg.UpgradeBlocked("17-1") || pg.UpgradeBlocked(pg.Version) {
		t.Fatal("PostgreSQL 17 不能原地升级到 18")
	}
	if _, err := pg.PlanUpdate(old, ""); err == nil {
		t.Error("PostgreSQL 17 的实例不应能一键更新")
	}
	if _, err := pg.PlanConfigure(old, map[string]string{"port": "5433"}, nil); err == nil {
		t.Error("PostgreSQL 17 的实例改设置会换成 18 的镜像，应拒绝")
	}
	k, _ := c.Get("uptime-kuma")
	if k.UpgradeBlocked("1-1") || k.Upgrade == nil || k.Upgrade.Note == "" {
		t.Error("Uptime Kuma 1 → 2 可以原地升级，并带提示")
	}
}

func TestParseListDigests(t *testing.T) {
	out := "{\"app\":\"uptime-kuma\",\"version\":\"2-1\",\"type\":\"compose\",\"instance\":\"kuma\"}\n---\nkuma\tkuma-kuma-1\trunning\tUp 2 hours\t\n---\nlouislam/uptime-kuma:2\tsha256:abc\n<none>:<none>\t<none>\nredis:8-alpine\tsha256:def\n"
	l, d := ParseListDigests(out)
	if len(l) != 1 || len(l[0].Containers) != 1 || d["louislam/uptime-kuma:2"] != "sha256:abc" || d["redis:8-alpine"] != "sha256:def" || len(d) != 2 {
		t.Fatalf("三段输出解析不对：%+v %v", l, d)
	}
	// 没有容器那一段也要能解析。
	if l, d := ParseListDigests("---\n---\nx:1\tsha256:1\n"); len(l) != 0 || d["x:1"] != "sha256:1" {
		t.Fatalf("空列表解析不对：%+v %v", l, d)
	}
}

// 多个服务时容器名是“实例名-服务名”，不会出现 compose 默认的“项目名-服务名-序号”。
func TestRenderContainerNames(t *testing.T) {
	a := &App{Name: "demo", Type: TypeCompose, Compose: "services:\n  web:\n    image: example/web:1.0\n  db:\n    image: example/db:1.0\n"}
	s, err := a.render(nil, "shop")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`container_name: "shop-web"`, `container_name: "shop-db"`} {
		if !strings.Contains(s, w) {
			t.Errorf("缺少 %q：\n%s", w, s)
		}
	}
}

// 上手指南：每个内置应用都要有；“读取初始账号”的命令交给真正的判级引擎，必须是只读的 L0。
func TestGuides(t *testing.T) {
	c := catalog(t)
	e := risk.New()
	for _, a := range c.List() {
		if a.Guide == nil {
			t.Errorf("%s 没有上手指南", a.Name)
			continue
		}
		values, err := a.Values(nil)
		if err != nil {
			t.Fatalf("%s：%v", a.Name, err)
		}
		rec := Record{App: a.Name, Type: a.Type, Instance: a.Name, Params: values}
		g, err := a.GuideFor(rec)
		if err != nil || g == nil {
			t.Fatalf("%s 的上手指南渲染不出来：%v", a.Name, err)
		}
		for _, s := range append([]string{g.Account}, g.Steps...) {
			if strings.Contains(s, "{{") {
				t.Errorf("%s 的上手指南里还有没填的参数：%s", a.Name, s)
			}
		}
		if a.Type == TypeCompose && a.Name != "postgres" && a.Name != "mariadb" && a.Name != "redis" && len(g.Open) == 0 {
			t.Errorf("%s 是带页面的容器应用，上手指南里要有访问入口", a.Name)
		}
		if a.Guide.Reveal == nil {
			continue
		}
		cmd, err := a.RevealCommand(rec)
		if err != nil {
			t.Fatalf("%s：%v", a.Name, err)
		}
		if v := e.Classify(cmd); v.Level != risk.L0 {
			t.Errorf("%s 读取初始账号的命令被判为 %s，页面上会查不出来：%s", a.Name, v.Level, cmd)
		}
	}
}

// 主机上记的参数不直接信：渲染指南和读取命令之前重新校验，夹带命令的值渲染不出来。
func TestGuideRejectsBadParams(t *testing.T) {
	c := catalog(t)
	a, _ := c.Get("uptime-kuma")
	bad := Record{App: a.Name, Type: a.Type, Instance: a.Name, Params: map[string]string{"bind": "0.0.0.0", "port": "80; reboot"}}
	if g, err := a.GuideFor(bad); err == nil {
		t.Errorf("不合法的端口应该被拒绝，得到 %+v", g)
	}
	x, _ := c.Get("3x-ui")
	if _, err := x.RevealCommand(Record{App: x.Name, Type: x.Type, Instance: "x; reboot"}); err == nil {
		t.Error("不合法的实例名应该被拒绝")
	}
	g, err := a.GuideFor(Record{App: a.Name, Type: a.Type, Instance: "kuma", Params: map[string]string{"bind": "127.0.0.1", "port": "3001"}})
	if err != nil || len(g.Open) != 1 || !g.Open[0].Local || g.Open[0].Port != "3001" {
		t.Errorf("只在本机监听的入口要标出来：%+v %v", g, err)
	}
}
