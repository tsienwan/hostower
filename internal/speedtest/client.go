package speedtest

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// “这台设备”测速：把正在看面板的这台设备（比如家里连着 Wi-Fi 的手机）当成一个节点。
//
//   - 延迟、下载、上传是浏览器和面板之间量的（见 gateway/speedclient.go），数字由浏览器交回来，
//     只代表“这台设备 ↔ 面板”这一段；面板前面有反向代理或 CDN 时，量到的是到代理那一段。
//   - 回程路由（主机 → 这台设备）和别的命令一样交给内核：在选中的主机上对面板看到的来源地址跑一次路由追踪，再 ping 十次。
//     地址是网关从连接上读出来的，不接受请求里自报的。
//   - 去程路由（这台设备 → 服务器）浏览器追踪不了：可以在电脑上跑 tracert / traceroute，把输出贴进来，按同一份网段表认线路。

// MethodClient 是“这台设备”这种测速方式。
const MethodClient = "client"

// ClientSizes 是浏览器测速时下载量的上限（MB）；上传量上限是它的四分之一。下载和上传各自最多跑 ClientSeconds 秒。
var ClientSizes = []int{25, 100, 250}

// ClientSeconds 是浏览器测速时下载、上传各自最多跑几秒。
const ClientSeconds = 10

// ClientPings 是浏览器量延迟时发几次请求。
const ClientPings = 20

// ClientVersion 是 Client 存成 JSON 时的格式版本。
const ClientVersion = 1

const clientMark = "echo 'speedtest client ip="

// ClientCommand 生成“从主机追踪到这台设备”的命令：先路由追踪，再 ping 十次看丢包和抖动（家用路由器常常不回应，那就只有路由）。
// install 为真时前面多一段：traceroute 和 tracepath 都没有就用系统的包管理器装上 traceroute。
func ClientCommand(ip string, install bool) (string, error) {
	a, err := netip.ParseAddr(ip)
	if err != nil || a.Zone() != "" || a.IsUnspecified() {
		return "", errors.New("读不出这台设备的来源地址，没法追踪回程路由")
	}
	ip = a.Unmap().String()
	trace, err := traceScript([]string{ip})
	if err != nil {
		return "", errors.New("这台设备的来源地址不能用来追踪路由")
	}
	var b strings.Builder
	if install {
		b.WriteString(installTraceroute)
	}
	b.WriteString(clientMark + ip + "'; ")
	b.WriteString(trace)
	b.WriteString(backPing("'" + ip + "'"))
	return b.String(), nil
}

// installTraceroute 是“traceroute 和 tracepath 都没有就用系统的包管理器装上 traceroute”的那一段。
var installTraceroute = "if ! command -v traceroute >/dev/null 2>&1 && ! command -v tracepath >/dev/null 2>&1; then echo 'speedtest installing traceroute'; " +
	installTree("traceroute", "echo 'speedtest warn=no-package-manager'") + "fi; "

// backPing 是从主机 ping 目标十次、只留统计行的那一段；target 是已经写成 shell 词的目标。
func backPing(target string) string {
	return "if command -v ping >/dev/null 2>&1; then ping -n -c 10 -i 0.2 -W 1 " + target + " 2>&1 | tail -n 3 | " +
		"while IFS= read -r l; do echo \"speedtest back $l\"; done; fi"
}

// clientSpec 认出 ClientCommand 生成的命令（带不带安装都算）：读出地址后重新生成一遍，逐字相同才算。
func clientSpec(command string, routes []string) (Spec, bool) {
	if len(routes) != 1 {
		return Spec{}, false
	}
	for _, install := range []bool{false, true} {
		if c, err := ClientCommand(routes[0], install); err == nil && c == command {
			return Spec{Method: MethodClient, Routes: routes}, true
		}
	}
	return Spec{}, false
}

// HostPing 是从主机 ping 这台设备的结果。Recv 为 0 表示对方不回应（家用路由器很常见），不代表不通。
type HostPing struct {
	Sent   int     `json:"sent"`
	Recv   int     `json:"recv"`
	Min    float64 `json:"min,omitempty"`
	Avg    float64 `json:"avg,omitempty"`
	Max    float64 `json:"max,omitempty"`
	Jitter float64 `json:"jitter,omitempty"` // ping 报的 mdev
}

var (
	pingCountRe = regexp.MustCompile(`(\d{1,4}) packets transmitted, (\d{1,4}) (?:packets )?received`)
	pingRttRe   = regexp.MustCompile(`= ([0-9.]+)/([0-9.]+)/([0-9.]+)(?:/([0-9.]+))? ms`)
)

