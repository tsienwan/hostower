package speedtest

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Speedtest by Ookla：用 Ookla 官方的命令行程序测速，可以指定测速服务器（包括国内各运营商自己架的节点）。
//
// 主机上已经有官方的 speedtest（比如所有者自己从 Ookla 的软件源装的，会跟着系统更新）就直接用它：
// 认的办法是 speedtest --version 的第一行以 "Speedtest by Ookla " 开头（同名的 Python 版 speedtest-cli 不是这样）。
// 没有的话才下载固定版本的官方压缩包，装在登录账号家目录下的固定位置。用的是哪个、什么版本，都打印出来记进报告。
// 压缩包留在同一个目录里（不删：删除运行时才确定的路径会被判成高危），下次下载直接覆盖。
// 下面每个架构的压缩包和里面程序的 SHA-256 都写死在代码里，所以命令文本（也就是批准绑定的摘要）
// 就确定了要运行的是哪个程序：每次运行前都先校验程序本身，对不上就重新下载、校验压缩包和程序，还对不上就不运行。
// 换版本只能改这里，随正式版本发布。
//
// Ookla 的程序要求接受它的许可协议和隐私政策（命令里带 --accept-license --accept-gdpr）。
// 网关只在所有者在页面上接受过之后才生成这条命令；接受这件事写审计。
// 它按带宽自动决定传多少数据，测一次可能用掉几百 MB 到 1 GB 以上，数据量控制不了。

// 测速方式。
const (
	MethodOokla      = "ookla"
	MethodCloudflare = "cloudflare"
)

// OoklaVersion 是使用的 Ookla 命令行程序版本。
const OoklaVersion = "1.2.0"

// OoklaDir 是程序在主机上的位置（登录账号的家目录下）。
const OoklaDir = "$HOME/.hostower/ookla-" + OoklaVersion

// OoklaTerms 和 OoklaPrivacy 是 Ookla 的许可协议和隐私政策。
const (
	OoklaTerms   = "https://www.speedtest.net/about/eula"
	OoklaPrivacy = "https://www.speedtest.net/about/privacy"
)

// MaxServerID 是测速服务器编号的上限（Ookla 的编号是正整数）。
const MaxServerID = 9999999

type ooklaBuild struct {
	match string // uname -m 的匹配写法（case 分支）
	arch  string // 压缩包名里的架构
	tgz   string // 压缩包的 SHA-256
	bin   string // 里面 speedtest 程序的 SHA-256
}

// ooklaBuilds 是各架构的官方压缩包（https://install.speedtest.net/app/cli/ookla-speedtest-<版本>-linux-<架构>.tgz）。
var ooklaBuilds = []ooklaBuild{
	{"x86_64|amd64", "x86_64", "5690596c54ff9bed63fa3732f818a05dbc2db19ad36ed68f21ca5f64d5cfeeb7", "31f1124c5ab8acdae6b9fe1741e704df420f9f2e7d429679fabe62075453c051"},
	{"aarch64|arm64", "aarch64", "3953d231da3783e2bf8904b6dd72767c5c6e533e163d3742fd0437affa431bd3", "d99fa13293f658b53eaa79fe81f4b210db39fdfc1e9698f33da3f234a6008df7"},
	{"armv7*|armv8l", "armhf", "e45fcdebbd8a185553535533dd032d6b10bc8c64eee4139b1147b9c09835d08d", "66ad57568664e6f8580e14ad67316a57038fd22b30548bef98531df4ebcc8956"},
	{"armv6*|armv5*", "armel", "629a455a2879224bd0dbd4b36d8c721dda540717937e4660b4d2c966029466bf", "d103b5372da7720413f5263e0b557b6d477669785da2b6d7393d00e9708daf2b"},
	{"i386|i486|i586|i686", "i386", "9ff7e18dbae7ee0e03c66108445a2fb6ceea6c86f66482e1392f55881b772fe8", "8c600519568eddf31849fbbe9c65b1987dd6f81d69d9b443d4e4afdb3f4864b0"},
}

