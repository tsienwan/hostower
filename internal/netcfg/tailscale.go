package netcfg

import (
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/snippet"
)

// ---------- Tailscale ----------
//
// Tailscale 把几台主机连进同一个加密的内网（每台有一个 100.x 的地址），主机之间互相访问不用把端口对公网开放。
// 它决定这台主机在哪个内网里、能被谁连上，所以这里的改动在判级里都是 L2：安装、加入、改设置、断开、退出登录
// 都要在网页上批准并重新验证身份。
// 三条底线：
//   - 授权密钥不进命令：密钥存在塔台的应用密钥里，执行时从标准输入送给 tailscale（--auth-key=file:/dev/stdin），
//     命令、请求、审计里只有引用名。不想存密钥就不选，命令会打印一个登录链接，等人在浏览器里登录。
//   - 不把自己断开：Hostower 是经 Tailscale 连到这台主机的（来源地址在 Tailscale 的网段里）时，不提供断开和退出登录。
//   - 安装只用官方软件源（pkgs.tailscale.com）：把源和它的签名密钥放到系统的标准位置，再用包管理器装。

// TSOptions 是 Hostower 管的几项 Tailscale 设置。
type TSOptions struct {
	// Hostname 是这台机器在 Tailscale 网络里的名字；空表示用系统的主机名。
	Hostname string `json:"hostname"`
	// AcceptRoutes：接受别的节点通告的子网路由。
	AcceptRoutes bool `json:"acceptRoutes"`
	// AcceptDNS：使用 Tailscale 网络的 DNS 设置（MagicDNS）。
	AcceptDNS bool `json:"acceptDns"`
	// SSH：开启 Tailscale SSH（网络里被 ACL 允许的人可以直接登录这台主机，不经过 sshd 的密钥）。
	SSH bool `json:"ssh"`
}

// Tailscale 是 Tailscale 的现状。
type Tailscale struct {
	Installed   bool `json:"installed"`
	Active      bool `json:"active"` // tailscaled 服务在运行
	Installable bool `json:"installable"`
	// State 是后端状态：Running（已连上）| NeedsLogin（还没登录）| Stopped（已登录但断开了）| Starting | ""（读不出来）
	State   string   `json:"state"`
	IPs     []string `json:"ips"`     // 这台主机在 Tailscale 网络里的地址
	Name    string   `json:"name"`    // 在网络里的名字
	DNSName string   `json:"dnsName"` // MagicDNS 名字（不带结尾的点）
	Tailnet string   `json:"tailnet"` // 所在的网络
	Version string   `json:"version"`
	// Options 是当前的设置；读不出来时为空。
	Options *TSOptions `json:"options"`
	// ViaTailscale 表示 Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把自己断在外面。
	ViaTailscale bool `json:"viaTailscale"`
}

var (
	tsNameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	// tsNets 是 Tailscale 分配地址用的网段。
	tsNets = func() []*net.IPNet {
		var out []*net.IPNet
		for _, c := range []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"} { // hygiene:allow（Tailscale 固定用的网段，不是真实主机）
			_, n, _ := net.ParseCIDR(c)
			out = append(out, n)
		}
		return out
	}()
)

