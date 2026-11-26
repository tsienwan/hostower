package netcfg

import (
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// debianOut：装了 Nginx、certbot、Fail2ban 的 Debian 12，没有 auth.log。
const debianOut = `== os
PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
ID=debian
== uid
1000
== has
nft
nginx
certbot
fail2ban-client
apt-get
sudo
== ufw
== firewalld
== firewalld-ports
== firewalld-services
== caddy
== caddy-import
== sites
== nginx
active
== nginx-sites
# hostower-site app.example.com 127.0.0.1:3001 kind=proxy tls=le body=100 hsts=1
# hostower-site files.example.com /srv/files kind=static tls=http
== certs
Saving debug log to /var/log/letsencrypt/letsencrypt.log

- - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
Found the following certs:
  Certificate Name: app.example.com
    Serial Number: 4a1f
    Key Type: ECDSA
    Domains: app.example.com
    Expiry Date: 2027-01-20 08:00:00+00:00 (VALID: 62 days)
    Certificate Path: /etc/letsencrypt/live/app.example.com/fullchain.pem
    Private Key Path: /etc/letsencrypt/live/app.example.com/privkey.pem
  Certificate Name: old.example.com
    Domains: old.example.com www.old.example.com
    Expiry Date: 2026-11-01 08:00:00+00:00 (INVALID: EXPIRED)
== renew
active
== f2b
active
== f2b-jails
-- sshd
Status for the jail: sshd
|- Filter
|  |- Currently failed:	2
|  |- Total failed:	31
|  ` + "`" + `- Journal matches:	_SYSTEMD_UNIT=sshd.service + _COMM=sshd
` + "`" + `- Actions
   |- Currently banned:	2
   |- Total banned:	9
   ` + "`" + `- Banned IP list:	203.0.113.45 198.51.100.23
== f2b-conf
# hostower-f2b bantime=3600 findtime=600 maxretry=5 ignoreip=192.0.2.0/24 jails=sshd
== authlog
== client
203.0.113.9
== sshd
== listen
LISTEN 0 511 0.0.0.0:80 0.0.0.0:*
== docker
== end
`

func TestParseWeb(t *testing.T) {
	st := Parse(debianOut, 22)
	if !st.Nginx.Installed || !st.Nginx.Active || st.Caddy.Installed || !st.Certbot.Installed || !st.Certbot.AutoRenew {
		t.Errorf("Nginx / certbot 识别不对：%+v %+v", st.Nginx, st.Certbot)
	}
	if len(st.Sites) != 2 || st.Sites[0].Server != "nginx" || st.Sites[0].TLS != "le" || st.Sites[0].MaxBody != 100 || !st.Sites[0].HSTS ||
		st.Sites[1].Kind != "static" || st.Sites[1].Target != "/srv/files" {
		t.Errorf("Nginx 站点识别不对：%+v", st.Sites)
	}
	if len(st.Certs) != 2 || st.Certs[0].Days != 62 || !st.Certs[0].Valid || st.Certs[1].Valid || len(st.Certs[1].Domains) != 2 {
		t.Errorf("证书识别不对：%+v", st.Certs)
	}
	f := st.Fail2ban
	if !f.Installed || !f.Active || f.Unknown || f.AuthLog || len(f.Jails) != 1 || f.Jails[0].Banned != 2 || len(f.Jails[0].IPs) != 2 || f.Jails[0].TotalFailed != 31 {
		t.Errorf("Fail2ban 识别不对：%+v", f)
	}
	if f.Config == nil || f.Config.MaxRetry != 5 || len(f.Config.Ignore) != 1 || f.Config.Jails[0] != "sshd" {
		t.Errorf("Fail2ban 设置识别不对：%+v", f.Config)
	}
	if st.ClientIP != "203.0.113.9" {
		t.Errorf("来源地址不对：%q", st.ClientIP)
	}
}

func TestNginxSites(t *testing.T) {
	e := risk.New()
	st := Parse(debianOut, 22)
	// Let's Encrypt：先写 http、校验、重载，再申请证书，最后换成 HTTPS。
	cmd, site, err := SiteAddCommand(st, SiteRequest{Address: "new.example.com", Port: 8080, TLS: "le", Agree: true, Email: "ops@example.com", MaxBody: 50, HSTS: true})
	if err != nil || site.Server != "nginx" {
		t.Fatal(err, site)
	}
	order := []string{"tls=http", "nginx -t", "systemctl reload-or-restart nginx", "certbot certonly --webroot -w /var/www/hostower-acme -d new.example.com --cert-name new.example.com",
		"-m ops@example.com", "--deploy-hook 'systemctl reload nginx'", "tls=le", "return 301 https://$host$request_uri;", "listen 443 ssl http2;",
		"client_max_body_size 50m;", "ssl_certificate_key /etc/letsencrypt/live/new.example.com/privkey.pem;", "Strict-Transport-Security", "nginx -t", "systemctl reload-or-restart nginx"}
	at := 0
	for _, s := range order {
		i := strings.Index(cmd[at:], s)
		if i < 0 {
			t.Fatalf("Let's Encrypt 站点的命令里 %q 缺失或顺序不对：\n%s", s, cmd)
		}
		at += i
	}
	if got := e.Classify(cmd); got.Level != risk.L1 {
		t.Errorf("添加 Nginx 站点应是 L1，得到 %s：%+v", got.Level, got.Findings)
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "new.example.com", Port: 8080, TLS: "le"}); err == nil {
		t.Error("没同意 Let's Encrypt 订户协议应被拒绝")
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "new.example.com:8443", Port: 8080, TLS: "le", Agree: true}); err == nil {
		t.Error("带端口的地址申请不了 Let's Encrypt 证书")
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "new.example.com", Port: 8080, TLS: "auto"}); err == nil {
		t.Error("Nginx 没有 auto 这种方式")
	}
	// 已有证书。
	cmd, _, err = SiteAddCommand(st, SiteRequest{Address: "x.example.com", Kind: "redirect", URL: "https://example.com/new", Perm: true, TLS: "custom",
		Cert: "/etc/ssl/certs/x.pem", Key: "/etc/ssl/private/x.key", KeepHTTP: true})
	if err != nil || !strings.Contains(cmd, "return 301 https://example.com/new;") || !strings.Contains(cmd, "ssl_certificate_key /etc/ssl/private/x.key;") ||
		strings.Contains(cmd, "return 301 https://$host") || e.Classify(cmd).Level != risk.L1 {
		t.Errorf("已有证书的跳转站点不对：%v\n%s", err, cmd)
	}
	for _, bad := range []SiteRequest{
		{Address: "y.example.com", TLS: "custom", Port: 1, Cert: "/root/.ssh/id_rsa", Key: "/etc/ssl/private/x.key"},
		{Address: "y.example.com", TLS: "custom", Port: 1, Cert: "/etc/ssl/../shadow.pem", Key: "/etc/ssl/private/x.key"},
		{Address: "y.example.com", Kind: "static", Dir: "/etc"},
		{Address: "y.example.com", Kind: "static", Dir: "/home/ops/.ssh"},
		{Address: "y.example.com", Kind: "static", Dir: "/"},
		{Address: "y.example.com", Kind: "redirect", URL: "https://example.com/a;b"},
		{Address: "y.example.com", Kind: "redirect", URL: "javascript:alert(1)"},
		{Address: "y.example.com", Kind: "redirect", URL: "https://example.com/$host"},
		{Address: "y.example.com", Kind: "other", Port: 1},
		{Address: "y.example.com", Port: 1, MaxBody: -1},
		{Address: "y.example.com", Port: 1, Server: "apache"},
		{Address: "y.example.com", Port: 1, TLS: "le", Agree: true, Email: "a b@example.com"},
	} {
		if _, _, err := SiteAddCommand(st, bad); err == nil {
			t.Errorf("不合法的站点应被拒绝：%+v", bad)
		}
	}
	cmd, _, err = SiteAddCommand(st, SiteRequest{Address: "www.example.com", Kind: "static", Dir: "/var/www/site/"})
	if err != nil || !strings.Contains(cmd, "root /var/www/site;") || !strings.Contains(cmd, "listen 80;") || e.Classify(cmd).Level != risk.L1 {
		t.Errorf("静态站点不对：%v\n%s", err, cmd)
	}
	rm, err := SiteRemoveCommand(st, "files.example.com")
	if err != nil || !strings.Contains(rm, "/etc/nginx/conf.d/hostower-files.example.com.conf.removed") || !strings.Contains(rm, "systemctl reload-or-restart nginx") || e.Classify(rm).Level != risk.L1 {
		t.Errorf("撤下 Nginx 站点不对：%v\n%s", err, rm)
	}
	// 日志是只读的。
	for _, kind := range []string{"access", "error"} {
		c, err := SiteLogCommand("nginx", "app.example.com", kind)
		if err != nil || e.Classify(c).Level != risk.L0 {
			t.Errorf("看日志应是 L0：%v %s", err, c)
		}
	}
	if _, err := SiteLogCommand("nginx", "app.example.com", "../../etc/shadow"); err == nil {
		t.Error("不认识的日志种类应被拒绝")
	}
	for _, bad := range [][2]string{{"nginx", "../etc/shadow"}, {"nginx", "a.example.com/../../x"}, {"apache", "app.example.com"}} {
		if _, err := SiteLogCommand(bad[0], bad[1], "access"); err == nil {
			t.Errorf("应被拒绝：%v", bad)
		}
	}
	if e.Classify(F2BLogCommand).Level != risk.L0 {
		t.Error("看 Fail2ban 日志应是 L0")
	}
	// 一台主机只管一个反向代理。
	if _, err := CaddyInstallCommand(st); err == nil {
		t.Error("装了 Nginx 时不应再装 Caddy")
	}
	bare := Parse(strings.NewReplacer("nginx\n", "", "certbot\n", "").Replace(debianOut), 22)
	inst, err := NginxInstallCommand(bare)
	if err != nil || !strings.Contains(inst, "apt-get install -y -q nginx") || !strings.Contains(inst, "systemctl enable --now nginx") || e.Classify(inst).Level != risk.L1 {
		t.Errorf("安装 Nginx 不对：%v\n%s", err, inst)
	}
	rh := Parse(strings.NewReplacer("ID=debian", `ID=rocky`+"\n"+`ID_LIKE="rhel centos fedora"`, "apt-get", "dnf", "nginx\n", "").Replace(debianOut), 22)
	inst, err = NginxInstallCommand(rh)
	if err != nil || !strings.Contains(inst, "dnf install -y -q nginx") || !strings.Contains(inst, "setsebool -P httpd_can_network_connect 1") {
		t.Errorf("RHEL 上安装 Nginx 不对：%v\n%s", err, inst)
	}
	// 没装 certbot 时申请证书顺带装上。
	cmd, _, err = SiteAddCommand(bare2(t), SiteRequest{Address: "new.example.com", Port: 8080, TLS: "le", Agree: true})
	if err != nil || !strings.Contains(cmd, "apt-get install -y -q certbot") || !strings.Contains(cmd, "--register-unsafely-without-email") {
		t.Errorf("没装 certbot 时应先装：%v\n%s", err, cmd)
	}
}

