package speedtest

import (
	"strings"
	"testing"
)

func TestOoklaCommand(t *testing.T) {
	routes := []string{Server, "198.51.100.7"}
	if _, err := OoklaCommand(-1, false, nil); err == nil {
		t.Fatal("负的服务器编号应该被拒绝")
	}
	if _, err := OoklaCommand(1, false, []string{"x'; rm -rf / #"}); err == nil {
		t.Fatal("带引号的目标应该被拒绝")
	}
	auto, _ := OoklaCommand(0, false, nil)
	if strings.Contains(auto, " -s ") || strings.Contains(auto, "traceroute") || !strings.Contains(auto, "--accept-license --accept-gdpr") {
		t.Fatal("自动选服务器时不该带 -s，没有目标时不该追踪")
	}
	for _, b := range ooklaBuilds {
		if len(b.tgz) != 64 || len(b.bin) != 64 || !strings.Contains(auto, b.tgz) || !strings.Contains(auto, b.bin) {
			t.Fatalf("%s 的校验值不对", b.arch)
		}
	}
	for _, install := range []bool{false, true} {
		c, err := OoklaCommand(3633, install, routes)
		if err != nil {
			t.Fatal(err)
		}
		sp, ok := SpecOf(c)
		if !ok || sp.Method != MethodOokla || sp.Server != 3633 || len(sp.Routes) != 2 {
			t.Fatalf("命令里读回的参数不对：%+v %v", sp, ok)
		}
		if install != strings.Contains(c, "speedtest installing curl") {
			t.Fatal("安装 curl 那段不对")
		}
		if _, ok := SpecOf(strings.Replace(c, OoklaVersion, "9.9.9", 1)); ok {
			t.Fatal("改过的命令不应该认得出来")
		}
	}
	if !strings.Contains(auto, `case "$v" in "Speedtest by Ookla "*)`) || !strings.Contains(auto, `if [ "$u" = pinned ]; then`) {
		t.Fatal("应该先认系统里的官方 speedtest，没有才用锁定版本")
	}
	if sp, ok := SpecOf(auto); !ok || sp.Server != 0 {
		t.Fatal("自动选服务器的命令也应该认得出来")
	}
}

func TestOoklaParse(t *testing.T) {
	out := "speedtest route 198.51.100.7  1  192.0.2.1  0.5 ms\n" +
		`speedtest ookla {"type":"result","timestamp":"2026-01-01T00:00:00Z","ping":{"jitter":0.42,"latency":12.345,"low":11.9,"high":13.2},` +
		`"download":{"bandwidth":12500000,"bytes":150000000,"elapsed":12000,"latency":{"iqm":40.5,"low":12,"high":200,"jitter":3}},` +
		`"upload":{"bandwidth":6250000,"bytes":50000000,"elapsed":8000,"latency":{"iqm":30.1}},"packetLoss":0.5,"isp":"Example ISP",` +
		`"interface":{"internalIp":"192.0.2.2","externalIp":"203.0.113.9"},` +
		`"server":{"id":3633,"host":"speed.example.com","port":8080,"name":"Example Telecom","location":"Shanghai","country":"China","ip":"198.51.100.20"},` +
		`"result":{"id":"0f0f","url":"https://www.speedtest.net/result/c/0f0f0f0f-1111-2222-3333-444444444444","persisted":true}}` + "\n"
	rep := Parse(out)
	o := rep.Detail.Ookla
	if o == nil || o.Binary != "" {
		t.Fatalf("没说用的哪个程序时应该留空：%+v", o)
	}
	sys := Parse("speedtest version Speedtest by Ookla 1.2.0.84 (5ed00a8b2d) Linux/x86_64-linux-musl 5.15.0 x86_64\nspeedtest using=system\n" + out).Detail.Ookla // 版本号，不是地址 hygiene:allow
	pin := Parse("speedtest installing ookla\nspeedtest using=pinned\n" + out).Detail.Ookla
	if sys.Binary != "system" || sys.Version != "1.2.0.84" || pin.Binary != "pinned" || pin.Version != OoklaVersion { // hygiene:allow
		t.Fatalf("程序来源不对：%+v %+v", sys, pin)
	}
	if rep.Error != "" || o == nil || rep.PingMS != 12.35 || rep.DownBps != 100e6 || rep.UpBps != 50e6 {
		t.Fatalf("解析不对：%+v", rep)
	}
	if o.Server.ID != 3633 || o.Server.Label() != "Example Telecom · Shanghai" || *o.PacketLoss != 0.5 || o.Jitter != 0.42 || o.DownLatency != 40.5 ||
		o.URL == "" || rep.Detail.Net.IP != "203.0.113.9" || rep.Detail.Down.Bytes != 150e6 || len(rep.Detail.Routes) != 1 {
		t.Fatalf("明细不对：%+v %+v", o, rep.Detail)
	}
	// 结果页地址不是 speedtest.net 的就不要。
	bad := strings.Replace(out, "https://www.speedtest.net/result/c/", "https://example.com/r/", 1)
	if Parse(bad).Detail.Ookla.URL != "" {
		t.Fatal("不认识的结果页地址应该丢掉")
	}
	e := Parse(`speedtest ookla {"type":"log","level":"error","message":"Configuration - Could not retrieve or read configuration (ConfigurationError)"}` + "\n")
	if !strings.HasPrefix(e.Error, "Speedtest 出错：") {
		t.Fatalf("应该报出 Ookla 的错误：%q", e.Error)
	}
	for _, code := range []string{"unsupported-arch", "no-sha256sum", "download-failed", "bad-checksum"} {
		if Parse("speedtest installing ookla\nspeedtest error="+code+"\n").Error == "" {
			t.Fatal(code + " 应该报错")
		}
	}
}

