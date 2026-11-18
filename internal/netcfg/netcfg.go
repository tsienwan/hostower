// Package netcfg 管主机的“网络入口”：防火墙（ufw / firewalld）放行端口，以及用 Caddy 做反向代理（域名 + 自动 HTTPS）。
//
// 这里不碰主机：只生成一段只读的检测脚本、解析它的输出，再把要做的改动渲染成命令。
// 命令交给网关走 kernel.SubmitCommand：判级、审批、执行、审计。改防火墙在判级里是 L2（会影响远程访问），
// 写 Caddy 站点配置是 L1，检测是 L0。
//
// 两条底线：
//   - 不把自己锁在外面：启用防火墙前先放行 SSH 端口（和你勾选的其他端口）；SSH 端口不能被关掉。
//   - 不混用：主机上已经在用的防火墙是哪个就用哪个；ufw 和 firewalld 同时启用时什么都不做，只提示。
package netcfg

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/snippet"
)

// SitesDir 是 Hostower 管理的 Caddy 站点目录：每个站点一个文件，主配置里只加一行 import。
const SitesDir = "/etc/caddy/sites.d"

// Caddyfile 是 Caddy 的主配置。
const Caddyfile = "/etc/caddy/Caddyfile"

const siteMarker = "# hostower-site "

// DetectCommand 是只读的检测脚本：系统、防火墙、Caddy、对外监听的端口、Docker 发布的端口。必须是 L0（测试里锁住）。
const DetectCommand = `echo '== os'; cat /etc/os-release 2>/dev/null; ` +
	`echo '== uid'; id -u; ` +
	`echo '== nnp'; grep -s '^NoNewPrivs:' /proc/self/status; ` +
	`echo '== has'; for c in ufw firewall-cmd firewall-offline-cmd nft iptables caddy docker apt-get dnf yum sudo; do command -v "$c" >/dev/null 2>&1 && echo "$c"; done; ` +
	`echo '== ufw'; ufw status 2>/dev/null || sudo -n ufw status 2>/dev/null; ` +
	`echo '== firewalld'; systemctl is-active firewalld 2>/dev/null; ` +
	`echo '== firewalld-ports'; firewall-cmd --list-ports 2>/dev/null || sudo -n firewall-cmd --list-ports 2>/dev/null; ` +
	`echo '== firewalld-services'; firewall-cmd --list-services 2>/dev/null || sudo -n firewall-cmd --list-services 2>/dev/null; ` +
	`echo '== caddy'; systemctl is-active caddy 2>/dev/null; ` +
	`echo '== caddy-import'; grep -s '^import ` + SitesDir + `/' ` + Caddyfile + `; ` +
	`echo '== sites'; grep -hs '^# hostower-site ' ` + SitesDir + `/*.caddy; ` +
	`echo '== sshd'; grep -is '^[[:space:]]*port[[:space:]]' /etc/ssh/sshd_config; ` +
	`echo '== listen'; ss -Htln 2>/dev/null; ` +
	`echo '== docker'; docker ps --format '{{.Names}}	{{.Ports}}' 2>/dev/null; ` +
	`echo '== end'`

// Port 是一条放行规则。
type Port struct {
	Port  int    `json:"port"`
	Proto string `json:"proto"` // tcp | udp
}

func (p Port) String() string { return strconv.Itoa(p.Port) + "/" + p.Proto }

// Valid 校验端口。
func (p Port) Valid() error {
	if p.Port < 1 || p.Port > 65535 {
		return errors.New("端口要在 1 到 65535 之间")
	}
	if p.Proto != "tcp" && p.Proto != "udp" {
		return errors.New("协议只能是 tcp 或 udp")
	}
	return nil
}

// Listen 是一个正在对外（非回环地址）监听的端口。
type Listen struct {
	Port int    `json:"port"`
	Addr string `json:"addr"`
}