// addBackLine 把一行 "speedtest back <ping 的统计行>" 并进 p。
func addBackLine(p *HostPing, line string) *HostPing {
	if m := pingCountRe.FindStringSubmatch(line); m != nil {
		if p == nil {
			p = &HostPing{}
		}
		p.Sent, _ = strconv.Atoi(m[1])
		p.Recv, _ = strconv.Atoi(m[2])
		if p.Recv > p.Sent {
			p.Recv = p.Sent
		}
	}
	if m := pingRttRe.FindStringSubmatch(line); m != nil && p != nil {
		f := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return round(v) }
		p.Min, p.Avg, p.Max, p.Jitter = f(m[1]), f(m[2]), f(m[3]), f(m[4])
	}
	return p
}

// Client 是浏览器交回来的测量结果（这台设备 ↔ 面板），加上之后贴进来的去程路由。
// 数字是浏览器自己量的、由登录的用户提交，只用来显示；地址由服务端填。
type Client struct {
	V     int    `json:"v"`
	IP    string `json:"ip,omitempty"`    // 面板看到的来源地址
	Proxy bool   `json:"proxy,omitempty"` // 面板前面有反向代理：量到的是到代理那一段
	Proto string `json:"proto,omitempty"` // 浏览器报的协议：h2、h3、http/1.1
	Conn  string `json:"conn,omitempty"`  // 浏览器报的连接类型：wifi、cellular…（很多浏览器不报）
	// Pings 是每次请求的往返时间（毫秒）；Lost 是失败了几次。
	Pings []float64 `json:"pings"`
	Lost  int       `json:"lost,omitempty"`
	Down  *Transfer `json:"down,omitempty"`
	Up    *Transfer `json:"up,omitempty"`
	// Forward 是贴进来的去程路由。
	Forward *Route `json:"forward,omitempty"`
}

var clientWordRe = regexp.MustCompile(`^[a-z0-9./-]{1,16}$`)

func okNum(v float64, max float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= max
}

func cleanTransfer(t *Transfer) (*Transfer, error) {
	if t == nil {
		return nil, nil
	}
	if !okNum(t.Bytes, 4e9) || !okNum(t.MS, 600e3) || !okNum(t.Bps, 1e12) || t.Conns < 0 || t.Conns > 16 {
		return nil, errors.New("测量结果里的数字不对")
	}
	return &Transfer{Bytes: math.Round(t.Bytes), MS: round(t.MS), Bps: math.Round(t.Bps), Conns: t.Conns}, nil
}

// clean 检查并整理浏览器交来的数字；地址、代理和去程路由不从这里来。
func (c Client) clean() (Client, error) {
	out := Client{V: ClientVersion, Pings: []float64{}, Lost: c.Lost}
	if len(c.Pings) > 2*ClientPings || c.Lost < 0 || c.Lost > 2*ClientPings {
		return out, errors.New("测量结果里的延迟次数不对")
	}
	for _, p := range c.Pings {
		if !okNum(p, 60e3) {
			return out, errors.New("测量结果里的数字不对")
		}
		out.Pings = append(out.Pings, round(p))
	}
	var err error
	if out.Down, err = cleanTransfer(c.Down); err != nil {
		return out, err
	}
	if out.Up, err = cleanTransfer(c.Up); err != nil {
		return out, err
	}
	if clientWordRe.MatchString(c.Proto) {
		out.Proto = c.Proto
	}
	if clientWordRe.MatchString(c.Conn) {
		out.Conn = c.Conn
	}
	return out, nil
}

// median 返回中位数；空的时候是 0。
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if n := len(s); n%2 == 1 {
		return s[n/2]
	} else {
		return round((s[n/2-1] + s[n/2]) / 2)
	}
}

// apply 把浏览器量的数字放到结果上（列表里显示的延迟、下载、上传、数据量）。
func (c Client) apply(r *Result) {
	r.PingMS = median(c.Pings)
	var total float64
	if c.Down != nil {
		r.DownBps = c.Down.Bps
		total += c.Down.Bytes
	}
	if c.Up != nil {
		r.UpBps = c.Up.Bps
		total += c.Up.Bytes
	}
	r.SizeMB = int(math.Round(total / 1e6))
}

func (s *Service) client(ctx context.Context, id string) (Client, bool) {
	var data string
	if err := s.db.QueryRowContext(ctx, `SELECT data FROM speedtest_clients WHERE request_id = ?`, id).Scan(&data); err != nil {
		return Client{}, false
	}
	var c Client
	if json.Unmarshal([]byte(data), &c) != nil || c.V != ClientVersion {
		return Client{}, false
	}
	return c, true
}

