package speedtest

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/store"
)

func TestCommandAndParse(t *testing.T) {
	routes := []string{Server, "198.51.100.7"}
	if _, err := Command(7, nil); err == nil {
		t.Fatal("不在可选值里的数据量应该被拒绝")
	}
	if _, err := Command(25, []string{"x'; rm -rf / #"}); err == nil {
		t.Fatal("带引号和空格的目标应该被拒绝")
	}
	c, err := Command(25, routes)
	if err != nil {
		t.Fatal(err)
	}
	if sp, ok := SpecOf(c); !ok || sp.Method != MethodCloudflare || sp.SizeMB != 25 || len(sp.Routes) != 2 || sp.Routes[1] != "198.51.100.7" {
		t.Fatalf("命令里读回的参数不对：%+v", sp)
	}
	if _, ok := SpecOf(c + " ; rm -rf /"); ok {
		t.Fatal("改过的命令不应该认得出来")
	}
	if c0, _ := Command(10, nil); func() int { s, _ := SpecOf(c0); return s.SizeMB }() != 10 || strings.Contains(c0, "traceroute") {
		t.Fatal("没有追踪目标时命令里不该有路由追踪")
	}
	out := "speedtest trace ip=192.0.2.10\nspeedtest trace colo=SJC\nspeedtest trace loc=US\nspeedtest trace tls=TLSv1.3\nspeedtest trace bad=<b>\n" +
		"speedtest ping dns=0.008 connect=0.081 tls=0.150 ttfb=0.200\nspeedtest ping dns=0.002 connect=0.074 tls=0.140 ttfb=0.180\n" +
		"speedtest route 198.51.100.7 traceroute to 198.51.100.7 (198.51.100.7), 30 hops max, 60 byte packets\n" +
		"speedtest route 198.51.100.7  1  10.0.0.1  0.412 ms\n" + // 私有地址 hygiene:allow
		"speedtest route 198.51.100.7  2  *\n" +
		"speedtest route 198.51.100.7  3  59.43.1.1  150.2 ms\n" + // hygiene:allow
		"speedtest route 198.51.100.7  4  59.43.2.2  160.5 ms\n" + // hygiene:allow
		"speedtest route 198.51.100.7  5  198.51.100.7  170.1 ms\n" +
		"speedtest route 203.0.113.9  1?: [LOCALHOST]   pmtu 1500\n" +
		"speedtest route 203.0.113.9  1:  192.0.2.1   0.5ms\n" +
		"speedtest route 203.0.113.9  2:  218.105.3.3   180.0ms\n" + // hygiene:allow
		"speedtest route 203.0.113.9  3:  no reply\n" +
		"speedtest down speed=6000000 bytes=25000000 time=4.2 ttfb=0.21 ip=203.0.113.5 http=2\nspeedtest up speed=3000000 bytes=6250000 time=2.1\n"
	rep := Parse(out)
	if rep.Error != "" || rep.PingMS < 71.9 || rep.PingMS > 72.1 || rep.DownBps != 48e6 || rep.UpBps != 24e6 {
		t.Fatalf("解析不对：%+v", rep)
	}
	d := rep.Detail
	if len(d.Pings) != 2 || d.Pings[1].TCP != 72 || d.Pings[1].TLS != 66 || d.Pings[1].Wait != 40 {
		t.Fatalf("延迟明细不对：%+v", d.Pings)
	}
	if d.Net.IP != "192.0.2.10" || d.Net.Colo != "SJC" || d.Net.ServerIP != "203.0.113.5" || d.Net.HTTP != "2" || d.Down.MS != 4200 {
		t.Fatalf("网络信息不对：%+v %+v", d.Net, d.Down)
	}
	if len(d.Routes) != 2 || len(d.Routes[0].Hops) != 5 || d.Routes[0].Hops[0].Net != "LAN" || d.Routes[0].Line != "电信 CN2 GIA" {
		t.Fatalf("路由不对：%+v", d.Routes)
	}
	if r := d.Routes[1]; len(r.Hops) != 3 || r.Hops[0].IP != "192.0.2.1" || r.Line != "联通 9929" || r.Hops[2].IP != "" {
		t.Fatalf("tracepath 的路由不对：%+v", r)
	}
	if lineOf([]string{"163", "CN2"}) != "电信 CN2 GT" || lineOf([]string{"4837", "CMI"}) != "联通 4837 + 移动 CMI" {
		t.Fatal("线路推测不对")
	}
	if r := Parse("speedtest route 198.51.100.7 error=no-traceroute\n").Detail.Routes; len(r) != 1 || r[0].Error == "" {
		t.Fatalf("没有 traceroute 时应该说明：%+v", r)
	}
	ci, _ := CommandWith(25, true, routes)
	if s, _ := SpecOf(ci); s.SizeMB != 25 || ci == c {
		t.Fatal("带安装的测速命令也应该认得出来")
	}
	for _, out := range []string{"speedtest installing curl\nspeedtest error=install-failed\n", "speedtest error=no-package-manager\n"} {
		if Parse(out).Error == "" {
			t.Fatalf("安装失败应该报错：%q", out)
		}
	}
	if Parse("speedtest error=no-curl\n").Error == "" {
		t.Fatal("没有 curl 时应该报错")
	}
	if Parse("speedtest down speed=0 bytes=0\nspeedtest up speed=0 bytes=0\n").Error == "" {
		t.Fatal("下载失败时应该报错")
	}
}