// DockerPort 是 Docker 直接发布到所有地址的端口：它们绕过 ufw 和 firewalld。
type DockerPort struct {
	Container string `json:"container"`
	Port      int    `json:"port"`
	Proto     string `json:"proto"`
}

// Site 是一个 Hostower 管理的 Caddy 站点。
type Site struct {
	Address  string `json:"address"`
	Upstream string `json:"upstream"`
}

// Status 是检测结果。
type Status struct {
	OS     string `json:"os"`     // 例如 Ubuntu 24.04.1 LTS
	Family string `json:"family"` // debian | rhel | other
	Root   bool   `json:"root"`   // 登录账号就是 root
	Sudo   bool   `json:"sudo"`   // 有 sudo 命令（免密与否要执行时才知道）
	// NoNewPrivs 表示执行命令的进程被禁止提权（systemd 的 NoNewPrivileges=true）：这时 sudo 一定失败。
	// 典型情况是“本机”主机：命令由 Hostower 进程直接执行，继承了服务的这项加固。
	NoNewPrivs bool `json:"noNewPrivs"`
	Firewall   struct {
		// Tool 是正在用的防火墙：ufw | firewalld | ""（都没启用）| conflict（两个都启用了）
		Tool string `json:"tool"`
		// Recommended 是这台主机推荐装的那个（都没装时）：ufw | firewalld | ""（不支持一键安装）
		Recommended  string   `json:"recommended"`
		UFWInstalled bool     `json:"ufwInstalled"`
		UFWActive    bool     `json:"ufwActive"`
		FWDInstalled bool     `json:"firewalldInstalled"`
		FWDActive    bool     `json:"firewalldActive"`
		Rules        []string `json:"rules"` // 放行的端口和服务（原样）
		Unknown      bool     `json:"unknown"`
	} `json:"firewall"`
	Caddy struct {
		Installed   bool `json:"installed"`
		Active      bool `json:"active"`
		Imported    bool `json:"imported"`    // 主配置里已经有 import sites.d
		Installable bool `json:"installable"` // 系统的软件源里有 Caddy（Debian/Ubuntu、Fedora 等）
	} `json:"caddy"`
	Sites   []Site       `json:"sites"`
	SSHPort int          `json:"sshPort"`
	Listen  []Listen     `json:"listen"`
	Docker  []DockerPort `json:"docker"`
}

var (
	ufwRuleRe   = regexp.MustCompile(`^(\S+)\s+(ALLOW|DENY|REJECT|LIMIT)(?:\s+IN)?\s+(.+)$`)
	dockerPubRe = regexp.MustCompile(`(?:^|,\s*)(?:0\.0\.0\.0|\[?::\]?):(\d+)->\d+/(tcp|udp)`)
)

