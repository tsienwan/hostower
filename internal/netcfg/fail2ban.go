package netcfg

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/snippet"
)

// ---------- Fail2ban ----------
//
// Fail2ban 装在宿主机上，它按日志里的失败次数往防火墙里加封禁规则，所以这里的改动在判级里都是 L2：
// 安装、改设置、解封都要在网页上批准并重新验证身份。
// Hostower 只写一个文件（F2BFile），第一行是标记，检测脚本读它就知道当前的设置；主机上原有的 jail.local 等不动。
// 两条底线：
//   - 不把自己封掉：白名单里固定有 Hostower 连这台主机用的来源地址（SSH_CONNECTION）和本机回环地址。
//   - 先写配置再装包、写完先 fail2ban-client -t 检查：配置不对就恢复原样，不带着坏配置重启。

// F2BFile 是 Hostower 写的 Fail2ban 配置。
const F2BFile = "/etc/fail2ban/jail.d/hostower.local"

const f2bMarker = "# hostower-f2b "

// F2BJails 是 Hostower 能开关的 jail：sshd，以及装了 Nginx 时的两个 Web 防护。
var F2BJails = []string{"sshd", "nginx-http-auth", "nginx-botsearch"}

// Jail 是一个正在运行的 jail 的状态。
type Jail struct {
	Name        string   `json:"name"`
	Failed      int      `json:"failed"`
	TotalFailed int      `json:"totalFailed"`
	Banned      int      `json:"banned"`
	TotalBanned int      `json:"totalBanned"`
	IPs         []string `json:"ips"`
}

// F2BConfig 是 Hostower 写的设置。
type F2BConfig struct {
	BanTime  int      `json:"bantime"`  // 封禁多久（秒）
	FindTime int      `json:"findtime"` // 在多长时间里统计失败次数（秒）
	MaxRetry int      `json:"maxretry"` // 失败几次就封
	Ignore   []string `json:"ignore"`   // 白名单（除了 Hostower 自己的来源地址和回环地址之外）
	Jails    []string `json:"jails"`    // 启用的 jail
}

// Fail2ban 是 Fail2ban 的现状。
type Fail2ban struct {
	Installed   bool `json:"installed"`
	Active      bool `json:"active"`
	Installable bool `json:"installable"`
	// Unknown 表示服务在运行，但登录账号读不出 jail 的状态（没有免密 sudo）。
	Unknown bool       `json:"unknown"`
	Jails   []Jail     `json:"jails"`
	Config  *F2BConfig `json:"config"` // 没有 Hostower 写的设置时为空
	// AuthLog 表示有 /var/log/auth.log 或 /var/log/secure；都没有时 sshd 要从 systemd 的日志里读。
	AuthLog bool `json:"authLog"`
}

// DefaultF2B 是第一次安装时的设置：10 分钟内失败 5 次，封 1 小时。
func DefaultF2B() F2BConfig {
	return F2BConfig{BanTime: 3600, FindTime: 600, MaxRetry: 5, Ignore: []string{}, Jails: []string{"sshd"}}
}

func countAfter(l, key string) (int, bool) {
	if i := strings.Index(l, key); i >= 0 {
		n, err := strconv.Atoi(strings.TrimSpace(l[i+len(key):]))
		return n, err == nil
	}
	return 0, false
}

func parseFail2ban(installed bool, sec map[string][]string) Fail2ban {
	f := Fail2ban{Installed: installed, Jails: []Jail{}}
	f.Active = len(sec["f2b"]) > 0 && strings.TrimSpace(sec["f2b"][0]) == "active"
	var cur *Jail
	for _, l := range sec["f2b-jails"] {
		if name, ok := strings.CutPrefix(strings.TrimSpace(l), "-- "); ok {
			f.Jails = append(f.Jails, Jail{Name: strings.TrimSpace(name), IPs: []string{}})
			cur = &f.Jails[len(f.Jails)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if n, ok := countAfter(l, "Currently failed:"); ok {
			cur.Failed = n
		} else if n, ok := countAfter(l, "Total failed:"); ok {
			cur.TotalFailed = n
		} else if n, ok := countAfter(l, "Currently banned:"); ok {
			cur.Banned = n
		} else if n, ok := countAfter(l, "Total banned:"); ok {
			cur.TotalBanned = n
		} else if i := strings.Index(l, "Banned IP list:"); i >= 0 {
			for _, ip := range strings.Fields(l[i+len("Banned IP list:"):]) {
				if net.ParseIP(ip) != nil {
					cur.IPs = append(cur.IPs, ip)
				}
			}
		}
	}
	f.Unknown = f.Active && len(f.Jails) == 0
	for _, l := range sec["f2b-conf"] {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(l), f2bMarker); ok {
			c := F2BConfig{Ignore: []string{}, Jails: []string{}}
			for _, kv := range strings.Fields(rest) {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "bantime":
					c.BanTime, _ = strconv.Atoi(v)
				case "findtime":
					c.FindTime, _ = strconv.Atoi(v)
				case "maxretry":
					c.MaxRetry, _ = strconv.Atoi(v)
				case "ignoreip":
					c.Ignore = splitList(v)
				case "jails":
					c.Jails = splitList(v)
				}
			}
			f.Config = &c
		}
	}
	f.AuthLog = len(sec["authlog"]) > 0
	return f
}