func bare2(t *testing.T) Status {
	t.Helper()
	return Parse(strings.Replace(debianOut, "certbot\n", "", 1), 22)
}

func TestCaddyOptions(t *testing.T) {
	e := risk.New()
	st := Parse(ubuntuOut, 22)
	cmd, _, err := SiteAddCommand(st, SiteRequest{Address: "files.example.com", Kind: "static", Dir: "/srv/files", TLS: "custom",
		Cert: "/etc/ssl/certs/files.pem", Key: "/etc/letsencrypt/live/files.example.com/privkey.pem", KeepHTTP: true, HSTS: true, MaxBody: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"files.example.com, http://files.example.com {", "tls /etc/ssl/certs/files.pem /etc/letsencrypt/live/files.example.com/privkey.pem",
		"root * /srv/files", "file_server", "max_size 20MB", "Strict-Transport-Security", "output file /var/log/caddy/hostower-files.example.com.log"} {
		if !strings.Contains(cmd, s) {
			t.Errorf("Caddy 站点里缺少 %q：\n%s", s, cmd)
		}
	}
	if got := e.Classify(cmd); got.Level != risk.L1 {
		t.Errorf("Caddy 站点应是 L1，得到 %s：%+v", got.Level, got.Findings)
	}
	cmd, _, err = SiteAddCommand(st, SiteRequest{Address: "go.example.com", Kind: "redirect", URL: "https://example.com", TLS: "http"})
	if err != nil || !strings.Contains(cmd, "http://go.example.com {") || !strings.Contains(cmd, "redir https://example.com temporary") {
		t.Errorf("Caddy 跳转站点不对：%v\n%s", err, cmd)
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "198.51.100.7:8443", Port: 1, TLS: "auto"}); err == nil {
		t.Error("IP 地址申请不了公开证书")
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "a.example.com", Port: 1, TLS: "le", Agree: true}); err == nil {
		t.Error("Caddy 不用 certbot")
	}
}

