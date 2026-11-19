package netcfg

import (
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

const ubuntuOut = `== os
PRETTY_NAME="Ubuntu 24.04.1 LTS"
ID=ubuntu
ID_LIKE=debian
== uid
1001
== has
ufw
nft
iptables
caddy
docker
apt-get
sudo
== ufw
Status: active

To                         Action      From
--                         ------      ----
22/tcp                     ALLOW       Anywhere
443                        ALLOW       Anywhere
22/tcp (v6)                ALLOW       Anywhere (v6)
== firewalld
inactive
== firewalld-ports
== firewalld-services
== caddy
active
== caddy-import
import /etc/caddy/sites.d/*.caddy
== sites
# hostower-site kuma.example.com 127.0.0.1:3001
== sshd
Port 2222
== listen
LISTEN 0      4096   0.0.0.0:2222      0.0.0.0:*
LISTEN 0      4096   127.0.0.1:9443    0.0.0.0:*
LISTEN 0      4096   [::]:443          [::]:*
LISTEN 0      4096   *:8443            *:*
== docker
web	0.0.0.0:8080->80/tcp, [::]:8080->80/tcp
db	127.0.0.1:5432->5432/tcp
== end
`

func TestParse(t *testing.T) {
	st := Parse(ubuntuOut, 0)
	if st.Family != "debian" || st.OS != "Ubuntu 24.04.1 LTS" || st.Root || !st.Sudo {
		t.Errorf("系统识别不对：%+v", st)
	}
	if st.Firewall.Tool != "ufw" || st.Firewall.Recommended != "ufw" || len(st.Firewall.Rules) != 2 {
		t.Errorf("防火墙识别不对：%+v", st.Firewall)
	}
	if !st.Caddy.Installed || !st.Caddy.Active || !st.Caddy.Imported || len(st.Sites) != 1 || st.Sites[0].Target != "127.0.0.1:3001" || st.Sites[0].TLS != "auto" || st.Sites[0].Kind != "proxy" {
		t.Errorf("Caddy 识别不对：%+v %+v", st.Caddy, st.Sites)
	}
	if st.SSHPort != 2222 {
		t.Errorf("SSH 端口应从 sshd_config 读出 2222，得到 %d", st.SSHPort)
	}
	if len(st.Listen) != 3 || st.Listen[0].Port != 443 {
		t.Errorf("对外监听的端口不对（回环地址要排除）：%+v", st.Listen)
	}
	if len(st.Docker) != 1 || st.Docker[0].Port != 8080 || st.Docker[0].Container != "web" {
		t.Errorf("Docker 直接发布到所有地址的端口不对：%+v", st.Docker)
	}
	nnp := Parse(strings.Replace(ubuntuOut, "== uid\n1001", "== uid\n1001\n== nnp\nNoNewPrivs:\t1", 1), 22)
	if !nnp.NoNewPrivs || nnp.CanRoot() {
		t.Error("禁止提权时 sudo 用不了，应识别出来")
	}
	if _, _, err := FirewallCommand(nnp, FirewallRequest{Action: "allow", Ports: []Port{{80, "tcp"}}}, nil); err == nil || !strings.Contains(err.Error(), "NoNewPrivileges") {
		t.Errorf("禁止提权时应直接说明原因：%v", err)
	}
	if st2 := Parse(ubuntuOut, 22); st2.SSHPort != 22 {
		t.Error("知道连接用的 SSH 端口时应以它为准")
	}
	both := strings.Replace(ubuntuOut, "== firewalld\ninactive", "== firewalld\nactive", 1)
	if Parse(both, 0).Firewall.Tool != "conflict" {
		t.Error("两个防火墙同时启用应识别为冲突")
	}
}

func TestLevels(t *testing.T) {
	e := risk.New()
	if got := e.Classify(DetectCommand); got.Level != risk.L0 {
		t.Fatalf("检测脚本必须是 L0，得到 %s：%+v", got.Level, got.Findings)
	}
	fresh := Parse(strings.NewReplacer("ufw\nnft", "nft", "Status: active", "").Replace(ubuntuOut), 22)
	fresh.Firewall.UFWInstalled, fresh.Firewall.UFWActive, fresh.Firewall.Tool = false, false, ""
	keep := []Port{{22, "tcp"}}
	cmd, tool, err := FirewallCommand(fresh, FirewallRequest{Action: "setup", Ports: []Port{{443, "tcp"}}}, keep)
	if err != nil || tool != "ufw" {
		t.Fatal(err, tool)
	}
	// 顺序：先装、先放行 SSH，最后才启用。
	iInstall, iSSH, iEnable := strings.Index(cmd, "apt-get install -y -q ufw"), strings.Index(cmd, "ufw allow 22/tcp"), strings.Index(cmd, "ufw --force enable")
	if iInstall < 0 || iSSH < iInstall || iEnable < iSSH || !strings.Contains(cmd, "ufw allow 443/tcp") {
		t.Errorf("安装并启用的顺序不对：\n%s", cmd)
	}
	if got := e.Classify(cmd).Level; got != risk.L2 {
		t.Errorf("启用防火墙应是 L2，得到 %s", got)
	}
	st := Parse(ubuntuOut, 22)
	cmd, _, err = FirewallCommand(st, FirewallRequest{Action: "allow", Ports: []Port{{3001, "tcp"}}}, keep)
	if err != nil || e.Classify(cmd).Level != risk.L2 {
		t.Errorf("放行端口应是 L2：%v", err)
	}
	if _, _, err := FirewallCommand(st, FirewallRequest{Action: "remove", Ports: []Port{{22, "tcp"}}}, keep); err == nil {
		t.Error("关掉 SSH 端口应被拒绝")
	}
	if _, _, err := FirewallCommand(st, FirewallRequest{Action: "allow", Ports: []Port{{70000, "tcp"}}}, keep); err == nil {
		t.Error("越界端口应被拒绝")
	}
	if _, _, err := FirewallCommand(st, FirewallRequest{Action: "allow", Ports: []Port{{80, "sctp"}}}, keep); err == nil {
		t.Error("不认识的协议应被拒绝")
	}
	conflict := st
	conflict.Firewall.Tool = "conflict"
	if _, _, err := FirewallCommand(conflict, FirewallRequest{Action: "allow", Ports: []Port{{80, "tcp"}}}, keep); err == nil {
		t.Error("两个防火墙同时启用时应拒绝")
	}
	// firewalld：用离线命令先放行，再启用。
	rh := Parse(strings.NewReplacer("ID=ubuntu", "ID=rocky", "ID_LIKE=debian", `ID_LIKE="rhel centos fedora"`, "apt-get", "dnf").Replace(ubuntuOut), 22)
	rh.Firewall.UFWInstalled, rh.Firewall.UFWActive, rh.Firewall.Tool = false, false, ""
	cmd, tool, err = FirewallCommand(rh, FirewallRequest{Action: "setup"}, keep)
	if err != nil || tool != "firewalld" || !strings.Contains(cmd, "dnf install -y -q firewalld") ||
		strings.Index(cmd, "firewall-offline-cmd --add-port=22/tcp") > strings.Index(cmd, "systemctl enable --now firewalld") {
		t.Errorf("firewalld 的安装启用不对：%v\n%s", err, cmd)
	}
	if got := e.Classify(cmd).Level; got != risk.L2 {
		t.Errorf("启用 firewalld 应是 L2，得到 %s", got)
	}
}

func TestSites(t *testing.T) {
	e := risk.New()
	st := Parse(ubuntuOut, 22)
	cmd, _, err := SiteAddCommand(st, SiteRequest{Address: "App.Example.com", Port: 3001})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"/etc/caddy/sites.d/app.example.com.caddy", "reverse_proxy 127.0.0.1:3001", "caddy validate", "systemctl reload caddy", ".rejected", "# hostower-site app.example.com 127.0.0.1:3001 kind=proxy tls=auto"} {
		if !strings.Contains(cmd, s) {
			t.Errorf("添加站点的命令里缺少 %q：\n%s", s, cmd)
		}
	}
	if got := e.Classify(cmd).Level; got != risk.L1 {
		t.Errorf("添加站点应是 L1，得到 %s：%+v", got, e.Classify(cmd).Findings)
	}
	cmd, _, err = SiteAddCommand(st, SiteRequest{Address: "198.51.100.7:8443", Port: 9443})
	if err != nil || !strings.Contains(cmd, "tls internal") || !strings.Contains(cmd, "198.51.100.7_8443.caddy") {
		t.Errorf("IP 地址的站点应用自签证书：%v\n%s", err, cmd)
	}
	for _, bad := range []string{"a.example.com { }", "example.com; reboot", "../etc/passwd", "198.51.100.7", "-x.example.com", "example.com:99999", "*.example.com"} {
		if _, _, err := SiteAddCommand(st, SiteRequest{Address: bad, Port: 3001}); err == nil {
			t.Errorf("不合法的地址 %q 应被拒绝", bad)
		}
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "kuma.example.com", Port: 3001}); err == nil {
		t.Error("地址重复应被拒绝")
	}
	rm, err := SiteRemoveCommand(st, "kuma.example.com")
	if err != nil || !strings.Contains(rm, ".removed") || e.Classify(rm).Level != risk.L1 {
		t.Errorf("撤下站点不对：%v\n%s", err, rm)
	}
	if _, err := SiteRemoveCommand(st, "other.example.com"); err == nil {
		t.Error("不是 Hostower 加的站点不能删")
	}
	noCaddy := st
	noCaddy.Caddy.Installed = false
	if _, _, err := SiteAddCommand(noCaddy, SiteRequest{Address: "app.example.com", Port: 3001}); err == nil {
		t.Error("没有 Caddy 时应提示先安装")
	}
	inst, err := CaddyInstallCommand(noCaddy)
	if err != nil || !strings.Contains(inst, "apt-get install -y -q caddy") || e.Classify(inst).Level > risk.L2 {
		t.Errorf("安装 Caddy 不对：%v", err)
	}
}
