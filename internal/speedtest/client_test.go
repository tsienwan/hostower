package speedtest

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/store"
)

// 回程路由的命令：地址只能是合法的 IP；能从命令里认回来。
func TestClientCommand(t *testing.T) {
	e := risk.New()
	for _, install := range []bool{false, true} {
		cmd, err := ClientCommand("198.51.100.7", install)
		if err != nil {
			t.Fatal(err)
		}
		// 只追踪和 ping 是只读的（L0）；带上安装 traceroute 就是变更软件包（L1）。
		want := risk.L0
		if install {
			want = risk.L1
		}
		if got := e.Classify(cmd); got.Level != want {
			t.Fatalf("应该是 %s，现在是 %s\n%s", want, got.Level, cmd)
		}
		spec, ok := SpecOf(cmd)
		if !ok || spec.Method != MethodClient || len(spec.Routes) != 1 || spec.Routes[0] != "198.51.100.7" {
			t.Fatalf("没有认回来：%+v %v", spec, ok)
		}
		if _, ok := SpecOf(cmd + "; id"); ok {
			t.Fatal("改过的命令不应该被认成测速")
		}
	}
	if cmd, err := ClientCommand("::ffff:198.51.100.7", false); err != nil || !strings.Contains(cmd, "'198.51.100.7'") {
		t.Fatalf("映射成 IPv6 的 IPv4 地址应该还原：%v %s", err, cmd)
	}
	if _, err := ClientCommand("2001:db8::7", false); err != nil {
		t.Fatalf("IPv6 地址应该能追踪：%v", err)
	}
	for _, bad := range []string{"", "example.com", "198.51.100.7; id", "fe80::1%eth0", "0.0.0.0", "'$(id)'", "::1"} {
		if _, err := ClientCommand(bad, false); err == nil {
			t.Fatalf("%q 不应该被接受", bad)
		}
	}
}

// 输出：路由标成回程、认出线路，ping 的统计读出来；没有下载和上传不算出错。
func TestClientParse(t *testing.T) {
	out := "speedtest client ip=198.51.100.7\n" +
		"speedtest route 198.51.100.7 traceroute to 198.51.100.7, 30 hops max\n" +
		"speedtest route 198.51.100.7  1  10.0.0.1  0.4 ms\n" + // hygiene:allow
		"speedtest route 198.51.100.7  2  59.43.182.1  31.2 ms\n" + // 公开的运营商骨干网段 hygiene:allow
		"speedtest route 198.51.100.7  3  *\n" +
		"speedtest route 198.51.100.7  4  198.51.100.7  160.5 ms\n" +
		"speedtest back 10 packets transmitted, 9 received, 10% packet loss, time 1811ms\n" +
		"speedtest back rtt min/avg/max/mdev = 158.1/160.2/171.9/3.4 ms\n"
	rep := Parse(out)
	if rep.Error != "" {
		t.Fatalf("不应该出错：%s", rep.Error)
	}
	d := rep.Detail
	if len(d.Routes) != 1 || len(d.Routes[0].Hops) != 4 || d.Routes[0].Line != "电信 CN2 GIA" {
		t.Fatalf("路由读得不对：%+v", d.Routes)
	}
	if b := d.Back; b == nil || b.Sent != 10 || b.Recv != 9 || b.Avg != 160.2 || b.Jitter != 3.4 {
		t.Fatalf("ping 读得不对：%+v", d.Back)
	}
	// 对方不回应：只有丢包数。busybox 的写法也认。
	b := Parse("speedtest client ip=198.51.100.7\nspeedtest back 10 packets transmitted, 0 packets received, 100% packet loss\n").Detail.Back
	if b == nil || b.Sent != 10 || b.Recv != 0 || b.Avg != 0 {
		t.Fatalf("不回应时读得不对：%+v", b)
	}
}

// 贴进来的去程路由：认 Windows 的 tracert（中英文）、traceroute 和 mtr；别的文字都丢掉。
func TestParsePasted(t *testing.T) {
	win := "Tracing route to example.com [198.51.100.7]\r\nover a maximum of 30 hops:\r\n\r\n" +
		"  1    <1 ms    <1 ms    <1 ms  192.168.1.1\r\n" + // hygiene:allow
		"  2     3 ms     2 ms     3 ms  gw.example.com [192.0.2.1]\r\n" +
		"  3     *        *        *     Request timed out.\r\n" +
		"  4    38 ms    37 ms    38 ms  219.158.3.1\r\n" + // 公开的运营商骨干网段 hygiene:allow
		"  5   162 ms   161 ms   160 ms  198.51.100.7\r\n\r\nTrace complete.\r\n"
	r, err := ParsePasted(win)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hops) != 5 || r.Hops[0].IP != "192.168.1.1" || r.Hops[0].Net != "LAN" || r.Hops[0].MS != 1 || r.Hops[1].IP != "192.0.2.1" || // hygiene:allow
		r.Hops[2].IP != "" || r.Hops[4].MS != 162 || r.Line != "联通 4837" || r.Dir != "forward" || r.Target != ForwardTarget {
		t.Fatalf("tracert 读得不对：%+v", r)
	}
	zh := "通过最多 30 个跃点跟踪到 198.51.100.7 的路由\n\n  1     1 毫秒    <1 毫秒    <1 毫秒 192.168.1.1\n  2     *        *        *     请求超时。\n  3    25 毫秒    24 毫秒    25 毫秒 198.51.100.7\n" // hygiene:allow
	if r, err = ParsePasted(zh); err != nil || len(r.Hops) != 3 || r.Hops[2].IP != "198.51.100.7" || r.Hops[2].MS != 25 {
		t.Fatalf("中文 tracert 读得不对：%v %+v", err, r)
	}
	unix := "traceroute to 198.51.100.7 (198.51.100.7), 30 hops max, 60 byte packets\n 1  192.0.2.1 (192.0.2.1)  0.412 ms  0.398 ms\n 2  * * *\n 3  198.51.100.7 (198.51.100.7)  12.5 ms\n"
	if r, err = ParsePasted(unix); err != nil || len(r.Hops) != 3 || r.Hops[0].MS != 0.41 || r.Hops[2].IP != "198.51.100.7" {
		t.Fatalf("traceroute 读得不对：%v %+v", err, r)
	}
	mtr := "HOST: web-1   Loss%   Snt   Last   Avg  Best  Wrst StDev\n  1.|-- 192.0.2.1   0.0%    10    0.4   0.4   0.3   0.6   0.1\n  2.|-- ???        100.0    10    0.0   0.0   0.0   0.0   0.0\n  3.|-- 198.51.100.7  0.0%  10  12.1  12.3  11.9  13.0  0.3\n"
	if r, err = ParsePasted(mtr); err != nil || len(r.Hops) != 3 || r.Hops[0].IP != "192.0.2.1" || r.Hops[1].IP != "" {
		t.Fatalf("mtr 读得不对：%v %+v", err, r)
	}
	for _, bad := range []string{"", "hello", "198.51.100.7 is alive\n10.0.0.1 too", strings.Repeat("x", MaxPasted+1)} { // hygiene:allow
		if _, err := ParsePasted(bad); err == nil {
			t.Fatalf("%q 不应该认出路由", bad[:min(len(bad), 20)])
		}
	}
}

func clientDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// 浏览器的数字并到结果上；去程路由贴上去、清掉；数字不对的不收。
func TestClientStore(t *testing.T) {
	ctx := context.Background()
	s := New(clientDB(t))
	if _, err := s.db.Exec(`INSERT INTO speedtests (request_id, host, ts, ping_ms, down_bps, up_bps, size_mb, error, detail, method) VALUES ('r1','web-1',1,0,0,0,0,'','','client')`); err != nil {
		t.Fatal(err)
	}
	m := Client{Pings: []float64{30, 10, 20}, Lost: 1, Proto: "h2", Conn: "wifi", IP: "203.0.113.9", Proxy: true,
		Down: &Transfer{Bytes: 25e6, MS: 2000, Bps: 100e6, Conns: 4}, Up: &Transfer{Bytes: 6e6, MS: 2400, Bps: 20e6, Conns: 2}}
	if err := s.SaveClient(ctx, "r1", "198.51.100.7", false, m); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveForward(ctx, "r1", " 1  192.0.2.1  1.0 ms\n 2  198.51.100.9  9.0 ms\n"); err != nil {
		t.Fatal(err)
	}
	// 重新交一次数字：去程路由还在。
	if err := s.SaveClient(ctx, "r1", "198.51.100.7", false, m); err != nil {
		t.Fatal(err)
	}
	list, err := s.Recent(ctx, 10)
	if err != nil || len(list) != 1 || list[0].PingMS != 20 || list[0].DownBps != 100e6 || list[0].UpBps != 20e6 || list[0].SizeMB != 31 {
		t.Fatalf("列表里的数字不对：%v %+v", err, list)
	}
	r, err := s.Get(ctx, Key{RequestID: "r1", Host: "web-1"})
	if err != nil || r.Detail == nil || r.Detail.Client == nil {
		t.Fatalf("明细里没有浏览器的数字：%v %+v", err, r)
	}
	c := r.Detail.Client
	if c.IP != "198.51.100.7" || c.Proxy || c.Proto != "h2" || c.Conn != "wifi" || c.Lost != 1 || c.Forward != nil {
		t.Fatalf("地址和代理应该是服务端填的：%+v", c)
	}
	if len(r.Detail.Routes) != 1 || r.Detail.Routes[0].Dir != "forward" || len(r.Detail.Routes[0].Hops) != 2 {
		t.Fatalf("去程路由没有并进来：%+v", r.Detail.Routes)
	}
	if _, err := s.SaveForward(ctx, "r1", "  "); err != nil {
		t.Fatal(err)
	}
	if r, _ = s.Get(ctx, Key{RequestID: "r1", Host: "web-1"}); len(r.Detail.Routes) != 0 || r.Detail.Client == nil {
		t.Fatalf("去程路由应该清掉、数字留着：%+v", r.Detail)
	}
	for _, bad := range []Client{
		{Pings: make([]float64, 2*ClientPings+1)},
		{Pings: []float64{-1}},
		{Lost: -1},
		{Down: &Transfer{Bytes: -1}},
		{Up: &Transfer{Bps: 1e13}},
	} {
		if err := s.SaveClient(ctx, "r1", "198.51.100.7", false, bad); err == nil {
			t.Fatalf("不对的数字不应该收下：%+v", bad)
		}
	}
	// 说不清的协议和连接类型不留。
	s.SaveClient(ctx, "r1", "198.51.100.7", false, Client{Proto: "<script>", Conn: "WIFI 6"})
	if c, _ := s.client(ctx, "r1"); c.Proto != "" || c.Conn != "" {
		t.Fatalf("不合法的协议和连接类型应该丢掉：%+v", c)
	}
	// 结果删掉后，过了一天的浏览器数据跟着删。
	s.db.Exec(`UPDATE speedtest_clients SET ts = 1`)
	if _, err := s.Delete(ctx, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.client(ctx, "r1"); ok {
		t.Fatal("测速结果删掉后，浏览器的数据应该跟着删")
	}
}