// Parse 解析检测脚本的输出。sshPort 是 Hostower 连这台主机用的 SSH 端口（不知道时传 0，就用 sshd_config 里的）。
func Parse(out string, sshPort int) Status {
	var st Status
	st.Sites, st.Listen, st.Docker, st.Firewall.Rules = []Site{}, []Listen{}, []DockerPort{}, []string{}
	sec := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if strings.HasPrefix(line, "== ") {
			cur = strings.TrimPrefix(line, "== ")
			continue
		}
		if cur != "" && strings.TrimSpace(line) != "" {
			sec[cur] = append(sec[cur], line)
		}
	}
	osv := map[string]string{}
	for _, l := range sec["os"] {
		if k, v, ok := strings.Cut(l, "="); ok {
			osv[k] = strings.Trim(v, `"`)
		}
	}
	st.OS = osv["PRETTY_NAME"]
	ids := " " + osv["ID"] + " " + osv["ID_LIKE"] + " "
	switch {
	case strings.Contains(ids, " debian ") || strings.Contains(ids, " ubuntu "):
		st.Family = "debian"
	case strings.Contains(ids, " rhel ") || strings.Contains(ids, " fedora ") || strings.Contains(ids, " centos "):
		st.Family = "rhel"
	default:
		st.Family = "other"
	}
	if len(sec["uid"]) > 0 {
		st.Root = strings.TrimSpace(sec["uid"][0]) == "0"
	}
	has := map[string]bool{}
	for _, c := range sec["has"] {
		has[strings.TrimSpace(c)] = true
	}
	st.Sudo = has["sudo"]
	for _, l := range sec["nnp"] {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "NoNewPrivs:" && f[1] == "1" {
			st.NoNewPrivs = true
		}
	}
	fw := &st.Firewall
	fw.UFWInstalled = has["ufw"]
	fw.FWDInstalled = has["firewall-cmd"]
	for _, l := range sec["ufw"] {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "Status: active") {
			fw.UFWActive = true
		}
		if m := ufwRuleRe.FindStringSubmatch(l); m != nil && m[1] != "To" {
			if m[2] == "ALLOW" || m[2] == "LIMIT" {
				fw.Rules = append(fw.Rules, m[1])
			}
		}
	}
	if fw.UFWInstalled && len(sec["ufw"]) == 0 {
		fw.Unknown = true // 读不出 ufw 的状态（没有免密 sudo）
	}
	if len(sec["firewalld"]) > 0 && strings.TrimSpace(sec["firewalld"][0]) == "active" {
		fw.FWDActive = true
		for _, l := range append(sec["firewalld-ports"], sec["firewalld-services"]...) {
			fw.Rules = append(fw.Rules, strings.Fields(l)...)
		}
	}
	switch {
	case fw.UFWActive && fw.FWDActive:
		fw.Tool = "conflict"
	case fw.UFWActive:
		fw.Tool = "ufw"
	case fw.FWDActive:
		fw.Tool = "firewalld"
	}
	switch {
	case st.Family == "debian" && has["apt-get"]:
		fw.Recommended = "ufw"
	case st.Family == "rhel" && (has["dnf"] || has["yum"]):
		fw.Recommended = "firewalld"
	}
	st.Caddy.Installed = has["caddy"]
	st.Caddy.Active = len(sec["caddy"]) > 0 && strings.TrimSpace(sec["caddy"][0]) == "active"
	st.Caddy.Imported = len(sec["caddy-import"]) > 0
	st.Caddy.Installable = fw.Recommended != "" // 和防火墙一样：apt / dnf 的发行版
	for _, l := range sec["sites"] {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(l), siteMarker); ok {
			if f := strings.Fields(rest); len(f) == 2 {
				st.Sites = append(st.Sites, Site{Address: f[0], Upstream: f[1]})
			}
		}
	}
	st.SSHPort = sshPort
	if st.SSHPort == 0 {
		st.SSHPort = 22
		for _, l := range sec["sshd"] {
			if f := strings.Fields(l); len(f) >= 2 {
				if n, err := strconv.Atoi(f[1]); err == nil && n > 0 && n < 65536 {
					st.SSHPort = n
					break
				}
			}
		}
	}
	seen := map[int]bool{}
	for _, l := range sec["listen"] {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		host, port, err := net.SplitHostPort(f[3])
		n, perr := strconv.Atoi(port)
		if err != nil || perr != nil || seen[n] {
			continue
		}
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() {
			continue
		}
		if strings.HasPrefix(host, "127.") || host == "localhost" {
			continue
		}
		seen[n] = true
		st.Listen = append(st.Listen, Listen{Port: n, Addr: f[3]})
	}
	sort.Slice(st.Listen, func(i, j int) bool { return st.Listen[i].Port < st.Listen[j].Port })
	dseen := map[string]bool{}
	for _, l := range sec["docker"] {
		name, ports, _ := strings.Cut(l, "\t")
		for _, m := range dockerPubRe.FindAllStringSubmatch(ports, -1) {
			n, _ := strconv.Atoi(m[1])
			k := name + "/" + m[1] + "/" + m[2]
			if !dseen[k] {
				dseen[k] = true
				st.Docker = append(st.Docker, DockerPort{Container: name, Port: n, Proto: m[2]})
			}
		}
	}
	return st
}

