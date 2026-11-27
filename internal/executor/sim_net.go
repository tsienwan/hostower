package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/netcfg"
)

// 模拟主机上的防火墙、Caddy / Nginx、证书和 Fail2ban：按主机名派生出几种常见情况
// （Ubuntu 开着 ufw、装了 Caddy 和 Fail2ban；裸的 Debian；没装 firewalld 的 Rocky；Alpine），
// 记住放行的端口、站点、证书和封禁，好在开发模式里演示“网络”页。

type simSite struct {
	server string // caddy | nginx
	marker string // 标记行里地址之后的部分
}

type simNet struct {
	family         string // debian | rhel | other
	os, id, like   string
	ufw, ufwOn     bool
	fwd, fwdOn     bool
	rules          map[string]bool
	caddy, caddyOn bool
	imported       bool
	nginx, nginxOn bool
	certbot        bool
	certs          map[string]int // 证书名 → 还剩几天
	f2b, f2bOn     bool
	authLog        bool
	f2bConf        string              // Hostower 写的标记行
	jails          map[string][]string // jail → 被封的 IP
	sites          map[string]simSite  // 地址 → 站点
	ts, tsOn       bool                // Tailscale 装了没有、服务在不在跑
	tsState        string              // NeedsLogin | Running | Stopped
	tsFlags        map[string]string   // tailscale up / set 给的参数
}

func (sh *simHost) net() *simNet {
	if sh.netState != nil {
		return sh.netState
	}
	n := &simNet{rules: map[string]bool{}, sites: map[string]simSite{}, certs: map[string]int{}, jails: map[string][]string{}}
	switch {
	case sh.seed < 0.3:
		n.family, n.os, n.id, n.like = "debian", "Ubuntu 24.04.1 LTS", "ubuntu", "debian"
		n.ufw, n.ufwOn = true, true
		n.rules["22/tcp"] = true
		n.caddy, n.caddyOn = true, true
		n.f2b, n.f2bOn, n.authLog = true, true, true
		n.jails["sshd"] = []string{"203.0.113.45", "198.51.100.23"}
	case sh.seed < 0.6:
		n.family, n.os, n.id, n.like = "debian", "Debian GNU/Linux 12 (bookworm)", "debian", ""
	case sh.seed < 0.8:
		n.family, n.os, n.id, n.like = "rhel", "Rocky Linux 9.4 (Blue Onyx)", "rocky", "rhel centos fedora"
		n.authLog = true
	default:
		n.family, n.os, n.id, n.like = "other", "Alpine Linux v3.20", "alpine", ""
	}
	sh.netState = n
	return n
}