func TestTargets(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	if len(s.Targets()) != 1 || s.Targets()[0].Target != Server {
		t.Fatal("默认只追踪测速节点")
	}
	for _, bad := range [][]Target{{{Name: "a", Target: "a b"}}, {{Target: "x.example.com"}, {Target: "x.example.com"}}, make([]Target, MaxRoutes+1)} {
		if s.SetTargets(context.Background(), bad) == nil {
			t.Fatalf("应该被拒绝：%+v", bad)
		}
	}
	want := []Target{{Name: "上海电信", Target: "198.51.100.7"}}
	if err := s.SetTargets(context.Background(), want); err != nil || s.Targets()[0] != want[0] {
		t.Fatalf("没存下来：%v", err)
	}
}

func TestRetentionAndDelete(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	ctx := context.Background()
	if s.Retention() != DefaultRetention {
		t.Fatal("没有设置时应该用默认规则")
	}
	now := time.Now()
	ins := func(id, host string, age time.Duration) {
		db.Exec(`INSERT INTO speedtests (request_id, host, ts, ping_ms, down_bps, up_bps, size_mb, error) VALUES (?,?,?,0,0,0,10,'')`, id, host, now.Add(-age).UnixMilli())
	}
	for i := 0; i < 15; i++ {
		ins("rq_"+strconv.Itoa(i), "web-1", time.Duration(i)*time.Hour)
	}
	ins("rq_old", "web-2", 40*24*time.Hour)
	ins("rq_new", "web-2", time.Hour)
	if s.SetRetention(ctx, Retention{Days: 3, PerHost: 7}) == nil {
		t.Fatal("不在可选值里的规则应该被拒绝")
	}
	if err := s.SetRetention(ctx, Retention{Days: 30, PerHost: 10}); err != nil {
		t.Fatal(err)
	}
	if s.Retention() != (Retention{Days: 30, PerHost: 10}) {
		t.Fatal("规则没存下来")
	}
	// web-1 留最新的 10 条；web-2 那条 40 天前的过期了。
	if n := s.Count(ctx); n != 11 {
		t.Fatalf("清理后应该剩 11 条，实际 %d", n)
	}
	if n, _ := s.Delete(ctx, []Key{{"rq_0", "web-1"}, {"rq_0", "web-2"}}, false); n != 1 {
		t.Fatalf("只应该删掉对得上的那一条，实际删了 %d", n)
	}
	if n, _ := s.Delete(ctx, nil, true); n != 10 || s.Count(ctx) != 0 {
		t.Fatal("清空后应该一条不剩")
	}
}

func TestRecord(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	c, _ := Command(10, []string{"198.51.100.7"})
	now := time.Now()
	r := &kernel.Request{ID: "rq_1", Snippet: Tag, State: kernel.StateDone, FinishedAt: &now, Targets: []*kernel.Target{
		{Host: "web-1", Command: c, State: "ok", Output: "speedtest ping dns=0 connect=0.02\nspeedtest route 198.51.100.7  1  198.51.100.7  1.0 ms\nspeedtest down speed=1000 bytes=10000000\nspeedtest up speed=500 bytes=2500000\n"},
		{Host: "web-2", Command: "echo speedtest down speed=1 bytes=1", State: "ok", Output: "speedtest down speed=1 bytes=1"},
	}}
	s.record(r)
	s.record(r) // 同一个请求再来一次不重复
	s.record(&kernel.Request{ID: "rq_2", Snippet: "other@1", State: kernel.StateDone, Targets: r.Targets})
	got, err := s.Recent(context.Background(), 10)
	if err != nil || len(got) != 1 || got[0].Host != "web-1" || got[0].DownBps != 8000 || got[0].SizeMB != 10 {
		t.Fatalf("只应该记下我们生成的那条命令的结果：%+v %v", got, err)
	}
	full, err := s.Get(context.Background(), Key{"rq_1", "web-1"})
	if err != nil || full.Detail == nil || len(full.Detail.Pings) != 1 || len(full.Detail.Routes) != 1 || full.Detail.Routes[0].Hops[0].MS != 1 {
		t.Fatalf("明细没存下来：%+v %v", full, err)
	}
}

