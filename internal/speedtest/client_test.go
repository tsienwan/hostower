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

func clientDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// 浏览器的数字并到结果上；数字不对的不收。
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
	list, err := s.Recent(ctx, 10)
	if err != nil || len(list) != 1 || list[0].PingMS != 20 || list[0].DownBps != 100e6 || list[0].UpBps != 20e6 || list[0].SizeMB != 31 {
		t.Fatalf("列表里的数字不对：%v %+v", err, list)
	}
	r, err := s.Get(ctx, Key{RequestID: "r1", Host: "web-1"})
	if err != nil || r.Detail == nil || r.Detail.Client == nil {
		t.Fatalf("明细里没有浏览器的数字：%v %+v", err, r)
	}
	c := r.Detail.Client
	if c.IP != "198.51.100.7" || c.Proxy || c.Proto != "h2" || c.Conn != "wifi" || c.Lost != 1 {
		t.Fatalf("地址和代理应该是服务端填的：%+v", c)
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