// CanRoot 报告能不能以 root 身份改系统：本来就是 root，或者有 sudo 且没被禁止提权。
func (st Status) CanRoot() bool { return st.Root || (st.Sudo && !st.NoNewPrivs) }

func (st Status) rootErr(what string) error {
	if st.NoNewPrivs {
		return errors.New("执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，" + what + "。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管")
	}
	return errors.New("登录账号不是 root，也没有 sudo：" + what)
}

// asRoot 以 root 身份运行一条命令：本来就是 root 就直接运行，否则用免密 sudo。
// 两个分支都写明完整的命令，判级引擎看得到实际要做什么（不能用变量拼命令名，那样会把它藏起来）。
func asRoot(cmd string) string {
	return `if [ "$(id -u)" = 0 ]; then ` + cmd + `; else sudo -n ` + cmd + `; fi`
}

func step(cmd, fail string) string {
	return "{ " + asRoot(cmd) + "; } || { echo " + snippet.Quote("hostower: "+fail) + "; exit 1; }; "
}

// FirewallRequest 是一次防火墙操作。
type FirewallRequest struct {
	Action string `json:"action"` // setup（安装并启用 / 只启用）| allow | remove
	Tool   string `json:"tool"`   // setup 时选的防火墙：ufw | firewalld（空表示用推荐的）
	Ports  []Port `json:"ports"`  // setup 时一起放行的端口；allow / remove 的端口
}

// FirewallCommand 渲染防火墙操作。keep 是绝对不能关掉的端口（SSH，以及面板自己对外的端口）。
func FirewallCommand(st Status, req FirewallRequest, keep []Port) (string, string, error) {
	for _, p := range req.Ports {
		if err := p.Valid(); err != nil {
			return "", "", err
		}
	}
	fw := st.Firewall
	if fw.Tool == "conflict" {
		return "", "", errors.New("ufw 和 firewalld 同时启用了：请先在主机上停掉其中一个，Hostower 不会同时管两个")
	}
	if !st.CanRoot() {
		return "", "", st.rootErr("改不了防火墙")
	}
	var b strings.Builder
	switch req.Action {
	case "setup":
		tool := req.Tool
		if fw.Tool != "" {
			return "", "", fmt.Errorf("这台主机已经在用 %s 了", fw.Tool)
		}
		if tool == "" {
			tool = fw.Recommended
		}
		switch tool {
		case "ufw":
			if !fw.UFWInstalled {
				if st.Family != "debian" {
					return "", "", errors.New("只在 Debian / Ubuntu 上一键安装 ufw")
				}
				b.WriteString(step("apt-get install -y -q ufw", "安装 ufw 失败（可以先在主机上执行 apt-get update）"))
			}
		case "firewalld":
			if !fw.FWDInstalled {
				if st.Family != "rhel" {
					return "", "", errors.New("只在 RHEL / Rocky / Alma / Fedora 上一键安装 firewalld")
				}
				b.WriteString(step("dnf install -y -q firewalld", "安装 firewalld 失败"))
			}
		default:
			return "", "", errors.New("这台主机的系统不支持一键安装防火墙，请在主机上自己安装 ufw 或 firewalld")
		}
		// 先放行（SSH 一定在里面），最后才启用：任何一步失败都不会启用，不会把自己锁在外面。
		ports := mergePorts(keep, req.Ports)
		for _, p := range ports {
			if tool == "ufw" {
				b.WriteString(step("ufw allow "+p.String(), "放行 "+p.String()+" 失败，没有启用防火墙"))
			} else {
				b.WriteString(step("firewall-offline-cmd --add-port="+p.String(), "放行 "+p.String()+" 失败，没有启用防火墙"))
			}
		}
		if tool == "ufw" {
			b.WriteString(step("ufw --force enable", "启用 ufw 失败"))
			b.WriteString(asRoot("ufw status verbose"))
		} else {
			b.WriteString(step("systemctl enable --now firewalld", "启用 firewalld 失败"))
			b.WriteString(asRoot("firewall-cmd --list-all"))
		}
		return b.String(), tool, nil
	case "allow", "remove":
		if fw.Tool == "" {
			return "", "", errors.New("这台主机还没有启用防火墙：先安装并启用")
		}
		if len(req.Ports) == 0 {
			return "", "", errors.New("没有选端口")
		}
		for _, p := range req.Ports {
			if req.Action == "remove" {
				for _, k := range keep {
					if k == p {
						return "", "", fmt.Errorf("%s 是 SSH 或面板在用的端口，关掉会把你锁在外面", p.String())
					}
				}
			}
			switch {
			case fw.Tool == "ufw" && req.Action == "allow":
				b.WriteString(step("ufw allow "+p.String(), "放行 "+p.String()+" 失败"))
			case fw.Tool == "ufw":
				b.WriteString(step("ufw delete allow "+p.String(), "删除 "+p.String()+" 的放行规则失败"))
			case req.Action == "allow":
				b.WriteString(step("firewall-cmd --permanent --add-port="+p.String(), "放行 "+p.String()+" 失败"))
			default:
				b.WriteString(step("firewall-cmd --permanent --remove-port="+p.String(), "删除 "+p.String()+" 的放行规则失败"))
			}
		}
		if fw.Tool == "firewalld" {
			b.WriteString(step("firewall-cmd --reload", "重新加载 firewalld 失败"))
		}
		if fw.Tool == "ufw" {
			b.WriteString(asRoot("ufw status verbose"))
		} else {
			b.WriteString(asRoot("firewall-cmd --list-all"))
		}
		return b.String(), fw.Tool, nil
	}
	return "", "", errors.New("未知的操作")
}

