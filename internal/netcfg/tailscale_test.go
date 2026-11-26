package netcfg

import (
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

func tsStatus(state string) Status {
	st := Status{Root: true, Family: "debian"}
	st.Tailscale = Tailscale{Installed: true, Active: true, State: state}
	return st
}

// 检测：从 tailscale status --json 和 debug prefs 里读出状态、地址和设置；远端给的值只留合法的。
func TestTailscaleDetect(t *testing.T) {
	out := "== os\nID=debian\n== uid\n0\n== has\ntailscale\napt-get\n== ts\nactive\n== ts-status\n" +
		`{"Version":"1.80.2-t1a2b3c","BackendState":"Running","TailscaleIPs":["100.64.0.7","fd7a:115c:a1e0::7","not-an-ip"],` + "\n" + // hygiene:allow（Tailscale 固定用的网段，不是真实主机）
		`"Self":{"HostName":"web-1","DNSName":"web-1.example-tailnet.ts.net."},"CurrentTailnet":{"Name":"example-tailnet.ts.net"}}` + "\n" +
		"== ts-prefs\n" + `{"RouteAll":true,"CorpDNS":false,"RunSSH":true,"Hostname":"web-1"}` + "\n== client\n100.64.0.9\n== end\n" // hygiene:allow（Tailscale 固定用的网段，不是真实主机）
	ts := Parse(out, 0).Tailscale
	if !ts.Installed || !ts.Active || !ts.Installable || ts.State != "Running" || len(ts.IPs) != 2 || ts.IPs[0] != "100.64.0.7" || // hygiene:allow（Tailscale 固定用的网段，不是真实主机）
		ts.Name != "web-1" || ts.DNSName != "web-1.example-tailnet.ts.net" || ts.Tailnet != "example-tailnet.ts.net" || ts.Version != "1.80.2" {
		t.Fatalf("状态读得不对：%+v", ts)
	}
	if ts.Options == nil || !ts.Options.AcceptRoutes || ts.Options.AcceptDNS || !ts.Options.SSH || ts.Options.Hostname != "web-1" {
		t.Fatalf("设置读得不对：%+v", ts.Options)
	}
	if !ts.ViaTailscale {
		t.Fatal("来源地址在 Tailscale 网段里，应该认出是经 Tailscale 连过来的")
	}
	// 没装、读不出来：都是空的，不报错。
	ts = Parse("== os\nID=alpine\n== uid\n1000\n== has\n== ts\n== ts-status\n{broken\n== ts-prefs\n== client\n198.51.100.10\n== end\n", 0).Tailscale
	if ts.Installed || ts.Active || ts.Installable || ts.State != "" || len(ts.IPs) != 0 || ts.Options != nil || ts.ViaTailscale {
		t.Fatalf("没装时不应该有状态：%+v", ts)
	}
}

// 安装、加入、改设置、断开、退出登录都是 L2；授权密钥不出现在命令里。
func TestTailscaleCommands(t *testing.T) {
	e := risk.New()
	l2 := func(what, cmd string, err error, want ...string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s：%v", what, err)
		}
		for _, w := range want {
			if !strings.Contains(cmd, w) {
				t.Fatalf("%s：命令里没有 %q\n%s", what, w, cmd)
			}
		}
		if got := e.Classify(cmd); got.Level != risk.L2 {
			t.Fatalf("%s：应该是 L2，现在是 %s\n%s", what, got.Level, cmd)
		}
	}
	fresh := Status{Root: true, Family: "debian"}
	cmd, err := TSCommand(fresh, TSRequest{Action: "install"}, false)
	l2("Debian 上安装", cmd, err, "https://pkgs.tailscale.com/stable/$os/$ver.noarmor.gpg", "/usr/share/keyrings/tailscale-archive-keyring.gpg",
		"/etc/apt/sources.list.d/tailscale.list", "apt-get install -y -q tailscale", "systemctl enable --now tailscaled")
	fresh.Family = "rhel"
	cmd, err = TSCommand(fresh, TSRequest{Action: "install"}, false)
	l2("RHEL 系上安装", cmd, err, "https://pkgs.tailscale.com/stable/$repo/tailscale.repo", "/etc/yum.repos.d/tailscale.repo", "dnf install -y -q tailscale")
	fresh.Family = "other"
	if _, err = TSCommand(fresh, TSRequest{Action: "install"}, false); err == nil {
		t.Fatal("不认识的系统不应该给安装命令")
	}
	if _, err = TSCommand(tsStatus("NeedsLogin"), TSRequest{Action: "install"}, false); err == nil {
		t.Fatal("已经装了不应该再装")
	}

	o := TSOptions{Hostname: "web-1", AcceptRoutes: true, AcceptDNS: true}
	cmd, err = TSCommand(tsStatus("NeedsLogin"), TSRequest{Action: "up", Options: o}, true)
	l2("用授权密钥加入", cmd, err, "tailscale up --reset --auth-key=file:/dev/stdin --hostname=web-1 --accept-routes=true --accept-dns=true --ssh=false --timeout=60s")
	if strings.Contains(cmd, "tskey") {
		t.Fatalf("命令里不应该有密钥：%s", cmd)
	}
	cmd, err = TSCommand(tsStatus("NeedsLogin"), TSRequest{Action: "up", Options: TSOptions{AcceptDNS: true}}, false)
	l2("打开登录链接加入", cmd, err, "tailscale up --reset --hostname= --accept-routes=false --accept-dns=true --ssh=false --timeout=540s")
	if strings.Contains(cmd, "auth-key") {
		t.Fatalf("没选密钥时不应该带 --auth-key：%s", cmd)
	}
	if _, err = TSCommand(tsStatus("Running"), TSRequest{Action: "up"}, false); err == nil {
		t.Fatal("已经在网络里不应该再加入")
	}
	for _, bad := range []string{"a b", "-a", "a;b", "$(id)", "a.b", strings.Repeat("a", 64)} {
		if _, err = TSCommand(tsStatus("NeedsLogin"), TSRequest{Action: "up", Options: TSOptions{Hostname: bad}}, false); err == nil {
			t.Fatalf("名字 %q 不应该被接受", bad)
		}
	}

	cmd, err = TSCommand(tsStatus("Running"), TSRequest{Action: "set", Options: TSOptions{SSH: true}}, false)
	l2("改设置", cmd, err, "tailscale set --hostname= --accept-routes=false --accept-dns=false --ssh=true")
	if _, err = TSCommand(tsStatus("NeedsLogin"), TSRequest{Action: "set"}, false); err == nil {
		t.Fatal("没登录不应该能改设置")
	}
	cmd, err = TSCommand(tsStatus("Running"), TSRequest{Action: "down"}, false)
	l2("断开", cmd, err, "tailscale down")
	cmd, err = TSCommand(tsStatus("Stopped"), TSRequest{Action: "logout"}, false)
	l2("退出登录", cmd, err, "tailscale logout")

	// Hostower 自己就是经 Tailscale 连过来的：不给断开和退出登录。
	via := tsStatus("Running")
	via.Tailscale.ViaTailscale = true
	for _, a := range []string{"down", "logout"} {
		if _, err = TSCommand(via, TSRequest{Action: a}, false); err == nil {
			t.Fatalf("经 Tailscale 连过来时不应该能 %s", a)
		}
	}
	// 提不了权：什么都做不了。
	if _, err = TSCommand(Status{Family: "debian"}, TSRequest{Action: "install"}, false); err == nil {
		t.Fatal("没有 root 也没有 sudo 时不应该给命令")
	}
}