func sortedNames[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (n *simNet) detect(h inventory.Host) []string {
	out := []string{"== os", fmt.Sprintf("PRETTY_NAME=%q", n.os), "ID=" + n.id}
	if n.like != "" {
		out = append(out, fmt.Sprintf("ID_LIKE=%q", n.like))
	}
	out = append(out, "== uid", "1000", "== has", "nft", "iptables", "sudo")
	for _, c := range []struct {
		on   bool
		name string
	}{{n.ufw, "ufw"}, {n.fwd, "firewall-cmd"}, {n.fwd, "firewall-offline-cmd"}, {n.caddy, "caddy"}, {n.nginx, "nginx"}, {n.certbot, "certbot"}, {n.f2b, "fail2ban-client"}, {n.ts, "tailscale"}, {h.HasTag("docker"), "docker"}} {
		if c.on {
			out = append(out, c.name)
		}
	}
	switch n.family {
	case "debian":
		out = append(out, "apt-get")
	case "rhel":
		out = append(out, "dnf")
	}
	rules := sortedNames(n.rules)
	out = append(out, "== ufw")
	if n.ufw {
		if n.ufwOn {
			out = append(out, "Status: active", "", "To                         Action      From", "--                         ------      ----")
			for _, r := range rules {
				out = append(out, fmt.Sprintf("%-26s ALLOW       Anywhere", r))
			}
		} else {
			out = append(out, "Status: inactive")
		}
	}
	out = append(out, "== firewalld")
	if n.fwdOn {
		out = append(out, "active", "== firewalld-ports", strings.Join(rules, " "), "== firewalld-services", "ssh")
	} else {
		out = append(out, "inactive", "== firewalld-ports", "== firewalld-services")
	}
	out = append(out, "== caddy")
	if n.caddyOn {
		out = append(out, "active")
	}
	out = append(out, "== caddy-import")
	if n.imported {
		out = append(out, "import "+netcfg.SitesDir+"/*.caddy")
	}
	addrs := sortedNames(n.sites)
	for _, server := range []string{"caddy", "nginx"} {
		if server == "caddy" {
			out = append(out, "== sites")
		} else {
			out = append(out, "== nginx")
			if n.nginxOn {
				out = append(out, "active")
			}
			out = append(out, "== nginx-sites")
		}
		for _, a := range addrs {
			if s := n.sites[a]; s.server == server {
				out = append(out, "# hostower-site "+a+" "+s.marker)
			}
		}
	}
	out = append(out, "== certs")
	if n.certbot && len(n.certs) > 0 {
		out = append(out, "Found the following certs:")
		for _, c := range sortedNames(n.certs) {
			days := n.certs[c]
			out = append(out, "  Certificate Name: "+c, "    Domains: "+c,
				fmt.Sprintf("    Expiry Date: %s (VALID: %d days)", "2027-02-17 08:00:00+00:00", days),
				"    Certificate Path: /etc/letsencrypt/live/"+c+"/fullchain.pem")
		}
	}
	out = append(out, "== renew")
	if n.certbot {
		out = append(out, "active")
	}
	out = append(out, "== f2b")
	if n.f2bOn {
		out = append(out, "active", "== f2b-jails")
		for _, j := range sortedNames(n.jails) {
			ips := n.jails[j]
			out = append(out, "-- "+j, "Status for the jail: "+j, "|- Filter", "|  |- Currently failed:\t1", fmt.Sprintf("|  |- Total failed:\t%d", 17+len(ips)*5),
				"`- Actions", fmt.Sprintf("   |- Currently banned:\t%d", len(ips)), fmt.Sprintf("   |- Total banned:\t%d", len(ips)+3),
				"   `- Banned IP list:\t"+strings.Join(ips, " "))
		}
	} else {
		out = append(out, "== f2b-jails")
	}
	out = append(out, "== f2b-conf")
	if n.f2bConf != "" {
		out = append(out, n.f2bConf)
	}
	out = append(out, "== authlog")
	if n.authLog {
		out = append(out, "/var/log/auth.log")
	}
	out = append(out, "== ts")
	if n.tsOn {
		// 每台模拟主机一个固定的 Tailscale 地址，名字默认就是主机名。
		sum := 0
		for _, c := range h.Name {
			sum += int(c)
		}
		name := h.Name
		if v := n.tsFlags["hostname"]; v != "" {
			name = v
		}
		out = append(out, "active", "== ts-status")
		if n.tsState == "NeedsLogin" {
			out = append(out, `{"Version":"1.80.2-t1a2b3c","BackendState":"NeedsLogin","TailscaleIPs":null,"Self":{"HostName":"`+name+`","DNSName":""}}`)
		} else {
			out = append(out, fmt.Sprintf(`{"Version":"1.80.2-t1a2b3c","BackendState":%q,"TailscaleIPs":["100.64.0.%d","fd7a:115c:a1e0::%x"],`+
				`"Self":{"HostName":%q,"DNSName":"%s.example-tailnet.ts.net."},"CurrentTailnet":{"Name":"example-tailnet.ts.net"}}`, n.tsState, 2+sum%250, 2+sum%250, name, name))
		}
		out = append(out, "== ts-prefs", fmt.Sprintf(`{"RouteAll":%t,"CorpDNS":%t,"RunSSH":%t,"Hostname":%q}`,
			n.tsFlags["accept-routes"] == "true", n.tsFlags["accept-dns"] != "false", n.tsFlags["ssh"] == "true", n.tsFlags["hostname"]))
	} else {
		out = append(out, "== ts-status", "== ts-prefs")
	}
	out = append(out, "== client", "198.51.100.10")
	out = append(out, "== sshd", "== listen", "LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*", "LISTEN 0 4096 127.0.0.1:18443 0.0.0.0:*")
	if n.caddyOn || n.nginxOn {
		out = append(out, "LISTEN 0 4096 *:80 *:*")
	}
	https := n.caddyOn
	for _, s := range n.sites {
		https = https || (s.server == "nginx" && !strings.Contains(s.marker, "tls=http"))
	}
	if https {
		out = append(out, "LISTEN 0 4096 *:443 *:*")
	}
	// 带 docker 标签的主机：几个直接发布到所有地址的容器（同一个容器多个端口、IPv4 和 IPv6 各一条、TCP 和 UDP 都有），演示“绕过防火墙”的提示。
	type pub struct {
		name  string
		ports []int
		udp   bool
	}
	pubs := []pub{{"stack-web-1", []int{80}, false}, {"compose-bot-monitor-1", []int{8010}, false}, {"compose-bot-monitor-mysql-1", []int{3308}, false},
		{"compose-remake-server-1", []int{35001, 35002, 35003, 35004, 35005}, true}}
	if h.HasTag("docker") {
		for _, p := range pubs {
			for _, n := range p.ports {
				out = append(out, fmt.Sprintf("LISTEN 0 4096 0.0.0.0:%d 0.0.0.0:*", n))
			}
		}
	}
	out = append(out, "== docker")
	if h.HasTag("docker") {
		for _, p := range pubs {
			var m []string
			for _, n := range p.ports {
				m = append(m, fmt.Sprintf("0.0.0.0:%d->%d/tcp, [::]:%d->%d/tcp", n, n, n, n))
				if p.udp {
					m = append(m, fmt.Sprintf("0.0.0.0:%d->%d/udp, [::]:%d->%d/udp", n, n, n, n))
				}
			}
			out = append(out, p.name+"\t"+strings.Join(m, ", "))
		}
		out = append(out, "stack-app-1\t8080/tcp")
	}
	return append(out, "== end")
}

var (
	simUFWAllowRe  = regexp.MustCompile(`then ufw allow (\d+/(?:tcp|udp));`)
	simUFWDeleteRe = regexp.MustCompile(`then ufw delete allow (\d+/(?:tcp|udp));`)
	simFWDAddRe    = regexp.MustCompile(`then firewall(?:-offline)?-cmd (?:--permanent )?--add-port=(\d+/(?:tcp|udp));`)
	simFWDRemoveRe = regexp.MustCompile(`then firewall-cmd --permanent --remove-port=(\d+/(?:tcp|udp));`)
	simSiteRe      = regexp.MustCompile(`(?m)^# hostower-site (\S+) (.+)$`)
	simSiteRmRe    = regexp.MustCompile(`then mv -f (\S+) \S+\.removed;`)
	simF2BRe       = regexp.MustCompile(`(?m)^# hostower-f2b .*$`)
	simUnbanRe     = regexp.MustCompile(`then fail2ban-client set (\S+) unbanip (\S+);`)
	simTSRe        = regexp.MustCompile(`then tailscale (up|set) ([^;]*);`)
	simTSFlagRe    = regexp.MustCompile(`--(hostname|accept-routes|accept-dns|ssh)=(\S*)`)
	simForceRe     = regexp.MustCompile(`then certbot renew --cert-name (\S+) --force-renewal`)
)

// netLog 给“看日志”的只读命令编几行像样的日志。
func netLog(cmd string) ([]string, bool) {
	switch {
	case strings.HasPrefix(cmd, "tail -n 200 /var/log/nginx/") && strings.Contains(cmd, ".access.log"):
		return []string{
			`203.0.113.45 - - [19/Nov/2026:08:01:12 +0000] "GET / HTTP/2.0" 200 5124 "-" "Mozilla/5.0"`,
			`198.51.100.23 - - [19/Nov/2026:08:01:13 +0000] "GET /api/status HTTP/2.0" 200 87 "-" "curl/8.5.0"`,
			`192.0.2.77 - - [19/Nov/2026:08:02:40 +0000] "GET /wp-login.php HTTP/1.1" 404 153 "-" "python-requests/2.31"`,
		}, true
	case strings.HasPrefix(cmd, "tail -n 200 /var/log/nginx/"):
		return []string{`2026/11/19 08:02:40 [error] 1234#1234: *88 open() "/usr/share/nginx/html/wp-login.php" failed (2: No such file or directory), client: 192.0.2.77`}, true
	case strings.HasPrefix(cmd, "tail -n 200 /var/log/caddy/"):
		return []string{`{"level":"info","ts":1795075272.1,"logger":"http.log.access","msg":"handled request","request":{"remote_ip":"203.0.113.45","method":"GET","uri":"/"},"status":200}`}, true
	case strings.HasPrefix(cmd, "journalctl -u caddy"):
		return []string{"Nov 19 08:00:01 web-1 caddy[812]: {\"level\":\"info\",\"msg\":\"certificate obtained successfully\"}"}, true
	case cmd == netcfg.F2BLogCommand:
		return []string{
			"2026-11-19 07:58:02,114 fail2ban.filter [901]: INFO [sshd] Found 203.0.113.45 - 2026-11-19 07:58:01",
			"2026-11-19 07:58:09,532 fail2ban.actions [901]: NOTICE [sshd] Ban 203.0.113.45",
		}, true
	}
	return nil, false
}

// netCommand 处理“网络”页生成的命令；不是这类命令时 ok 为 false。
func (s *Sim) netCommand(h inventory.Host, sh *simHost, cmd string) (lines []string, code int, ok bool) {
	if cmd == netcfg.DetectCommand {
		return sh.net().detect(h), 0, true
	}
	if lines, ok := netLog(cmd); ok {
		return lines, 0, true
	}
	// 只认“网络”页的命令：别的脚本（比如测速前装 curl）也有同样的 root 判断，不能误吞。
	if !strings.Contains(cmd, `if [ "$(id -u)" = 0 ]; then`) ||
		!(strings.Contains(cmd, "then ufw ") || strings.Contains(cmd, "then firewall") || strings.Contains(cmd, "caddy") || strings.Contains(cmd, "nginx") ||
			strings.Contains(cmd, "certbot") || strings.Contains(cmd, "fail2ban") || strings.Contains(cmd, "tailscale") || strings.Contains(cmd, "install -y -q ufw") || strings.Contains(cmd, "install -y -q firewalld")) {
		return nil, 0, false
	}
	n := sh.net()
	say := func(s ...string) { lines = append(lines, s...) }
	if strings.Contains(cmd, "install -y -q ufw") {
		n.ufw = true
		say("Setting up ufw (0.36.2-6) ...")
	}
	if strings.Contains(cmd, "install -y -q firewalld") {
		n.fwd = true
		say("Installed: firewalld-1.3.4-1.el9.noarch")
	}
	if strings.Contains(cmd, "install -y -q caddy") {
		n.caddy, n.caddyOn = true, true
		say("Setting up caddy (2.6.2-5) ...")
	}
	if strings.Contains(cmd, "install -y -q nginx") {
		n.nginx, n.nginxOn = true, true
		say("Setting up nginx (1.22.1-9) ...")
	}
	// 启动、停止反向代理（添加站点时的 reload-or-restart 也会把没在运行的服务启动起来）。
	for _, sv := range []struct {
		name          string
		installed, on *bool
	}{{"caddy", &n.caddy, &n.caddyOn}, {"nginx", &n.nginx, &n.nginxOn}} {
		if !*sv.installed {
			continue
		}
		if strings.Contains(cmd, "systemctl start "+sv.name) || strings.Contains(cmd, "systemctl reload-or-restart "+sv.name) {
			*sv.on = true
		}
		if strings.Contains(cmd, "systemctl stop "+sv.name) {
			*sv.on = false
		}
	}
	if strings.Contains(cmd, "install -y -q certbot") {
		n.certbot = true
		say("Setting up certbot (2.1.0-4) ...", "Created symlink /etc/systemd/system/timers.target.wants/certbot.timer → /lib/systemd/system/certbot.timer.")
	}
	if strings.Contains(cmd, "install -y -q fail2ban") {
		n.f2b = true
		say("Setting up fail2ban (1.0.2-2) ...")
	}
	if strings.Contains(cmd, "install -y -q tailscale") {
		n.ts, n.tsOn, n.tsState, n.tsFlags = true, true, "NeedsLogin", map[string]string{}
		say("Setting up tailscale (1.80.2) ...", "Created symlink /etc/systemd/system/multi-user.target.wants/tailscaled.service → /lib/systemd/system/tailscaled.service.")
	}
	if n.ts {
		if m := simTSRe.FindStringSubmatch(cmd); m != nil {
			if n.tsFlags == nil {
				n.tsFlags = map[string]string{}
			}
			for _, f := range simTSFlagRe.FindAllStringSubmatch(m[2], -1) {
				n.tsFlags[f[1]] = f[2]
			}
			if m[1] == "up" {
				if !strings.Contains(m[2], "--auth-key=") && n.tsState == "NeedsLogin" {
					say("", "To authenticate, visit:", "", "\thttps://login.tailscale.com/a/1a2b3c4d5e6f", "", "Success.")
				}
				n.tsOn, n.tsState = true, "Running"
			}
		}
		if strings.Contains(cmd, "then tailscale down;") {
			n.tsState = "Stopped"
		}
		if strings.Contains(cmd, "then tailscale logout;") {
			n.tsState = "NeedsLogin"
		}
		if strings.Contains(cmd, "tailscale ip -4") && n.tsState == "Running" {
			sum := 0
			for _, c := range h.Name {
				sum += int(c)
			}
			say(fmt.Sprintf("100.64.0.%d", 2+sum%250))
		}
	}
	for _, m := range simUFWAllowRe.FindAllStringSubmatch(cmd, -1) {
		n.rules[m[1]] = true
		say("Rule added")
	}
	for _, m := range simFWDAddRe.FindAllStringSubmatch(cmd, -1) {
		n.rules[m[1]] = true
		say("success")
	}
	for _, m := range simUFWDeleteRe.FindAllStringSubmatch(cmd, -1) {
		delete(n.rules, m[1])
		say("Rule deleted")
	}
	for _, m := range simFWDRemoveRe.FindAllStringSubmatch(cmd, -1) {
		delete(n.rules, m[1])
		say("success")
	}
	if strings.Contains(cmd, "ufw --force enable") {
		n.ufwOn = true
		say("Firewall is active and enabled on system startup")
	}
	if strings.Contains(cmd, "systemctl enable --now firewalld") {
		n.fwdOn = true
	}
	if ms := simSiteRe.FindAllStringSubmatch(cmd, -1); ms != nil {
		m := ms[len(ms)-1]
		server := "caddy"
		if strings.Contains(cmd, netcfg.NginxDir+"/") {
			server = "nginx"
		}
		update := strings.Contains(cmd, ".prev")
		if _, dup := n.sites[m[1]]; dup && !update {
			return append(lines, "Error: ambiguous site definition: "+m[1], "hostower: Caddy 配置校验没通过，已撤下这个站点（地址重复或者写法不对）"), 1, true
		}
		if server == "caddy" {
			n.imported = true
			say("Valid configuration")
		} else {
			say("nginx: the configuration file /etc/nginx/nginx.conf syntax is ok", "nginx: configuration file /etc/nginx/nginx.conf test is successful")
		}
		if strings.Contains(cmd, "certbot certonly") {
			n.certs[m[1]] = 89
			say("Successfully received certificate.", "Certificate is saved at: /etc/letsencrypt/live/"+m[1]+"/fullchain.pem",
				"This certificate expires on 2027-02-17.", "Certbot has set up a scheduled task to automatically renew this certificate in the background.")
		}
		n.sites[m[1]] = simSite{server: server, marker: m[2]}
		if update {
			say("hostower: 站点 " + m[1] + " 已改好")
		} else {
			say("hostower: 站点 " + m[1] + " 已生效")
		}
	}
	if m := simSiteRmRe.FindStringSubmatch(cmd); m != nil {
		for a, st := range n.sites {
			if netcfg.SiteFile(st.server, a) == m[1] {
				delete(n.sites, a)
			}
		}
		say("hostower: 站点已撤下")
	}
	switch {
	case strings.Contains(cmd, "certbot renew --dry-run"):
		say("Processing /etc/letsencrypt/renewal/*.conf", "Congratulations, all simulated renewals succeeded.")
	case simForceRe.MatchString(cmd):
		name := simForceRe.FindStringSubmatch(cmd)[1]
		n.certs[name] = 89
		say("Renewing an existing certificate for "+name, "Congratulations, all renewals succeeded.")
	case strings.Contains(cmd, "then certbot renew"):
		say("Processing /etc/letsencrypt/renewal/*.conf", "No renewals were attempted.")
	}
	if m := simF2BRe.FindString(cmd); m != "" {
		n.f2bConf, n.f2bOn = m, true
		want := map[string]bool{}
		for _, kv := range strings.Fields(m) {
			if v, ok := strings.CutPrefix(kv, "jails="); ok {
				for _, j := range strings.Split(v, ",") {
					if j != "" {
						want[j] = true
					}
				}
			}
		}
		for j := range n.jails {
			if !want[j] {
				delete(n.jails, j)
			}
		}
		for j := range want {
			if _, ok := n.jails[j]; !ok {
				n.jails[j] = []string{}
				if j == "sshd" {
					n.jails[j] = []string{"192.0.2.201"}
				}
			}
		}
		say("OK: configuration test is successful", "Status", fmt.Sprintf("|- Number of jail:\t%d", len(n.jails)), "`- Jail list:\t"+strings.Join(sortedNames(n.jails), ", "))
	}
	for _, m := range simUnbanRe.FindAllStringSubmatch(cmd, -1) {
		ips := n.jails[m[1]]
		for i, ip := range ips {
			if ip == m[2] {
				n.jails[m[1]] = append(ips[:i:i], ips[i+1:]...)
				break
			}
		}
		say("1")
	}
	if len(lines) == 0 {
		say("ok")
	}
	return lines, 0, true
}
