// Package speedtest 给主机测网速：延迟、下载、上传。有两种方式：Speedtest by Ookla（默认，见 ookla.go）和 Cloudflare。
//
// 它不碰主机，只做两件事：生成一条固定的测速命令（由网关像其他命令一样交给内核：判级、审批、审计都照常），
// 以及从内核的事件里听这类请求的结束，把每台主机的输出解析成数字存起来。
// 测速会从主机向测速服务（Ookla 的测速服务器，或者 Cloudflare 的 speed.cloudflare.com）下载和上传数据，所以会消耗流量；
// 它被判为 L1，需要批准。
package speedtest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
)

// Tag 写在请求的 Snippet 字段里，用来认出测速请求（Snippet 的名字里不会有冒号，伪造不了）。
const Tag = "builtin:speedtest@1"

// Server 是测速用的服务。
const Server = "speed.cloudflare.com"

// Sizes 是可选的下载量（MB）；上传量是它的四分之一。
var Sizes = []int{10, 25, 100}

// installManagers 是自动安装软件包时按顺序尝试的包管理器和安装命令，{pkg} 换成包名。
var installManagers = [][2]string{
	{"apt-get", "{ apt-get install -y -q {pkg} || { apt-get update -q && apt-get install -y -q {pkg}; }; }"},
	{"dnf", "dnf install -y -q {pkg}"},
	{"yum", "yum install -y -q {pkg}"},
	{"apk", "apk add -q {pkg}"},
	{"zypper", "zypper -n -q install {pkg}"},
	{"pacman", "pacman -S --noconfirm --needed {pkg}"},
}

// installTree 是按包管理器装一个包的那串 if/elif：root 直接装，否则用 sudo -n（需要免密 sudo，不会卡住等密码）。
// 每个分支都写成明确的命令（不用变量拼命令名），判级能看清楚它在做什么：变更软件包，L1。
// 都没有时执行 otherwise。
func installTree(pkg, otherwise string) string {
	var b strings.Builder
	for i, m := range installManagers {
		if i == 0 {
			b.WriteString("if")
		} else {
			b.WriteString(" elif")
		}
		cmd := strings.ReplaceAll(m[1], "{pkg}", pkg)
		sudo := strings.ReplaceAll(cmd, m[0]+" ", "sudo -n "+m[0]+" ")
		fmt.Fprintf(&b, " command -v %s >/dev/null 2>&1; then if [ \"$(id -u)\" = 0 ]; then %s; else %s; fi", m[0], cmd, sudo)
	}
	b.WriteString("; else " + otherwise + "; fi; ")
	return b.String()
}

// installScript 是“没有 curl 就装一个”的那段；装不上就报错退出（测不了速）。
// trace 为真时再加一段“traceroute 和 tracepath 都没有就装 traceroute”；这段装不上不影响测速，只是没有线路。
func installScript(trace bool) string {
	var b strings.Builder
	b.WriteString("if ! command -v curl >/dev/null 2>&1; then echo 'speedtest installing curl'; ")
	b.WriteString(installTree("curl", "echo 'speedtest error=no-package-manager'; exit 0"))
	b.WriteString("if ! command -v curl >/dev/null 2>&1; then echo 'speedtest error=install-failed'; exit 0; fi; fi; ")
	if trace {
		b.WriteString("if ! command -v traceroute >/dev/null 2>&1 && ! command -v tracepath >/dev/null 2>&1; then echo 'speedtest installing traceroute'; ")
		b.WriteString(installTree("traceroute", "echo 'speedtest warn=no-package-manager'"))
		b.WriteString("fi; ")
	}
	return b.String()
}

// CommandWith 生成测速命令。每台主机上跑同一条；只输出以 "speedtest " 开头的几行：
// 先向测速服务要一份 trace（出口 IP、节点、HTTP 和 TLS 版本），再连五次量延迟（取扣掉域名解析后的 TCP 握手，最快的一次），
// 然后对每个追踪目标跑一次路由追踪（traceroute 先用 ICMP、不行再用 UDP——很多网络不回 UDP 探测；没有 traceroute 就用 tracepath），最后下载和上传各一次，记下传输量、用时和首字节时间。
// 主机上需要有 curl；没有 traceroute 和 tracepath 时只是少了线路那一段。
// install 为 true 时前面多一段：没有 curl 就先用系统的包管理器装上；有追踪目标时，traceroute 和 tracepath 都没有也装上 traceroute。
// routes 是路由追踪的目标，必须先用 ValidTarget 检查过。
func CommandWith(sizeMB int, install bool, routes []string) (string, error) {
	return CloudflareCommand(sizeMB, install, routes, Options{})
}