func TestInstallTools(t *testing.T) {
	routes := []string{Server}
	withTrace, _ := CommandWith(25, true, routes)
	curlOnly, _ := CommandWith(25, true, nil)
	if !strings.Contains(withTrace, "speedtest installing traceroute") || !strings.Contains(withTrace, "apt-get install -y -q traceroute") {
		t.Fatal("有追踪目标时应该顺便安装 traceroute")
	}
	if strings.Contains(curlOnly, "traceroute") {
		t.Fatal("没有追踪目标时不该安装 traceroute")
	}
	if s, _ := SpecOf(withTrace); s.SizeMB != 25 || len(s.Routes) != 1 {
		t.Fatal("带安装 traceroute 的命令也应该认得出来")
	}
	if c, tr := Tools("/usr/bin/curl\n/usr/sbin/tracepath\n"); !c || !tr {
		t.Fatal("应该认出 curl 和 tracepath")
	}
	if c, tr := Tools("/usr/bin/traceroute\n"); c || !tr {
		t.Fatal("只有 traceroute 时不该认成有 curl")
	}
	if c, tr := Tools(""); c || tr {
		t.Fatal("什么都没有")
	}
}

// CN2 GIA 和 GT 按顺序区分：163 骨干在 CN2 段外侧才是 GT。
func TestLineOfOrder(t *testing.T) {
	cases := []struct {
		seq  []string
		want string
	}{
		{[]string{"CN2", "CN2", "163", "163"}, "电信 CN2 GT"},       // 从境外进来：CN2 之后换 163 骨干
		{[]string{"163", "163", "CN2", "CN2"}, "电信 CN2 GT"},       // 从境内出去：先 163 再 CN2
		{[]string{"CN2", "163", "CN2", "CN2"}, "电信 CN2 GIA"},      // 夹在 CN2 中间的 163 不算换网
		{[]string{"CN2", "CHINANET", "CN2", "CN2"}, "电信 CN2 GIA"}, // 只按自治系统号认出的电信地址
		{[]string{"CN2", "CN2", "CHINANET"}, "电信 CN2 GIA"},        // CN2 之后进了省网（城域网），不是 163 骨干
		{[]string{"CHINANET", "CHINANET"}, "电信 163"},              // 不走 CN2 的电信
		{[]string{"9929", "CN2"}, "电信 CN2 GIA + 联通 9929"},
		{[]string{"CHINANET", "CHINANET", "163", "CN2", "163"}, "电信 163"},                             // 夹在 163 中间的单个 CN2 不算走了 CN2
		{[]string{"CHINANET", "163", "CN2", "CN2", "163"}, "电信 CN2 GT"},                               // 连续两跳 CN2 是真的换了网
		{[]string{"CN2", "CHINANET", "CN2", "CN2", "CMNET", "CMNET", "163"}, "电信 CN2 GIA + 移动 CMNET"}, // CN2 交给移动后再出现的 163 不算
		{[]string{"CN2", "CHINANET", "CN2", "163", "CN2", "163", "CHINANET"}, "电信 CN2 GT"},            // CN2 和 163 交替，最后落在 163
		{[]string{"CN2", "CHINANET", "CN2", "CHINANET", "CHINANET"}, "电信 CN2 GIA"},                    // CN2 直接进省网
	}
	for _, c := range cases {
		if got := lineOf(c.seq); got != c.want {
			t.Errorf("%v：得到 %q，期望 %q", c.seq, got, c.want)
		}
	}
}

func TestLineOfPremium(t *testing.T) {
	if got := lineOf([]string{"CMI", "CMIN2"}); got != "移动 CMIN2" {
		t.Errorf("移动精品网：%q", got)
	}
	if got := lineOf([]string{"CTGNet", "CHINANET"}); got != "电信 CTGNet" {
		t.Errorf("电信国际：%q", got)
	}
	if got := lineOf([]string{"CTGNet", "CN2", "CN2"}); got != "电信 CN2 GIA" {
		t.Errorf("CTGNet 接 CN2：%q", got)
	}
}