// ooklaScript 是“系统里有官方的就用它；没有就按架构选压缩包、没有或对不上就下载并校验；再运行”的那段。只输出以 "speedtest " 开头的几行。
func ooklaScript(server int, o Options) string {
	var b strings.Builder
	b.WriteString(`d="` + OoklaDir + `"; b="$d/speedtest"; u=pinned; v="$(speedtest --version 2>/dev/null | head -n 1)"; ` +
		`case "$v" in "Speedtest by Ookla "*) b="$(command -v speedtest)"; u=system; echo "speedtest version $v";; esac; `)
	b.WriteString(`if [ "$u" = pinned ]; then case "$(uname -m)" in`)
	for _, o := range ooklaBuilds {
		fmt.Fprintf(&b, ` %s) a=%s; th=%s; bh=%s;;`, o.match, o.arch, o.tgz, o.bin)
	}
	b.WriteString(` *) echo 'speedtest error=unsupported-arch'; exit 0;; esac; `)
	b.WriteString(`if ! command -v sha256sum >/dev/null 2>&1; then echo 'speedtest error=no-sha256sum'; exit 0; fi; `)
	b.WriteString(`if [ "$(sha256sum "$b" 2>/dev/null | cut -d ' ' -f 1)" != "$bh" ]; then echo 'speedtest installing ookla'; ` +
		`if ! curl --version >/dev/null 2>&1; then echo 'speedtest error=no-curl'; exit 0; fi; mkdir -p "$d" && ` +
		`curl -fsSL -m 120 -o "$d/speedtest.tgz" "https://install.speedtest.net/app/cli/ookla-speedtest-` + OoklaVersion + `-linux-$a.tgz" || { echo 'speedtest error=download-failed'; exit 0; }; ` +
		`if [ "$(sha256sum "$d/speedtest.tgz" | cut -d ' ' -f 1)" != "$th" ]; then echo 'speedtest error=bad-checksum'; exit 0; fi; ` +
		`tar xzf "$d/speedtest.tgz" -C "$d" speedtest; ` +
		`if [ "$(sha256sum "$b" 2>/dev/null | cut -d ' ' -f 1)" != "$bh" ]; then echo 'speedtest error=bad-checksum'; exit 0; fi; chmod 755 "$b"; fi; fi; `)
	// 指定 IP 协议：Ookla 的程序没有 -4/-6，只能绑定这种协议的出口地址（-i）。出口地址按到一个保留地址的路由查出来（不发包）。
	switch o.IP {
	case "4":
		b.WriteString(`src="$(ip -4 route get 192.0.2.1 2>/dev/null | sed -n 's/.* src \([^ ]*\).*/\1/p')"; [ -n "$src" ] || { echo 'speedtest error=no-ipv4'; exit 0; }; `)
	case "6":
		b.WriteString(`src="$(ip -6 route get 2001:db8::1 2>/dev/null | sed -n 's/.* src \([^ ]*\).*/\1/p')"; [ -n "$src" ] || { echo 'speedtest error=no-ipv6'; exit 0; }; `)
	}
	b.WriteString(`echo "speedtest using=$u"; "$b" --accept-license --accept-gdpr -f json -p no`)
	if server > 0 {
		b.WriteString(" -s " + strconv.Itoa(server))
	}
	// 单线程：下载和上传都只开一条连接（程序没写进帮助、但支持的连接数范围参数）。
	if o.Threads == "single" {
		b.WriteString(" --download-conn-range=1-1 --upload-conn-range=1-1")
	}
	if o.IP != "" {
		b.WriteString(` -i "$src"`)
	}
	b.WriteString(` 2>&1 | while IFS= read -r l; do echo "speedtest ookla $l"; done`)
	return b.String()
}

// OoklaCommand 生成用 Ookla 测速的命令。server 为 0 时让程序自己选最近的服务器。
// 用锁定版本时主机上需要 curl（只在第一次下载程序时用）和 sha256sum、tar；install 为 true 时没有 curl 先装上。
// 有追踪目标时先追踪路由（和 Cloudflare 测速同一段）。
func OoklaCommand(server int, install bool, routes []string) (string, error) {
	return OoklaCommandWith(server, install, routes, Options{})
}

// OoklaCommandWith 和 OoklaCommand 一样，多了线程和 IP 协议选项。默认（多线程、自动）生成的命令和 OoklaCommand 逐字相同。
func OoklaCommandWith(server int, install bool, routes []string, o Options) (string, error) {
	o, err := o.Normalize(MethodOokla)
	if err != nil {
		return "", err
	}
	if server < 0 || server > MaxServerID {
		return "", errors.New("测速服务器编号不对")
	}
	trace, err := traceScript(routes)
	if err != nil {
		return "", err
	}
	c := trace + ooklaScript(server, o)
	if install {
		c = installScript(len(routes) > 0) + c
	}
	return c, nil
}

var ooklaServerRe = regexp.MustCompile(`-f json -p no -s ([0-9]{1,7})[ "]`)