// CloudflareCommand 和 CommandWith 一样，多了选项：IP 协议给每次 curl 加上 -4 或 -6；
// 多线程时下载和上传各同时开 CloudflareConns 条连接，总数据量不变。默认（单线程、自动）生成的命令和 Command 逐字相同。
func CloudflareCommand(sizeMB int, install bool, routes []string, o Options) (string, error) {
	o, err := o.Normalize(MethodCloudflare)
	if err != nil {
		return "", err
	}
	c, err := Command(sizeMB, routes)
	if err != nil {
		return "", err
	}
	if o.IP != "" {
		c = strings.ReplaceAll(c, "curl -s ", "curl -"+o.IP+" -s ")
	}
	if o.Threads == "multi" {
		down := sizeMB * 1000 * 1000
		up := down / 4
		ip := ""
		if o.IP != "" {
			ip = " -" + o.IP
		}
		oneDown := fmt.Sprintf("curl%s -s -o /dev/null -m 120 -w 'speedtest down speed=%%{speed_download} bytes=%%{size_download} time=%%{time_total} ttfb=%%{time_starttransfer} ip=%%{remote_ip} http=%%{http_version}\\n' 'https://%s/__down?bytes=%d'; ", ip, Server, down)
		oneUp := fmt.Sprintf("head -c %d /dev/zero | curl%s -s -o /dev/null -m 120 -w 'speedtest up speed=%%{speed_upload} bytes=%%{size_upload} time=%%{time_total}\\n' --data-binary @- 'https://%s/__up'", up, ip, Server)
		if !strings.Contains(c, oneDown) || !strings.HasSuffix(c, oneUp) {
			return "", errors.New("生成多线程测速命令失败")
		}
		loop := "for c in"
		for i := 1; i <= CloudflareConns; i++ {
			loop += " " + strconv.Itoa(i)
		}
		c = strings.Replace(c, oneDown, loop+"; do "+strings.Replace(strings.TrimSuffix(oneDown, "; "), fmt.Sprintf("bytes=%d'", down), fmt.Sprintf("bytes=%d'", down/CloudflareConns), 1)+" & done; wait; ", 1)
		c = strings.TrimSuffix(c, oneUp) + loop + "; do " + strings.Replace(oneUp, fmt.Sprintf("head -c %d ", up), fmt.Sprintf("head -c %d ", up/CloudflareConns), 1) + " & done; wait"
	}
	if install {
		c = installScript(len(routes) > 0) + c
	}
	return c, nil
}

// Options 是测速的选项：几条连接、用哪种 IP 协议。
type Options struct {
	Threads string `json:"threads,omitempty"` // single 或 multi；空表示这种测速方式的默认（Ookla 多线程，Cloudflare 单线程）
	IP      string `json:"ip,omitempty"`      // 4 或 6；空表示自动
}

// CloudflareConns 是 Cloudflare 多线程测速同时开的连接数；数据量照旧，平分到每条连接上。
const CloudflareConns = 4

// Normalize 补上测速方式的默认值，并检查选项是不是可选的值。
func (o Options) Normalize(method string) (Options, error) {
	if o.Threads == "" {
		o.Threads = "single"
		if method == MethodOokla {
			o.Threads = "multi"
		}
	}
	if o.Threads != "single" && o.Threads != "multi" {
		return o, errors.New("线程选项不对")
	}
	if o.IP != "" && o.IP != "4" && o.IP != "6" {
		return o, errors.New("IP 协议选项不对")
	}
	return o, nil
}

// allOptions 是全部可选的组合（SpecOf 逐个重新生成来认命令）。
var allOptions = func() []Options {
	var out []Options
	for _, t := range []string{"single", "multi"} {
		for _, ip := range []string{"", "4", "6"} {
			out = append(out, Options{Threads: t, IP: ip})
		}
	}
	return out
}()

// MaxRoutes 是一次测速最多追踪几个目标：够放下四个城市的三家运营商（见 Presets）。
// traceroute 每跳只发一个探测、等 1 秒，最坏 30 秒一个目标，加上测速也在命令超时之内。
const MaxRoutes = 12