func (s *Service) putClient(ctx context.Context, id string, c Client) error {
	c.V = ClientVersion
	if c.Pings == nil {
		c.Pings = []float64{}
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO speedtest_clients (request_id, ts, data) VALUES (?,?,?)
		ON CONFLICT (request_id) DO UPDATE SET data = excluded.data`, id, time.Now().UnixMilli(), string(b))
	return err
}

// SaveClient 记下浏览器对一次“这台设备”测速交回来的数字。ip 和 proxy 由网关填；已经贴过的去程路由留着。
func (s *Service) SaveClient(ctx context.Context, id, ip string, proxy bool, measured Client) error {
	c, err := measured.clean()
	if err != nil {
		return err
	}
	c.IP, c.Proxy = ip, proxy
	if old, ok := s.client(ctx, id); ok {
		c.Forward = old.Forward
	}
	return s.putClient(ctx, id, c)
}

// SaveForward 把贴进来的去程路由（tracert、traceroute、tracepath、mtr 的输出）解析后记到一次“这台设备”测速上；text 为空是清掉。
func (s *Service) SaveForward(ctx context.Context, id, text string) (*Route, error) {
	c, _ := s.client(ctx, id)
	if strings.TrimSpace(text) == "" {
		c.Forward = nil
		return nil, s.putClient(ctx, id, c)
	}
	r, err := ParsePasted(text)
	if err != nil {
		return nil, err
	}
	c.Forward = &r
	return &r, s.putClient(ctx, id, c)
}

// pruneClients 删掉已经没有对应测速结果的浏览器数据。留一天的余量：浏览器的数字可能比请求先到。
func (s *Service) pruneClients(ctx context.Context) {
	s.db.ExecContext(ctx, `DELETE FROM speedtest_clients WHERE ts < ? AND request_id NOT IN (SELECT request_id FROM speedtests)`,
		time.Now().Add(-24*time.Hour).UnixMilli())
}

// ForwardTarget 是去程路由在 Route.Target 里的写法（它没有“目标地址”：是从设备到服务器）。
const ForwardTarget = "forward"

var (
	// 一跳的开头：traceroute / tracert（" 3  "）、tracepath（" 3:  "、" 3?: "）、mtr（" 3.|-- "）。
	pastedHopRe = regexp.MustCompile(`^\s*(\d{1,2})(?:\?)?(?::\s+|\.\|--\s*|\s+)(.*)$`)
	// 时间：英文系统是 "12 ms"、"<1 ms"，中文 Windows 是 "12 毫秒"。
	pastedMsRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s?(?:ms|毫秒)`)
)

// MaxPasted 是贴进来的路由输出最多多少字节。
const MaxPasted = 16 << 10

// ParsePasted 从贴进来的路由追踪输出里读出每一跳，并推测线路。认 Windows 的 tracert、traceroute、tracepath 和 mtr 的报告。
// 贴进来的是外来的文字：只留跳数、合法的 IP 地址和时间，别的都丢掉。
func ParsePasted(text string) (Route, error) {
	r := Route{Target: ForwardTarget, Dir: "forward", Hops: []Hop{}, Nets: []string{}}
	if len(text) > MaxPasted {
		return r, errors.New("贴进来的内容太长了")
	}
	last := 0
	for _, line := range strings.Split(text, "\n") {
		m := pastedHopRe.FindStringSubmatch(strings.TrimRight(line, "\r "))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n <= last || n > 64 || len(r.Hops) >= 64 {
			continue
		}
		h := Hop{N: n}
		for _, f := range strings.Fields(m[2]) {
			if a, err := netip.ParseAddr(strings.Trim(f, "[]()")); err == nil {
				h.IP = a.String()
				h.Net = netOf(a)
				break
			}
		}
		if h.IP != "" {
			if t := pastedMsRe.FindStringSubmatch(m[2]); t != nil {
				h.MS, _ = strconv.ParseFloat(t[1], 64)
				h.MS = round(h.MS)
			}
		}
		r.Hops = append(r.Hops, h)
		last = n
	}
	found := false
	for _, h := range r.Hops {
		found = found || h.IP != ""
	}
	if !found {
		return r, errors.New("没有从贴进来的内容里认出路由：请贴 tracert、traceroute 或 mtr 的完整输出")
	}
	rs := []Route{r}
	finishRoutes(rs)
	return rs[0], nil
}