func TestOptions(t *testing.T) {
	if _, err := (Options{Threads: "many"}).Normalize(MethodOokla); err == nil {
		t.Fatal("不认识的线程选项应该被拒绝")
	}
	if _, err := (Options{IP: "5"}).Normalize(MethodCloudflare); err == nil {
		t.Fatal("不认识的 IP 选项应该被拒绝")
	}
	routes := []string{"198.51.100.7"}
	// 默认选项生成的命令和之前逐字相同。
	plain, _ := Command(25, routes)
	if c, _ := CloudflareCommand(25, false, routes, Options{Threads: "single"}); c != plain {
		t.Fatal("Cloudflare 单线程、自动应该和原来的命令一样")
	}
	auto, _ := OoklaCommand(0, false, nil)
	if c, _ := OoklaCommandWith(0, false, nil, Options{Threads: "multi"}); c != auto {
		t.Fatal("Ookla 多线程、自动应该和原来的命令一样")
	}
	for _, o := range allOptions {
		for _, method := range []string{MethodOokla, MethodCloudflare} {
			var c string
			var err error
			if method == MethodOokla {
				c, err = OoklaCommandWith(3633, true, routes, o)
			} else {
				c, err = CloudflareCommand(25, true, routes, o)
			}
			if err != nil {
				t.Fatalf("%s %+v：%v", method, o, err)
			}
			sp, ok := SpecOf(c)
			if !ok || sp.Method != method || sp.Opts != o {
				t.Fatalf("%s %+v 读回来不对：%+v %v", method, o, sp, ok)
			}
		}
	}
	multi, _ := CloudflareCommand(25, false, routes, Options{Threads: "multi", IP: "6"})
	if strings.Count(multi, "& done; wait") != 2 || !strings.Contains(multi, "bytes=6250000'") || !strings.Contains(multi, "curl -6 -s") {
		t.Fatal("多线程应该四条连接平分数据量，并且每次 curl 都带 -6")
	}
	single6, _ := OoklaCommandWith(0, false, nil, Options{Threads: "single", IP: "6"})
	if !strings.Contains(single6, "--download-conn-range=1-1 --upload-conn-range=1-1 -i \"$src\"") || !strings.Contains(single6, "ip -6 route get 2001:db8::1") {
		t.Fatal("Ookla 单线程、IPv6 的参数不对")
	}
	// 四条连接的结果合起来算：字节相加、用时取最长。
	out := ""
	for i := 0; i < 4; i++ {
		out += "speedtest down speed=1500000 bytes=6250000 time=" + []string{"4.0", "4.2", "5.0", "4.5"}[i] + " ttfb=0.2\n"
		out += "speedtest up speed=500000 bytes=1562500 time=3.125\n"
	}
	rep := Parse(out)
	if rep.Error != "" || rep.Detail.Down.Conns != 4 || rep.Detail.Down.Bytes != 25e6 || rep.DownBps != 40e6 || rep.UpBps != 16e6 {
		t.Fatalf("多条连接合起来不对：%+v %+v", rep, rep.Detail.Down)
	}
	if Parse("speedtest using=pinned\nspeedtest error=no-ipv6\n").Error == "" {
		t.Fatal("没有 IPv6 时应该说明")
	}
}