// targetRe 是追踪目标允许的样子：IP 地址或者域名，没有空格和引号，可以放进单引号里。
var targetRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.:-]{0,252}$`)

// ValidTarget 报告追踪目标是不是 IP 地址或者域名。
func ValidTarget(t string) bool { return targetRe.MatchString(t) }

// Command 生成不带安装的测速命令。
func Command(sizeMB int, routes []string) (string, error) {
	ok := false
	for _, s := range Sizes {
		ok = ok || s == sizeMB
	}
	if !ok {
		return "", errors.New("测速的数据量不是可选的值")
	}
	trace, err := traceScript(routes)
	if err != nil {
		return "", err
	}
	down := sizeMB * 1000 * 1000
	up := down / 4
	return fmt.Sprintf("if ! curl --version >/dev/null 2>&1; then echo 'speedtest error=no-curl'; exit 0; fi; "+
		"curl -s -m 10 'https://%[1]s/cdn-cgi/trace' | while IFS= read -r l; do echo \"speedtest trace $l\"; done; "+
		"for i in 1 2 3 4 5; do curl -s -o /dev/null -m 10 -w 'speedtest ping dns=%%{time_namelookup} connect=%%{time_connect} tls=%%{time_appconnect} ttfb=%%{time_starttransfer}\\n' 'https://%[1]s/__down?bytes=0'; done; "+
		"%[4]s"+
		"curl -s -o /dev/null -m 120 -w 'speedtest down speed=%%{speed_download} bytes=%%{size_download} time=%%{time_total} ttfb=%%{time_starttransfer} ip=%%{remote_ip} http=%%{http_version}\\n' 'https://%[1]s/__down?bytes=%[2]d'; "+
		"head -c %[3]d /dev/zero | curl -s -o /dev/null -m 120 -w 'speedtest up speed=%%{speed_upload} bytes=%%{size_upload} time=%%{time_total}\\n' --data-binary @- 'https://%[1]s/__up'",
		Server, down, up, trace), nil
}

// traceScript 是对每个追踪目标跑一次路由追踪的那段（traceroute 先用 ICMP、不行再用 UDP；没有就用 tracepath）。没有目标时是空的。
func traceScript(routes []string) (string, error) {
	if len(routes) > MaxRoutes {
		return "", fmt.Errorf("路由追踪最多 %d 个目标", MaxRoutes)
	}
	if len(routes) == 0 {
		return "", nil
	}
	var trace strings.Builder
	trace.WriteString("for t in")
	for _, r := range routes {
		if !ValidTarget(r) {
			return "", errors.New("路由追踪的目标应该是 IP 地址或者域名")
		}
		trace.WriteString(" '" + r + "'")
	}
	trace.WriteString("; do if command -v traceroute >/dev/null 2>&1; then traceroute -n -I -q 1 -w 1 -m 30 \"$t\" 2>/dev/null || traceroute -n -q 1 -w 1 -m 30 \"$t\" 2>&1; " +
		"elif command -v tracepath >/dev/null 2>&1; then tracepath -n -m 30 \"$t\" 2>&1; else echo 'error=no-traceroute'; fi | " +
		"while IFS= read -r l; do echo \"speedtest route $t $l\"; done; done; ")
	return trace.String(), nil
}

var routeListRe = regexp.MustCompile(`for t in((?: '[^']*')+); do if command -v traceroute`)

// Spec 是一条测速命令的参数。
type Spec struct {
	Method string   // MethodOokla 或 MethodCloudflare
	SizeMB int      // Cloudflare 的下载量
	Server int      // Ookla 的服务器编号；0 是自动选
	Routes []string // 路由追踪目标
	Opts   Options  // 线程和 IP 协议（已经补上默认值）
}

// SpecOf 从测速命令里读回参数；不是我们生成的测速命令（带不带安装都算）时 ok 为 false。
// 做法是读出参数后用同样的参数重新生成一遍，逐字相同才算。
func SpecOf(command string) (spec Spec, ok bool) {
	if m := routeListRe.FindStringSubmatch(command); m != nil {
		for _, f := range strings.Fields(m[1]) {
			spec.Routes = append(spec.Routes, strings.Trim(f, "'"))
		}
	}
	if strings.Contains(command, "ookla-speedtest-") {
		spec.Method = MethodOokla
		if m := ooklaServerRe.FindStringSubmatch(command); m != nil {
			spec.Server, _ = strconv.Atoi(m[1])
		}
		for _, o := range allOptions {
			for _, install := range []bool{false, true} {
				if c, err := OoklaCommandWith(spec.Server, install, spec.Routes, o); err == nil && c == command {
					spec.Opts = o
					return spec, true
				}
			}
		}
		return Spec{}, false
	}
	spec.Method = MethodCloudflare
	for _, s := range Sizes {
		for _, o := range allOptions {
			for _, install := range []bool{false, true} {
				if c, err := CloudflareCommand(s, install, spec.Routes, o); err == nil && c == command {
					spec.SizeMB, spec.Opts = s, o
					return spec, true
				}
			}
		}
	}
	return Spec{}, false
}

// CheckCommand 是测速前检查主机上有没有 curl、traceroute、tracepath 的只读命令（L0）：有哪个就打印哪个的路径。
const CheckCommand = "for c in curl traceroute tracepath; do command -v \"$c\"; done"

// Tools 从 CheckCommand 的输出里读出有没有 curl，以及有没有能做路由追踪的工具。
func Tools(output string) (curl, trace bool) {
	for _, l := range strings.Split(output, "\n") {
		l = strings.TrimSpace(l)
		base := l[strings.LastIndex(l, "/")+1:]
		switch base {
		case "curl":
			curl = true
		case "traceroute", "tracepath":
			trace = true
		}
	}
	return
}

// Result 是一台主机的一次测速结果。
type Result struct {
	RequestID string  `json:"requestId"`
	Host      string  `json:"host"`
	TS        int64   `json:"ts"` // 毫秒
	PingMS    float64 `json:"pingMs"`
	DownBps   float64 `json:"downBps"` // 比特每秒
	UpBps     float64 `json:"upBps"`
	SizeMB    int     `json:"sizeMb"`
	Method    string  `json:"method"`            // MethodOokla 或 MethodCloudflare
	Server    string  `json:"server,omitempty"`  // Ookla 用的服务器（给人看的名字）
	Threads   string  `json:"threads,omitempty"` // single 或 multi；之前的结果没有记，是空的
	IP        string  `json:"ip,omitempty"`      // 4 或 6；空是自动
	Error     string  `json:"error,omitempty"`
	// Detail 只在单条报告里返回；列表里不带。之前的结果没有明细，是 nil。
	Detail *Detail `json:"detail,omitempty"`
}

// DetailVersion 是明细 JSON 的格式版本。
const DetailVersion = 1

// Detail 是一次测速的明细，存成 JSON（带版本号）。时间都是毫秒。
type Detail struct {
	V     int       `json:"v"`
	Pings []Ping    `json:"pings"`
	Down  *Transfer `json:"down,omitempty"`
	Up    *Transfer `json:"up,omitempty"`
	Net   NetInfo   `json:"net"`
	// Routes 是到每个追踪目标的路由。
	Routes []Route `json:"routes,omitempty"`
	// Ookla 是用 Ookla 测速时的明细（服务器、丢包、抖动、结果页）；Cloudflare 测速时没有。
	Ookla *OoklaInfo `json:"ookla,omitempty"`
}

// Ping 是一次连接的各段耗时：域名解析、TCP 握手（扣掉解析）、TLS 握手（扣掉前面的）、等首字节（扣掉前面的）。
type Ping struct {
	DNS  float64 `json:"dns"`
	TCP  float64 `json:"tcp"`
	TLS  float64 `json:"tls"`
	Wait float64 `json:"wait"`
}

// Transfer 是一次下载或上传：传了多少字节、用了多久、首字节时间、平均速度（比特每秒）。
type Transfer struct {
	Bytes float64 `json:"bytes"`
	MS    float64 `json:"ms"`
	TTFB  float64 `json:"ttfb,omitempty"`
	Bps   float64 `json:"bps"`
	Conns int     `json:"conns,omitempty"` // 同时开了几条连接（Cloudflare 多线程时大于 1）
}

// addTransfer 把一条连接的结果并进 t。多条连接同时跑时，字节数相加、用时取最长、首字节取最快，
// 速度按总字节数除以最长用时算；只有一条连接时用 curl 自己算的平均速度。
func addTransfer(t *Transfer, bytes, ms, ttfb, bps float64) *Transfer {
	if t == nil {
		return &Transfer{Bytes: bytes, MS: ms, TTFB: ttfb, Bps: bps, Conns: 1}
	}
	t.Conns++
	t.Bytes += bytes
	t.MS = math.Max(t.MS, ms)
	if ttfb > 0 && (t.TTFB == 0 || ttfb < t.TTFB) {
		t.TTFB = ttfb
	}
	if t.MS > 0 {
		t.Bps = round(t.Bytes * 8 / (t.MS / 1000))
	}
	return t
}

// NetInfo 是测速服务看到的这台主机：出口 IP、国家或地区、连到的节点，以及用的 HTTP 和 TLS 版本、服务器地址。
type NetInfo struct {
	IP       string `json:"ip,omitempty"`
	Loc      string `json:"loc,omitempty"`
	Colo     string `json:"colo,omitempty"`
	HTTP     string `json:"http,omitempty"`
	TLS      string `json:"tls,omitempty"`
	ServerIP string `json:"serverIp,omitempty"`
}

func kv(line string) map[string]string {
	out := map[string]string{}
	for _, f := range strings.Fields(line) {
		if k, v, ok := strings.Cut(f, "="); ok {
			out[k] = v
		}
	}
	return out
}

// traceRe 限定 trace 里取的值：只要短的、看得懂的字符。远端内容，不认识的就丢掉。
var traceRe = regexp.MustCompile(`^[A-Za-z0-9.:/_-]{1,64}$`)

func num(m map[string]string, k string) float64 {
	v, err := strconv.ParseFloat(m[k], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

func round(v float64) float64 { return math.Round(v*100) / 100 }

// Report 是从一台主机的输出里读出来的结果：汇总的三个数，加上明细。
type Report struct {
	PingMS, DownBps, UpBps float64
	Error                  string
	Detail                 Detail
}

// Parse 从一台主机的输出里读出结果。读不到下载或上传时写进 Error。
func Parse(output string) Report {
	rep := Report{Detail: Detail{V: DetailVersion, Pings: []Ping{}}}
	d := &rep.Detail
	ping := math.Inf(1)
	var downBytes, upBytes float64 = -1, -1
	ooklaErr, using, version := "", "", ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "speedtest ")
		if !ok {
			continue
		}
		if t, ok := strings.CutPrefix(rest, "route "); ok {
			d.Routes = addRouteLine(d.Routes, t)
			continue
		}
		if t, ok := strings.CutPrefix(rest, "ookla "); ok {
			if msg := parseOokla(&rep, t); msg != "" && ooklaErr == "" {
				ooklaErr = msg
			}
			if d.Ookla != nil {
				downBytes, upBytes = d.Down.Bytes, d.Up.Bytes
			}
			continue
		}
		if t, ok := strings.CutPrefix(rest, "version "); ok {
			// "Speedtest by Ookla 1.2.0.84 (5ed00a8b2d) Linux/x86_64-linux-musl …"：只留版本号（是版本号，不是地址）。 hygiene:allow
			if f := strings.Fields(strings.TrimPrefix(t, "Speedtest by Ookla ")); len(f) > 0 && traceRe.MatchString(f[0]) {
				version = f[0]
			}
			continue
		}
		if t, ok := strings.CutPrefix(rest, "using="); ok && (t == "system" || t == "pinned") {
			using = t
			continue
		}
		if t, ok := strings.CutPrefix(rest, "trace "); ok {
			k, v, _ := strings.Cut(t, "=")
			if !traceRe.MatchString(v) {
				continue
			}
			switch k {
			case "ip":
				d.Net.IP = v
			case "loc":
				d.Net.Loc = v
			case "colo":
				d.Net.Colo = v
			case "tls":
				d.Net.TLS = v
			}
			continue
		}
		m := kv(rest)
		switch {
		case m["error"] == "no-curl":
			return Report{Error: "主机上没有 curl，测不了速"}
		case m["error"] == "no-package-manager":
			return Report{Error: "主机上没有 curl，也没找到能用的包管理器，请手动安装 curl"}
		case m["error"] == "install-failed":
			return Report{Error: "自动安装 curl 失败（可能没有 root 或免密 sudo，或者软件源不可用）"}
		case m["error"] == "unsupported-arch":
			return Report{Error: "主机的处理器架构 Speedtest 不支持（支持 x86_64、ARM 和 i386）"}
		case m["error"] == "no-sha256sum":
			return Report{Error: "主机上没有 sha256sum，没法校验 Speedtest 程序，没有运行"}
		case m["error"] == "download-failed":
			return Report{Error: "下载 Speedtest 程序失败（连不上 install.speedtest.net 或超时）"}
		case m["error"] == "no-ipv4":
			return Report{Error: "主机上找不到 IPv4 出口地址，测不了 IPv4"}
		case m["error"] == "no-ipv6":
			return Report{Error: "主机上找不到 IPv6 出口地址（可能没有 IPv6），测不了 IPv6"}
		case m["error"] == "bad-checksum":
			return Report{Error: "下载的 Speedtest 程序校验不通过，没有运行"}
		case strings.HasPrefix(rest, "ping "):
			dns, conn, tls, ttfb := num(m, "dns"), num(m, "connect"), num(m, "tls"), num(m, "ttfb")
			if conn <= 0 {
				continue
			}
			if conn-dns < ping {
				ping = conn - dns
			}
			p := Ping{DNS: round(dns * 1000), TCP: round((conn - dns) * 1000)}
			if tls > conn {
				p.TLS = round((tls - conn) * 1000)
			}
			if ttfb > math.Max(tls, conn) {
				p.Wait = round((ttfb - math.Max(tls, conn)) * 1000)
			}
			if len(d.Pings) < 10 {
				d.Pings = append(d.Pings, p)
			}
		case strings.HasPrefix(rest, "down "):
			d.Down = addTransfer(d.Down, num(m, "bytes"), round(num(m, "time")*1000), round(num(m, "ttfb")*1000), num(m, "speed")*8)
			rep.DownBps, downBytes = d.Down.Bps, d.Down.Bytes
			if v := m["ip"]; traceRe.MatchString(v) {
				d.Net.ServerIP = v
			}
			if v := m["http"]; traceRe.MatchString(v) {
				d.Net.HTTP = v
			}
		case strings.HasPrefix(rest, "up "):
			d.Up = addTransfer(d.Up, num(m, "bytes"), round(num(m, "time")*1000), 0, num(m, "speed")*8)
			rep.UpBps, upBytes = d.Up.Bps, d.Up.Bytes
		}
	}
	finishRoutes(d.Routes)
	if o := d.Ookla; o != nil {
		o.Binary, o.Version = using, version
		if using == "pinned" {
			o.Version = OoklaVersion
		}
	}
	if !math.IsInf(ping, 1) {
		rep.PingMS = ping * 1000 // 秒 → 毫秒
	}
	switch {
	case ooklaErr != "" && d.Ookla == nil:
		rep.Error = "Speedtest 出错：" + ooklaErr
	case downBytes < 0 && upBytes < 0:
		rep.Error = "没有拿到测速结果"
	case downBytes <= 0:
		rep.Error = "下载失败（连不上测速服务器或超时）"
	case upBytes <= 0:
		rep.Error = "上传失败（连不上测速服务器或超时）"
	}
	return rep
}

// Service 听内核的事件，把测速请求的结果存下来。
type Service struct {
	db      *sql.DB
	geo     *geoCache
	servers *serverCache
}

// New 创建测速服务。
func New(db *sql.DB) *Service { return &Service{db: db, geo: newGeoCache(), servers: newServerCache()} }

// Retention 是测速历史的保留规则：每台主机最多留 PerHost 条，并且（Days 大于 0 时）删掉早于 Days 天的。
// 这些只是测速结果的副本；对应的请求和输出照常在审计里，不受影响。
type Retention struct {
	Days    int `json:"days"`
	PerHost int `json:"perHost"`
}

// DayChoices 和 PerHostChoices 是可选的值；Days 为 0 表示不按时间删。
var (
	DayChoices     = []int{0, 7, 30, 90, 180, 365}
	PerHostChoices = []int{10, 20, 50, 100, 500}
)

// DefaultRetention 是默认规则：每台主机留 100 条，不按时间删。
var DefaultRetention = Retention{Days: 0, PerHost: 100}

const retentionKey = "speedtest.retention"

// Valid 报告规则里的值是不是可选的值。
func (r Retention) Valid() bool {
	in := func(v int, list []int) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	return in(r.Days, DayChoices) && in(r.PerHost, PerHostChoices)
}

// Target 是一个路由追踪目标：名字只给人看，目标是 IP 地址或者域名。
type Target struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

const targetsKey = "speedtest.routes"

// DefaultTargets 是默认的追踪目标：只有 Cloudflare 的测速节点本身。想看回程线路，就加上自己那边各运营商的地址。
// 名字留空：名字是所有者填的数据，不跟着界面语言翻译，写死一个中文名在英文界面里会露出来；空名字时界面直接显示目标。
var DefaultTargets = []Target{{Name: "", Target: Server}}

// Targets 读当前的路由追踪目标。
func (s *Service) Targets() []Target {
	var v string
	if s.db.QueryRow("SELECT value FROM settings WHERE key = ?", targetsKey).Scan(&v) == nil {
		var t []Target
		if json.Unmarshal([]byte(v), &t) == nil && CheckTargets(t) == nil {
			return t
		}
	}
	return DefaultTargets
}

// CheckTargets 检查追踪目标：最多 MaxRoutes 个，目标是 IP 或域名、不重复，名字不超过 40 个字。
func CheckTargets(t []Target) error {
	if len(t) > MaxRoutes {
		return fmt.Errorf("路由追踪最多 %d 个目标", MaxRoutes)
	}
	seen := map[string]bool{}
	for _, x := range t {
		if !ValidTarget(x.Target) {
			return errors.New("路由追踪的目标应该是 IP 地址或者域名")
		}
		if seen[x.Target] {
			return errors.New("路由追踪的目标重复了")
		}
		seen[x.Target] = true
		if len([]rune(x.Name)) > 40 || strings.ContainsAny(x.Name, "\r\n") {
			return errors.New("目标的名字最多 40 个字")
		}
	}
	return nil
}

// SetTargets 保存路由追踪目标。
func (s *Service) SetTargets(ctx context.Context, t []Target) error {
	if err := CheckTargets(t); err != nil {
		return err
	}
	v, _ := json.Marshal(t)
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, targetsKey, string(v))
	return err
}