func TestCerts(t *testing.T) {
	e := risk.New()
	st := Parse(debianOut, 22)
	for _, req := range []CertRequest{{Action: "dry-run"}, {Action: "renew"}, {Action: "force", Name: "app.example.com"}} {
		cmd, _, err := CertCommand(st, req)
		if err != nil || e.Classify(cmd).Level != risk.L1 {
			t.Errorf("%s 应是 L1：%v\n%s", req.Action, err, cmd)
		}
	}
	cmd, _, _ := CertCommand(st, CertRequest{Action: "force", Name: "app.example.com"})
	if !strings.Contains(cmd, "--cert-name app.example.com --force-renewal --deploy-hook 'systemctl reload nginx'") {
		t.Errorf("强制续期不对：\n%s", cmd)
	}
	for _, bad := range []CertRequest{{Action: "force", Name: "nope.example.com"}, {Action: "force", Name: "app.example.com; reboot"}, {Action: "delete", Name: "app.example.com"}} {
		if _, _, err := CertCommand(st, bad); err == nil {
			t.Errorf("应被拒绝：%+v", bad)
		}
	}
}

func TestFail2ban(t *testing.T) {
	e := risk.New()
	st := Parse(debianOut, 22)
	c := F2BConfig{BanTime: 7200, FindTime: 600, MaxRetry: 3, Ignore: []string{"198.51.100.0/24", " 192.0.2.7 ", "198.51.100.0/24"}, Jails: []string{"nginx-botsearch", "sshd"}}
	cmd, err := F2BCommand(st, F2BRequest{Action: "config", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"cp -p /etc/fail2ban/jail.d/hostower.local /etc/fail2ban/jail.d/hostower.local.bak",
		"ignoreip = 127.0.0.1/8 ::1 203.0.113.9 198.51.100.0/24 192.0.2.7\n", "bantime = 7200", "maxretry = 3",
		"[sshd]\nenabled = true\nport = 22\nbackend = systemd\n", "[nginx-botsearch]\nenabled = true", "[nginx-http-auth]\nenabled = false",
		"jails=sshd,nginx-botsearch", "fail2ban-client -t", "mv -f /etc/fail2ban/jail.d/hostower.local.bak /etc/fail2ban/jail.d/hostower.local", "fail2ban-client reload"} {
		if !strings.Contains(cmd, s) {
			t.Errorf("改 Fail2ban 设置的命令里缺少 %q：\n%s", s, cmd)
		}
	}
	if got := e.Classify(cmd); got.Level != risk.L2 {
		t.Errorf("改 Fail2ban 设置应是 L2，得到 %s：%+v", got.Level, got.Findings)
	}
	un, err := F2BCommand(st, F2BRequest{Action: "unban", Jail: "sshd", IP: "203.0.113.45"})
	if err != nil || !strings.Contains(un, "fail2ban-client set sshd unbanip 203.0.113.45") || e.Classify(un).Level != risk.L2 {
		t.Errorf("解封应是 L2：%v\n%s", err, un)
	}
	for _, bad := range []F2BRequest{
		{Action: "unban", Jail: "sshd", IP: "203.0.113.0/24"},
		{Action: "unban", Jail: "sshd", IP: "192.0.2.4; reboot"},
		{Action: "unban", Jail: "recidive", IP: "203.0.113.45"},
		{Action: "config", Config: F2BConfig{BanTime: 10, FindTime: 600, MaxRetry: 5}},
		{Action: "config", Config: F2BConfig{BanTime: 600, FindTime: 600, MaxRetry: 0}},
		{Action: "config", Config: F2BConfig{BanTime: 600, FindTime: 600, MaxRetry: 5, Ignore: []string{"example.com"}}},
		{Action: "config", Config: F2BConfig{BanTime: 600, FindTime: 600, MaxRetry: 5, Jails: []string{"recidive"}}},
		{Action: "install", Config: DefaultF2B()},
	} {
		if _, err := F2BCommand(st, bad); err == nil {
			t.Errorf("应被拒绝：%+v", bad)
		}
	}
	// 安装：先写配置、再装包、检查、启动，判级 L2。
	fresh := Parse(strings.NewReplacer("fail2ban-client\n", "", "== f2b\nactive", "== f2b").Replace(debianOut), 22)
	fresh.Fail2ban.Config, fresh.Fail2ban.Jails = nil, nil
	inst, err := F2BCommand(fresh, F2BRequest{Action: "install", Config: DefaultF2B()})
	if err != nil {
		t.Fatal(err)
	}
	iConf, iPkg, iTest, iStart := strings.Index(inst, "tee /etc/fail2ban/jail.d/hostower.local"), strings.Index(inst, "apt-get install -y -q fail2ban python3-systemd"),
		strings.Index(inst, "fail2ban-client -t"), strings.Index(inst, "systemctl restart fail2ban")
	if iConf < 0 || iPkg < iConf || iTest < iPkg || iStart < iTest {
		t.Errorf("安装 Fail2ban 的顺序不对：\n%s", inst)
	}
	if got := e.Classify(inst).Level; got != risk.L2 {
		t.Errorf("安装 Fail2ban 应是 L2，得到 %s", got)
	}
	// 重启后要等它能应答再查状态，否则会报“Failed to access socket path”。
	if iWait, iStatus := strings.Index(inst, "fail2ban-client ping"), strings.LastIndex(inst, "fail2ban-client status"); iWait < iStart || iStatus < iWait {
		t.Errorf("启动后没有先等 Fail2ban 就绪再查状态：\n%s", inst)
	}
	noNginx := Parse(strings.Replace(debianOut, "nginx\n", "", 1), 22)
	if _, err := F2BCommand(noNginx, F2BRequest{Action: "config", Config: F2BConfig{BanTime: 600, FindTime: 600, MaxRetry: 5, Jails: []string{"nginx-botsearch"}}}); err == nil {
		t.Error("没装 Nginx 时开不了 Web 防护")
	}
}

// 启动没在运行的反向代理：L1；失败时带上服务日志；没装或已经在运行时不给命令。
func TestServerStart(t *testing.T) {
	e := risk.New()
	st := Parse(ubuntuOut, 22)
	st.Caddy.Installed, st.Caddy.Active = true, false
	cmd, err := ServerStartCommand(st, "caddy")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"systemctl start caddy", "journalctl -u caddy -n 20 --no-pager", "exit 1"} {
		if !strings.Contains(cmd, s) {
			t.Errorf("启动命令里缺少 %q：\n%s", s, cmd)
		}
	}
	if got := e.Classify(cmd).Level; got != risk.L1 {
		t.Errorf("启动 Caddy 应是 L1，得到 %s", got)
	}
	st.Caddy.Active = true
	if _, err := ServerStartCommand(st, "caddy"); err == nil {
		t.Error("已经在运行时不应给出启动命令")
	}
	st.Nginx.Installed = false
	if _, err := ServerStartCommand(st, "nginx"); err == nil {
		t.Error("没装时不应给出启动命令")
	}
	if _, err := ServerStartCommand(st, "sshd"); err == nil {
		t.Error("只能启动 Caddy 或 Nginx")
	}
}