func mergePorts(a, b []Port) []Port {
	seen := map[Port]bool{}
	var out []Port
	for _, p := range append(append([]Port{}, a...), b...) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// ---------- Caddy 站点 ----------

var (
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(:[0-9]{1,5})?$`)
	ipPortRe = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,3}){3}:[0-9]{1,5}$`)
)

// ValidAddress 校验站点地址：域名（可带端口；Caddy 自动申请证书），或者 IP:端口（用 Caddy 自签的证书）。
func ValidAddress(addr string) (ip bool, err error) {
	addr = strings.ToLower(addr)
	switch {
	case ipPortRe.MatchString(addr):
		host, port, _ := net.SplitHostPort(addr)
		n, _ := strconv.Atoi(port)
		if net.ParseIP(host) == nil || n < 1 || n > 65535 {
			return false, errors.New("地址不合法")
		}
		return true, nil
	case domainRe.MatchString(addr):
		if _, port, err := net.SplitHostPort(addr); err == nil {
			if n, _ := strconv.Atoi(port); n < 1 || n > 65535 {
				return false, errors.New("端口要在 1 到 65535 之间")
			}
		}
		return false, nil
	}
	return false, errors.New("地址要是域名（例如 app.example.com，可以带端口），或者 IP:端口")
}

func siteFile(addr string) string {
	return SitesDir + "/" + strings.ReplaceAll(strings.ToLower(addr), ":", "_") + ".caddy"
}

// pipeRoot 把标准输入交给一条以 root 身份运行的命令（同 asRoot）。
func pipeRoot(input, cmd string) string {
	return input + " | " + asRoot(cmd)
}