// TargetList 是追踪目标的地址列表，按顺序。
func TargetList(t []Target) []string {
	out := make([]string, 0, len(t))
	for _, x := range t {
		out = append(out, x.Target)
	}
	return out
}

// Retention 读当前的保留规则。
func (s *Service) Retention() Retention {
	var v string
	if s.db.QueryRow("SELECT value FROM settings WHERE key = ?", retentionKey).Scan(&v) == nil {
		var r Retention
		if json.Unmarshal([]byte(v), &r) == nil && r.Valid() {
			return r
		}
	}
	return DefaultRetention
}

// SetRetention 保存保留规则，并马上按新规则清理一次。
func (s *Service) SetRetention(ctx context.Context, r Retention) error {
	if !r.Valid() {
		return errors.New("保留规则不是可选的值")
	}
	v, _ := json.Marshal(r)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, retentionKey, string(v)); err != nil {
		return err
	}
	_, err := s.Prune(ctx)
	return err
}

// Prune 按保留规则删掉多出来的和过期的结果，返回删了几条。
func (s *Service) Prune(ctx context.Context) (int64, error) {
	r := s.Retention()
	var n int64
	if r.Days > 0 {
		res, err := s.db.ExecContext(ctx, `DELETE FROM speedtests WHERE ts < ?`, time.Now().AddDate(0, 0, -r.Days).UnixMilli())
		if err != nil {
			return n, err
		}
		d, _ := res.RowsAffected()
		n += d
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM speedtests WHERE rowid IN (
		SELECT rowid FROM (SELECT rowid, ROW_NUMBER() OVER (PARTITION BY host ORDER BY ts DESC) AS k FROM speedtests) WHERE k > ?)`, r.PerHost)
	if err != nil {
		return n, err
	}
	d, _ := res.RowsAffected()
	return n + d, nil
}

// Delete 删掉指定的结果（按请求和主机）；all 为真时全部删掉。返回删了几条。
func (s *Service) Delete(ctx context.Context, items []Key, all bool) (int64, error) {
	if all {
		res, err := s.db.ExecContext(ctx, `DELETE FROM speedtests`)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	var n int64
	for _, it := range items {
		res, err := s.db.ExecContext(ctx, `DELETE FROM speedtests WHERE request_id = ? AND host = ?`, it.RequestID, it.Host)
		if err != nil {
			return n, err
		}
		d, _ := res.RowsAffected()
		n += d
	}
	return n, nil
}

// Key 指一条测速结果。
type Key struct {
	RequestID string `json:"requestId"`
	Host      string `json:"host"`
}

// Count 返回现存的结果条数。
func (s *Service) Count(ctx context.Context) (n int) {
	s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM speedtests`).Scan(&n)
	return n
}