// OoklaInfo 是 Ookla 测速给出的明细。都是远端来的内容，只按纯文本显示。
type OoklaInfo struct {
	Server      OoklaServer `json:"server"`
	ISP         string      `json:"isp,omitempty"`
	PacketLoss  *float64    `json:"packetLoss,omitempty"` // 百分比；服务器不支持时没有
	Jitter      float64     `json:"jitter"`
	Low         float64     `json:"low,omitempty"`
	High        float64     `json:"high,omitempty"`
	DownLatency float64     `json:"downLatency,omitempty"` // 下载时的延迟（四分位均值）
	UpLatency   float64     `json:"upLatency,omitempty"`
	URL         string      `json:"url,omitempty"`     // speedtest.net 上的结果页
	Binary      string      `json:"binary,omitempty"`  // system：主机上已经装好的官方程序；pinned：下载的锁定版本
	Version     string      `json:"version,omitempty"` // 程序报出的版本
}

// OoklaServer 是这次用的测速服务器。
type OoklaServer struct {
	ID       int    `json:"id"`
	Name     string `json:"name,omitempty"`
	Location string `json:"location,omitempty"`
	Country  string `json:"country,omitempty"`
	Host     string `json:"host,omitempty"`
	IP       string `json:"ip,omitempty"`
}

// Label 是服务器给人看的名字：“China Telecom · Shanghai”。
func (s OoklaServer) Label() string {
	var parts []string
	for _, p := range []string{s.Name, s.Location} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

var resultURLRe = regexp.MustCompile(`^https://www\.speedtest\.net/result/c/[0-9a-fA-F-]{8,64}$`)

type ooklaLine struct {
	Type    string `json:"type"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Ping    struct {
		Jitter, Latency, Low, High float64
	} `json:"ping"`
	Download, Upload struct {
		Bandwidth, Bytes, Elapsed float64
		Latency                   struct{ IQM float64 } `json:"latency"`
	}
	PacketLoss *float64 `json:"packetLoss"`
	ISP        string   `json:"isp"`
	Interface  struct {
		ExternalIP string `json:"externalIp"`
	} `json:"interface"`
	Server struct {
		ID                                int
		Host, Name, Location, Country, IP string
	} `json:"server"`
	Result struct {
		URL string `json:"url"`
	} `json:"result"`
}

// fin 把远端给的数字限制在合理范围：不是数、无穷、负数都当 0。
func fin(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	return v
}

// parseOokla 读一行 Ookla 的 JSON 输出，把结果填进 rep。返回读到的错误信息（没有就是空）。
func parseOokla(rep *Report, line string) string {
	var o ooklaLine
	if json.Unmarshal([]byte(line), &o) != nil {
		return ""
	}
	switch o.Type {
	case "log":
		if o.Level == "error" {
			return clean(o.Message)
		}
	case "result":
		d := &rep.Detail
		rep.PingMS = round(fin(o.Ping.Latency))
		rep.DownBps = fin(o.Download.Bandwidth) * 8
		rep.UpBps = fin(o.Upload.Bandwidth) * 8
		d.Down = &Transfer{Bytes: fin(o.Download.Bytes), MS: fin(o.Download.Elapsed), Bps: rep.DownBps}
		d.Up = &Transfer{Bytes: fin(o.Upload.Bytes), MS: fin(o.Upload.Elapsed), Bps: rep.UpBps}
		info := &OoklaInfo{
			ISP: clean(o.ISP), Jitter: round(fin(o.Ping.Jitter)), Low: round(fin(o.Ping.Low)), High: round(fin(o.Ping.High)),
			DownLatency: round(fin(o.Download.Latency.IQM)), UpLatency: round(fin(o.Upload.Latency.IQM)),
			Server: OoklaServer{ID: o.Server.ID, Name: clean(o.Server.Name), Location: clean(o.Server.Location), Country: clean(o.Server.Country),
				Host: clean(o.Server.Host), IP: clean(o.Server.IP)},
		}
		if o.PacketLoss != nil {
			v := round(math.Min(fin(*o.PacketLoss), 100))
			info.PacketLoss = &v
		}
		if resultURLRe.MatchString(o.Result.URL) {
			info.URL = o.Result.URL
		}
		if traceRe.MatchString(o.Interface.ExternalIP) {
			d.Net.IP = o.Interface.ExternalIP
		}
		if traceRe.MatchString(info.Server.IP) {
			d.Net.ServerIP = info.Server.IP
		}
		d.Ookla = info
	}
	return ""
}
