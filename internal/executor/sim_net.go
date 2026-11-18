package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/netcfg"
)

// 模拟主机上的防火墙和 Caddy：按主机名派生出几种常见情况（Ubuntu 开着 ufw、裸的 Debian、没装 firewalld 的 Rocky、Alpine），
// 记住放行的端口和站点，好在开发模式里演示“网络”页。

type simNet struct {
	family         string // debian | rhel | other
	os, id, like   string
	ufw, ufwOn     bool
	fwd, fwdOn     bool
	rules          map[string]bool
	caddy, caddyOn bool
	imported       bool
	sites          map[string]string // 地址 → 上游
}

func (sh *simHost) net() *simNet {
	if sh.netState != nil {
		return sh.netState
	}
	n := &simNet{rules: map[string]bool{}, sites: map[string]string{}}
	switch {
	case sh.seed < 0.3:
		n.family, n.os, n.id, n.like = "debian", "Ubuntu 24.04.1 LTS", "ubuntu", "debian"
		n.ufw, n.ufwOn = true, true
		n.rules["22/tcp"] = true
		n.caddy, n.caddyOn = true, true
	case sh.seed < 0.6:
		n.family, n.os, n.id, n.like = "debian", "Debian GNU/Linux 12 (bookworm)", "debian", ""
	case sh.seed < 0.8:
		n.family, n.os, n.id, n.like = "rhel", "Rocky Linux 9.4 (Blue Onyx)", "rocky", "rhel centos fedora"
	default:
		n.family, n.os, n.id, n.like = "other", "Alpine Linux v3.20", "alpine", ""
	}
	sh.netState = n
	return n
}

func (n *simNet) detect(h inventory.Host) []string {
	out := []string{"== os", fmt.Sprintf("PRETTY_NAME=%q", n.os), "ID=" + n.id}
	if n.like != "" {
		out = append(out, fmt.Sprintf("ID_LIKE=%q", n.like))
	}
	out = append(out, "== uid", "1000", "== has", "nft", "iptables", "sudo")
	if n.ufw {
		out = append(out, "ufw")
	}
	if n.fwd {
		out = append(out, "firewall-cmd", "firewall-offline-cmd")
	}
	if n.caddy {
		out = append(out, "caddy")
	}
	if h.HasTag("docker") {
		out = append(out, "docker")
	}
	switch n.family {
	case "debian":
		out = append(out, "apt-get")
	case "rhel":
		out = append(out, "dnf")
	}
	rules := make([]string, 0, len(n.rules))
	for r := range n.rules {
		rules = append(rules, r)
	}
	sort.Strings(rules)
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
	out = append(out, "== sites")
	addrs := make([]string, 0, len(n.sites))
	for a := range n.sites {
		addrs = append(addrs, a)
	}
	sort.Strings(addrs)
	for _, a := range addrs {
		out = append(out, "# hostower-site "+a+" "+n.sites[a])
	}
	out = append(out, "== sshd", "== listen", "LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*", "LISTEN 0 4096 127.0.0.1:18443 0.0.0.0:*")
	if n.caddyOn {
		out = append(out, "LISTEN 0 4096 *:80 *:*", "LISTEN 0 4096 *:443 *:*")
	}
	out = append(out, "== docker")
	if h.HasTag("docker") {
		out = append(out, "stack-web-1\t0.0.0.0:80->80/tcp, [::]:80->80/tcp", "stack-app-1\t8080/tcp")
	}
	return append(out, "== end")
}

var (
	simUFWAllowRe  = regexp.MustCompile(`then ufw allow (\d+/(?:tcp|udp));`)
	simUFWDeleteRe = regexp.MustCompile(`then ufw delete allow (\d+/(?:tcp|udp));`)
	simFWDAddRe    = regexp.MustCompile(`then firewall(?:-offline)?-cmd (?:--permanent )?--add-port=(\d+/(?:tcp|udp));`)
	simFWDRemoveRe = regexp.MustCompile(`then firewall-cmd --permanent --remove-port=(\d+/(?:tcp|udp));`)
	simSiteRe      = regexp.MustCompile(`'# hostower-site (\S+) (\S+)'`)
	simSiteRmRe    = regexp.MustCompile(`then mv -f /etc/caddy/sites\.d/(\S+)\.caddy /etc/caddy/sites\.d/\S+\.caddy\.removed;`)
)

// netCommand 处理“网络”页生成的命令；不是这类命令时 ok 为 false。
func (s *Sim) netCommand(h inventory.Host, sh *simHost, cmd string) (lines []string, code int, ok bool) {
	if cmd == netcfg.DetectCommand {
		return sh.net().detect(h), 0, true
	}
	// 只认“网络”页的命令：别的脚本（比如测速前装 curl）也有同样的 root 判断，不能误吞。
	if !strings.Contains(cmd, `if [ "$(id -u)" = 0 ]; then`) ||
		!(strings.Contains(cmd, "then ufw ") || strings.Contains(cmd, "then firewall") || strings.Contains(cmd, "caddy") || strings.Contains(cmd, "install -y -q ufw") || strings.Contains(cmd, "install -y -q firewalld")) {
		return nil, 0, false
	}
	n := sh.net()
	say := func(s string) { lines = append(lines, s) }
	switch {
	case strings.Contains(cmd, "install -y -q ufw"):
		n.ufw = true
		say("Setting up ufw (0.36.2-6) ...")
	case strings.Contains(cmd, "install -y -q firewalld"):
		n.fwd = true
		say("Installed: firewalld-1.3.4-1.el9.noarch")
	case strings.Contains(cmd, "install -y -q caddy"):
		n.caddy, n.caddyOn = true, true
		say("Setting up caddy (2.6.2-5) ...")
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
	if m := simSiteRe.FindStringSubmatch(cmd); m != nil {
		if _, dup := n.sites[m[1]]; dup {
			return append(lines, "Error: ambiguous site definition: "+m[1], "hostower: Caddy 配置校验没通过，已撤下这个站点（地址重复或者写法不对）"), 1, true
		}
		n.imported = true
		n.sites[m[1]] = m[2]
		say("Valid configuration")
		say("hostower: 站点 " + m[1] + " → " + m[2] + " 已生效")
	}
	if m := simSiteRmRe.FindStringSubmatch(cmd); m != nil {
		delete(n.sites, strings.ReplaceAll(m[1], "_", ":"))
		say("hostower: 站点已撤下")
	}
	return lines, 0, true
}