// Run 一直听到 ctx 结束。
func (s *Service) Run(ctx context.Context, k *kernel.Kernel) {
	events, unsubscribe := k.Subscribe()
	defer unsubscribe()
	// 按时间删的规则要定期看一眼：没有新测速时也会过期。
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	s.Prune(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.Prune(ctx)
		case e := <-events:
			// 事件里的请求不带各台主机的输出（输出是单独流式推送的）；结束时再向内核要一份完整的快照来解析。
			if r, ok := e.Data.(*kernel.Request); ok && e.Type == "request" && r.Snippet == Tag &&
				(r.State == kernel.StateDone || r.State == kernel.StateFailed) {
				if full, err := k.Get(r.ID); err == nil {
					s.record(full)
				}
			}
		}
	}
}

func (s *Service) record(r *kernel.Request) {
	if r.Snippet != Tag || (r.State != kernel.StateDone && r.State != kernel.StateFailed) {
		return
	}
	at := time.Now()
	if r.FinishedAt != nil {
		at = *r.FinishedAt
	}
	for _, t := range r.Targets {
		spec, ok := SpecOf(t.Command)
		if !ok {
			continue // 命令不是我们生成的那条：不解析
		}
		rep := Parse(t.Output)
		size, server := spec.SizeMB, ""
		if o := rep.Detail.Ookla; o != nil {
			// Ookla 的数据量是实际传了多少（下载加上传）。
			size = int(math.Round((rep.Detail.Down.Bytes + rep.Detail.Up.Bytes) / 1e6))
			server = o.Server.Label()
		}
		detail := ""
		if t.State == "error" || t.State == "skipped" {
			rep.Error = "没有执行：" + t.Error
		} else if len(rep.Detail.Pings) > 0 || rep.Detail.Down != nil || rep.Detail.Up != nil || len(rep.Detail.Routes) > 0 {
			names := map[string]string{}
			for _, t := range s.Targets() {
				names[t.Target] = t.Name
			}
			for i := range rep.Detail.Routes {
				rep.Detail.Routes[i].Name = names[rep.Detail.Routes[i].Target]
			}
			b, _ := json.Marshal(rep.Detail)
			detail = string(b)
		}
		s.db.Exec(`INSERT OR IGNORE INTO speedtests (request_id, host, ts, ping_ms, down_bps, up_bps, size_mb, error, detail, method, server, threads, ip) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			r.ID, t.Host, at.UnixMilli(), rep.PingMS, rep.DownBps, rep.UpBps, size, rep.Error, detail, spec.Method, server, spec.Opts.Threads, spec.Opts.IP)
	}
	s.Prune(context.Background())
}

// Recent 返回最近的结果，新的在前。
func (s *Service) Recent(ctx context.Context, limit int) ([]Result, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT request_id, host, ts, ping_ms, down_bps, up_bps, size_mb, error, method, server, threads, ip FROM speedtests ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Result{}
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.RequestID, &r.Host, &r.TS, &r.PingMS, &r.DownBps, &r.UpBps, &r.SizeMB, &r.Error, &r.Method, &r.Server, &r.Threads, &r.IP); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Get 返回一条结果，带明细。没有这条时返回 sql.ErrNoRows。
func (s *Service) Get(ctx context.Context, k Key) (*Result, error) {
	var r Result
	var detail string
	err := s.db.QueryRowContext(ctx, `SELECT request_id, host, ts, ping_ms, down_bps, up_bps, size_mb, error, detail, method, server, threads, ip FROM speedtests WHERE request_id = ? AND host = ?`,
		k.RequestID, k.Host).Scan(&r.RequestID, &r.Host, &r.TS, &r.PingMS, &r.DownBps, &r.UpBps, &r.SizeMB, &r.Error, &detail, &r.Method, &r.Server, &r.Threads, &r.IP)
	if err != nil {
		return nil, err
	}
	if detail != "" {
		var d Detail
		if json.Unmarshal([]byte(detail), &d) == nil && d.V == DetailVersion {
			r.Detail = &d
		}
	}
	return &r, nil
}