func splitList(v string) []string {
	out := []string{}
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

// validIPOrNet 校验并规整一个 IP 或网段。
func validIPOrNet(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if ip := net.ParseIP(s); ip != nil {
		return ip.String(), true
	}
	if _, n, err := net.ParseCIDR(s); err == nil {
		return n.String(), true
	}
	return "", false
}

func (c *F2BConfig) validate(st Status) error {
	switch {
	case c.BanTime < 60 || c.BanTime > 365*86400:
		return errors.New("封禁时长要在 1 分钟到 365 天之间")
	case c.FindTime < 60 || c.FindTime > 7*86400:
		return errors.New("统计时间窗口要在 1 分钟到 7 天之间")
	case c.MaxRetry < 1 || c.MaxRetry > 100:
		return errors.New("失败次数要在 1 到 100 之间")
	case len(c.Ignore) > 50:
		return errors.New("白名单最多 50 条")
	}
	var ignore []string
	seen := map[string]bool{}
	for _, x := range c.Ignore {
		v, ok := validIPOrNet(x)
		if !ok {
			return errors.New("白名单里的 " + x + " 不是 IP 地址或网段")
		}
		if !seen[v] {
			seen[v] = true
			ignore = append(ignore, v)
		}
	}
	c.Ignore = append([]string{}, ignore...)
	var jails []string
	for _, want := range F2BJails {
		for _, j := range c.Jails {
			if j == want {
				if strings.HasPrefix(j, "nginx-") && !st.Nginx.Installed {
					return errors.New("这台主机没有装 Nginx，开不了 Web 防护")
				}
				jails = append(jails, j)
				break
			}
		}
	}
	for _, j := range c.Jails {
		if !contains(F2BJails, j) {
			return errors.New("不认识的防护项：" + j)
		}
	}
	c.Jails = append([]string{}, jails...)
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// f2bConfig 渲染 F2BFile 的内容。
func (st Status) f2bConfig(c F2BConfig) string {
	ignore := []string{"127.0.0.1/8", "::1"}
	if st.ClientIP != "" {
		ignore = append(ignore, st.ClientIP)
	}
	for _, x := range c.Ignore {
		if !contains(ignore, x) {
			ignore = append(ignore, x)
		}
	}
	var b strings.Builder
	b.WriteString(f2bMarker + "bantime=" + strconv.Itoa(c.BanTime) + " findtime=" + strconv.Itoa(c.FindTime) + " maxretry=" + strconv.Itoa(c.MaxRetry) +
		" ignoreip=" + strings.Join(c.Ignore, ",") + " jails=" + strings.Join(c.Jails, ",") + "\n")
	b.WriteString("# 这个文件由 Hostower 生成：请在 Hostower 的“网络”页里改，手改会被覆盖。\n")
	b.WriteString("[DEFAULT]\nbantime = " + strconv.Itoa(c.BanTime) + "\nfindtime = " + strconv.Itoa(c.FindTime) + "\nmaxretry = " + strconv.Itoa(c.MaxRetry) + "\n")
	b.WriteString("ignoreip = " + strings.Join(ignore, " ") + "\n")
	on := func(j string) string {
		if contains(c.Jails, j) {
			return "true"
		}
		return "false"
	}
	b.WriteString("\n[sshd]\nenabled = " + on("sshd") + "\nport = " + strconv.Itoa(st.SSHPort) + "\n")
	if !st.AuthLogged() {
		b.WriteString("backend = systemd\n")
	}
	if st.Nginx.Installed {
		b.WriteString("\n[nginx-http-auth]\nenabled = " + on("nginx-http-auth") + "\nport = http,https\nlogpath = /var/log/nginx/*error.log\n")
		b.WriteString("\n[nginx-botsearch]\nenabled = " + on("nginx-botsearch") + "\nport = http,https\nlogpath = /var/log/nginx/*access.log\n")
	}
	return b.String()
}

// AuthLogged 报告 sshd 的登录日志是不是写在文件里（不是的话 Fail2ban 要从 systemd 的日志读）。
func (st Status) AuthLogged() bool { return st.Fail2ban.AuthLog }

// F2BRequest 是一次 Fail2ban 操作。
type F2BRequest struct {
	Action string    `json:"action"` // install | config | unban
	Config F2BConfig `json:"config"`
	Jail   string    `json:"jail"`
	IP     string    `json:"ip"`
}

var jailNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// F2BCommand 渲染 Fail2ban 操作。
func F2BCommand(st Status, req F2BRequest) (string, error) {
	f := st.Fail2ban
	if !st.CanRoot() {
		return "", st.rootErr("管不了 Fail2ban")
	}
	switch req.Action {
	case "install", "config":
		c := req.Config
		if err := c.validate(st); err != nil {
			return "", err
		}
		conf := st.f2bConfig(c)
		var b strings.Builder
		if req.Action == "install" {
			if f.Installed {
				return "", errors.New("Fail2ban 已经装好了")
			}
			pkgs := "fail2ban"
			if st.Family == "debian" && !st.AuthLogged() {
				pkgs += " python3-systemd"
			}
			in, err := installPkgs(st, pkgs, "安装 Fail2ban 失败")
			if err != nil {
				return "", err
			}
			// 先写好配置再装包：装好后服务第一次启动就带着白名单，不会先按默认设置跑起来。
			b.WriteString(step("mkdir -p /etc/fail2ban/jail.d", "创建 /etc/fail2ban/jail.d 失败"))
			b.WriteString(writeRoot(F2BFile, conf, "写 Fail2ban 配置失败"))
			b.WriteString(in)
			b.WriteString(step("fail2ban-client -t", "Fail2ban 配置检查没通过"))
			b.WriteString(step("systemctl enable fail2ban", "设置 Fail2ban 开机自启失败"))
			b.WriteString(step("systemctl restart fail2ban", "启动 Fail2ban 失败"))
		} else {
			if !f.Installed {
				return "", errors.New("这台主机还没有装 Fail2ban")
			}
			bak := F2BFile + ".bak"
			// 改之前留一份旧的；检查不过就换回去，服务照旧运行。
			if f.Config != nil {
				b.WriteString(step("cp -p "+F2BFile+" "+bak, "备份旧配置失败"))
			}
			b.WriteString(writeRoot(F2BFile, conf, "写 Fail2ban 配置失败"))
			restore := asRoot("mv -f " + F2BFile + " " + F2BFile + ".rejected")
			if f.Config != nil {
				restore = asRoot("mv -f " + bak + " " + F2BFile)
			}
			b.WriteString("{ " + asRoot("fail2ban-client -t") + "; } || { " + restore + "; echo 'hostower: Fail2ban 配置检查没通过，已恢复原来的设置'; exit 1; }; ")
			if f.Active {
				b.WriteString(step("fail2ban-client reload", "重新加载 Fail2ban 失败"))
			} else {
				b.WriteString(step("systemctl enable fail2ban", "设置 Fail2ban 开机自启失败"))
				b.WriteString(step("systemctl restart fail2ban", "启动 Fail2ban 失败"))
			}
		}
		b.WriteString(asRoot("fail2ban-client status"))
		return b.String(), nil
	case "unban":
		ip, ok := validIPOrNet(req.IP)
		if !ok || strings.Contains(ip, "/") {
			return "", errors.New("要解封的不是 IP 地址")
		}
		found := false
		for _, j := range f.Jails {
			if j.Name == req.Jail {
				found = true
			}
		}
		if !jailNameRe.MatchString(req.Jail) || !found {
			return "", errors.New("没有这个 jail")
		}
		return step("fail2ban-client set "+req.Jail+" unbanip "+ip, "解封失败") +
			"echo " + snippet.Quote("hostower: 已在 "+req.Jail+" 里解封 "+ip), nil
	}
	return "", errors.New("未知的操作")
}

// F2BLogCommand 是看 Fail2ban 日志的只读命令。
// 有的系统让 Fail2ban 只写 systemd 的日志，读不到文件时就看它。
const F2BLogCommand = "{ tail -n 200 /var/log/fail2ban.log 2>/dev/null || sudo -n tail -n 200 /var/log/fail2ban.log 2>/dev/null; } || journalctl -u fail2ban -n 200 --no-pager 2>/dev/null || sudo -n journalctl -u fail2ban -n 200 --no-pager 2>&1"