// InTailnet 判断一个地址是不是 Tailscale 分配的地址。
func InTailnet(addr string) bool {
	ip := net.ParseIP(strings.TrimSpace(addr))
	if ip == nil {
		return false
	}
	for _, n := range tsNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clip 把远端给的一段文字裁到合理的长度，去掉控制字符：这些值只用来显示。
func clip(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > max {
		s = s[:max]
	}
	return s
}

func parseTailscale(installed bool, sec map[string][]string, clientIP string) Tailscale {
	t := Tailscale{Installed: installed, IPs: []string{}, ViaTailscale: InTailnet(clientIP)}
	t.Active = len(sec["ts"]) > 0 && unitRunning(sec["ts"][0])
	var st struct {
		Version      string
		BackendState string
		TailscaleIPs []string
		Self         struct {
			HostName string
			DNSName  string
		}
		CurrentTailnet *struct{ Name string }
	}
	if raw := strings.Join(sec["ts-status"], "\n"); strings.TrimSpace(raw) != "" && json.Unmarshal([]byte(raw), &st) == nil {
		switch st.BackendState {
		case "Running", "NeedsLogin", "Stopped", "Starting", "NeedsMachineAuth":
			t.State = st.BackendState
		}
		for _, ip := range st.TailscaleIPs {
			if p := net.ParseIP(ip); p != nil {
				t.IPs = append(t.IPs, p.String())
			}
		}
		t.Name = clip(st.Self.HostName, 63)
		t.DNSName = clip(strings.TrimSuffix(st.Self.DNSName, "."), 253)
		if st.CurrentTailnet != nil {
			t.Tailnet = clip(st.CurrentTailnet.Name, 120)
		}
		t.Version = clip(strings.SplitN(st.Version, "-", 2)[0], 32)
	}
	var prefs struct {
		RouteAll *bool
		CorpDNS  *bool
		RunSSH   *bool
		Hostname string
	}
	if raw := strings.Join(sec["ts-prefs"], "\n"); strings.TrimSpace(raw) != "" && json.Unmarshal([]byte(raw), &prefs) == nil && prefs.CorpDNS != nil {
		o := TSOptions{AcceptDNS: *prefs.CorpDNS}
		if prefs.RouteAll != nil {
			o.AcceptRoutes = *prefs.RouteAll
		}
		if prefs.RunSSH != nil {
			o.SSH = *prefs.RunSSH
		}
		if tsNameRe.MatchString(prefs.Hostname) {
			o.Hostname = prefs.Hostname
		}
		t.Options = &o
	}
	return t
}

// TSRequest 是一次 Tailscale 操作。
type TSRequest struct {
	Action  string    `json:"action"` // install | up | set | down | logout
	Options TSOptions `json:"options"`
}

// tsFlags 把设置写成 tailscale 的参数。每一项都写明，结果不依赖主机上原来的设置。
func tsFlags(o TSOptions) string {
	return "--hostname=" + o.Hostname + " --accept-routes=" + strconv.FormatBool(o.AcceptRoutes) +
		" --accept-dns=" + strconv.FormatBool(o.AcceptDNS) + " --ssh=" + strconv.FormatBool(o.SSH)
}

// tsFetch 把官方软件源里的一个文件下载到临时文件，再以 root 身份放到系统目录里。
// 先落到临时文件：下载失败时不会把一个空文件写进系统目录。
func tsFetch(url, dest, fail string) string {
	return `{ t=$(mktemp) && curl -fsSL -o "$t" "` + url + `" && { ` + asRoot(`install -m 0644 "$t" `+dest) + `; }; } || { rm -f "$t"; echo ` +
		snippet.Quote("hostower: "+fail) + `; exit 1; }; rm -f "$t"; `
}

const tsPkgs = "https://pkgs.tailscale.com/stable/"

// tsInstall 渲染“安装 Tailscale”：加官方软件源，装包，启动服务。装完还没有登录，要再“加入网络”。
func tsInstall(st Status) (string, error) {
	var b strings.Builder
	switch st.Family {
	case "debian":
		// 软件源按发行版和版本代号分目录；Ubuntu 的衍生版用 Ubuntu 的代号，其余的用 Debian 的。
		b.WriteString(`. /etc/os-release; os=debian; ver="${DEBIAN_CODENAME:-$VERSION_CODENAME}"; ` +
			`if [ "$ID" = ubuntu ] || [ -n "$UBUNTU_CODENAME" ]; then os=ubuntu; ver="${UBUNTU_CODENAME:-$VERSION_CODENAME}"; fi; ` +
			`case "$ver" in ''|*[!a-z]*) echo 'hostower: 认不出系统的版本代号，请按 Tailscale 官网的说明安装'; exit 1;; esac; `)
		in, err := installPkgs(st, "curl ca-certificates", "安装 curl 失败")
		if err != nil {
			return "", err
		}
		b.WriteString("command -v curl >/dev/null 2>&1 || " + strings.TrimSuffix(in, "; ") + "; ")
		b.WriteString(tsFetch(tsPkgs+`$os/$ver.noarmor.gpg`, "/usr/share/keyrings/tailscale-archive-keyring.gpg", "下载 Tailscale 软件源的签名密钥失败（这台主机连不上 pkgs.tailscale.com，或者它不支持这个系统版本）"))
		b.WriteString(tsFetch(tsPkgs+`$os/$ver.tailscale-keyring.list`, "/etc/apt/sources.list.d/tailscale.list", "下载 Tailscale 的软件源配置失败"))
		b.WriteString(step("apt-get update -q", "更新软件源失败"))
		b.WriteString(step("apt-get install -y -q tailscale", "安装 Tailscale 失败"))
	case "rhel":
		b.WriteString(`. /etc/os-release; maj="${VERSION_ID%%.*}"; case "$ID" in rhel|centos) repo="$ID/$maj";; ol) repo="oracle/$maj";; amzn) repo="amazon-linux/$VERSION_ID";; *) repo=fedora;; esac; ` +
			`case "$repo" in *[!a-z0-9/.-]*|*..*) echo 'hostower: 认不出系统的版本，请按 Tailscale 官网的说明安装'; exit 1;; esac; `)
		b.WriteString(tsFetch(tsPkgs+`$repo/tailscale.repo`, "/etc/yum.repos.d/tailscale.repo", "下载 Tailscale 的软件源配置失败（这台主机连不上 pkgs.tailscale.com，或者它不支持这个系统版本）"))
		b.WriteString(step("dnf install -y -q tailscale", "安装 Tailscale 失败"))
	default:
		return "", errors.New("这台主机的系统不支持一键安装 Tailscale，请按 Tailscale 官网的说明安装")
	}
	b.WriteString(step("systemctl enable --now tailscaled", "启动 Tailscale 服务失败"))
	b.WriteString(`echo 'hostower: Tailscale 已安装，还没有登录：回到网络页点“加入网络”'`)
	return b.String(), nil
}

// TSCommand 渲染 Tailscale 操作。withKey 表示加入网络时用授权密钥（从标准输入送进来）；
// 不用密钥时命令会打印登录链接并等人登录，最多等 9 分钟（请求本身 10 分钟超时）。
func TSCommand(st Status, req TSRequest, withKey bool) (string, error) {
	t := st.Tailscale
	if !st.CanRoot() {
		return "", st.rootErr("管不了 Tailscale")
	}
	if req.Action == "install" {
		if t.Installed {
			return "", errors.New("Tailscale 已经装好了")
		}
		return tsInstall(st)
	}
	if !t.Installed {
		return "", errors.New("这台主机还没有装 Tailscale")
	}
	o := req.Options
	o.Hostname = strings.TrimSpace(o.Hostname)
	if o.Hostname != "" && !tsNameRe.MatchString(o.Hostname) {
		return "", errors.New("Tailscale 里的名字只能用字母、数字和连字符，不能以连字符开头或结尾，最长 63 个字符")
	}
	done := func(msg string) string { return `echo ` + snippet.Quote("hostower: "+msg) }
	switch req.Action {
	case "up":
		if t.State == "Running" {
			return "", errors.New("这台主机已经在 Tailscale 网络里了：要改设置用“设置”")
		}
		var b strings.Builder
		if !t.Active {
			b.WriteString(step("systemctl enable --now tailscaled", "启动 Tailscale 服务失败"))
		}
		if withKey {
			b.WriteString(step("tailscale up --reset --auth-key=file:/dev/stdin "+tsFlags(o)+" --timeout=60s",
				"加入 Tailscale 网络失败：授权密钥可能过期、用过了，或者不属于这个网络"))
		} else {
			b.WriteString(`echo 'hostower: 下面会出现一个登录链接：在浏览器里打开它并登录 Tailscale 账号，这条请求会一直等到登录完成（最多 9 分钟）'; `)
			b.WriteString(step("tailscale up --reset "+tsFlags(o)+" --timeout=540s", "加入 Tailscale 网络失败：9 分钟内没有完成登录，或者被拒绝了"))
		}
		b.WriteString("tailscale ip -4 2>/dev/null; " + done("已加入 Tailscale 网络，上一行是这台主机在网络里的地址"))
		return b.String(), nil
	case "set":
		if t.State != "Running" && t.State != "Stopped" {
			return "", errors.New("这台主机还没有登录 Tailscale：先“加入网络”")
		}
		return step("tailscale set "+tsFlags(o), "修改 Tailscale 设置失败") + done("Tailscale 设置已修改"), nil
	case "down", "logout":
		if t.ViaTailscale {
			return "", errors.New("Hostower 就是经 Tailscale 连到这台主机的：断开或退出登录会把它自己断在外面。先把这台主机的地址改成不经过 Tailscale 的，再来操作")
		}
		if req.Action == "down" {
			if t.State != "Running" {
				return "", errors.New("这台主机现在没有连在 Tailscale 网络里")
			}
			return step("tailscale down", "断开 Tailscale 失败") + done("已断开 Tailscale（登录状态还在，随时可以重新连上）"), nil
		}
		if t.State == "NeedsLogin" || t.State == "" {
			return "", errors.New("这台主机没有登录 Tailscale")
		}
		return step("tailscale logout", "退出 Tailscale 登录失败") + done("已退出 Tailscale 登录，这台主机不再属于那个网络"), nil
	}
	return "", errors.New("未知的操作")
}