// SiteAddCommand 渲染“添加站点”：写一个站点文件，确保主配置 import 了站点目录，校验通过才重载 Caddy；校验不过就撤下这个站点。
func SiteAddCommand(st Status, addr string, port int) (string, error) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	ip, err := ValidAddress(addr)
	if err != nil {
		return "", err
	}
	if port < 1 || port > 65535 {
		return "", errors.New("转发到的端口要在 1 到 65535 之间")
	}
	if !st.Caddy.Installed {
		return "", errors.New("这台主机还没有装 Caddy")
	}
	if !st.CanRoot() {
		return "", st.rootErr("改不了 Caddy 的配置")
	}
	upstream := "127.0.0.1:" + strconv.Itoa(port)
	lines := []string{siteMarker + addr + " " + upstream, addr + " {"}
	if ip {
		lines = append(lines, "\ttls internal")
	}
	lines = append(lines, "\treverse_proxy "+upstream, "}")
	file := siteFile(addr)
	var printf strings.Builder
	printf.WriteString("printf '%s\\n'")
	for _, l := range lines {
		printf.WriteString(" " + snippet.Quote(l))
	}
	var b strings.Builder
	b.WriteString(step("mkdir -p "+SitesDir, "创建 "+SitesDir+" 失败"))
	b.WriteString(`grep -qs '^import ` + SitesDir + `/' ` + Caddyfile + ` || { ` +
		pipeRoot(`printf '\n%s\n' 'import `+SitesDir+`/*.caddy'`, "tee -a "+Caddyfile+" >/dev/null") +
		`; } || { echo 'hostower: 改不了 ` + Caddyfile + `'; exit 1; }; `)
	b.WriteString("{ " + pipeRoot(printf.String(), "tee "+file+" >/dev/null") + "; } || { echo 'hostower: 写站点配置失败'; exit 1; }; ")
	b.WriteString("{ " + asRoot("caddy validate --config "+Caddyfile+" --adapter caddyfile") + "; } || { " +
		asRoot("mv -f "+file+" "+file+".rejected") + "; echo 'hostower: Caddy 配置校验没通过，已撤下这个站点（地址重复或者写法不对）'; exit 1; }; ")
	b.WriteString(step("systemctl reload caddy", "重载 Caddy 失败"))
	b.WriteString(`echo ` + snippet.Quote("hostower: 站点 "+addr+" → "+upstream+" 已生效"))
	return b.String(), nil
}

// SiteRemoveCommand 渲染“删除站点”：把站点文件挪成 .removed（不删，想恢复改回名字就行），再重载 Caddy。
func SiteRemoveCommand(st Status, addr string) (string, error) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if _, err := ValidAddress(addr); err != nil {
		return "", err
	}
	found := false
	for _, s := range st.Sites {
		if s.Address == addr {
			found = true
		}
	}
	if !found {
		return "", errors.New("没有这个站点（只能删 Hostower 添加的站点）")
	}
	file := siteFile(addr)
	return step("mv -f "+file+" "+file+".removed", "撤下站点失败") + step("systemctl reload caddy", "重载 Caddy 失败") +
		`echo ` + snippet.Quote("hostower: 站点 "+addr+" 已撤下"), nil
}

// CaddyInstallCommand 渲染“安装 Caddy”：用系统软件源（Debian/Ubuntu、Fedora 等有现成的包），装好后开机自启。
func CaddyInstallCommand(st Status) (string, error) {
	if st.Caddy.Installed {
		return "", errors.New("Caddy 已经装好了")
	}
	if !st.CanRoot() {
		return "", st.rootErr("装不了 Caddy")
	}
	var b strings.Builder
	switch st.Family {
	case "debian":
		b.WriteString(step("apt-get install -y -q caddy", "安装 Caddy 失败：系统软件源里可能没有它，请按 Caddy 官网的说明添加它的软件源后再装"))
	case "rhel":
		b.WriteString(step("dnf install -y -q caddy", "安装 Caddy 失败：RHEL 系通常要先启用 EPEL 或 Caddy 的 COPR 源"))
	default:
		return "", errors.New("这台主机的系统不支持一键安装 Caddy，请按 Caddy 官网的说明安装")
	}
	b.WriteString(step("systemctl enable --now caddy", "启动 Caddy 失败"))
	b.WriteString(`echo 'hostower: Caddy 已安装并启动'`)
	return b.String(), nil
}