// 站点要用的端口被别的程序占着：检测时看得到程序名就直接拒绝；看不到的，命令开头再查一次。
// 重载失败要撤下刚写的站点并重启服务（Caddy 重载失败后会卡在“重载中”）。
func TestSitePortBusy(t *testing.T) {
	e := risk.New()
	out := strings.Replace(ubuntuOut, "== listen\n", "== listen\nLISTEN 0 4096 *:443 *:* users:((\"xray-linux-amd6\",pid=222828,fd=9))\n", 1)
	st := Parse(out, 22)
	st.Caddy.Installed = true
	var held *Listen
	for i := range st.Listen {
		if st.Listen[i].Port == 443 {
			held = &st.Listen[i]
		}
	}
	if held == nil || held.Proc != "xray-linux-amd6" {
		t.Fatalf("应认出占着 443 的程序：%+v", st.Listen)
	}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "app.example.com", Port: 3001}); err == nil || !strings.Contains(err.Error(), "443 端口已经被 xray-linux-amd6 占用") {
		t.Errorf("443 被别的程序占着时应拒绝，得到 %v", err)
	}
	// 带端口的地址、只用 HTTP 的站点不受 443 影响。
	cmd, _, err := SiteAddCommand(st, SiteRequest{Address: "app.example.com:8443", Port: 3001})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{`grep -E '[:.]8443[[:space:]]' | grep -v '"caddy"'`, "没有改动任何东西",
		"systemctl reload-or-restart caddy", "app.example.com_8443.caddy.rejected", "systemctl restart caddy", "已撤下这个站点并恢复原来的配置"} {
		if !strings.Contains(cmd, s) {
			t.Errorf("命令里缺少 %q：\n%s", s, cmd)
		}
	}
	if strings.Index(cmd, "ss -Htlnp") > strings.Index(cmd, "mkdir -p") {
		t.Error("端口检查要在动任何文件之前")
	}
	if got := e.Classify(cmd).Level; got != risk.L1 {
		t.Errorf("添加站点应是 L1，得到 %s：%+v", got, e.Classify(cmd).Findings)
	}
	// Caddy 自己占着 443 不算被占。
	st.Listen = []Listen{{Port: 443, Addr: "*:443", Proc: "caddy"}}
	if _, _, err := SiteAddCommand(st, SiteRequest{Address: "app.example.com", Port: 3001}); err != nil {
		t.Errorf("Caddy 自己占着 443 时应能添加：%v", err)
	}
}

// systemctl is-active 报 reloading 时进程是在的：不能当成没在运行（否则页面会给出只会卡住的“启动”）。
func TestReloadingCountsAsRunning(t *testing.T) {
	out := strings.Replace(ubuntuOut, "== caddy\nactive\n", "== caddy\nreloading\n", 1)
	if out == ubuntuOut {
		t.Skip("样例输出里没有 Caddy 的状态")
	}
	if st := Parse(out, 22); !st.Caddy.Active {
		t.Error("reloading 应算在运行")
	}
}
