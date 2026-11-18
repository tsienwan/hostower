package speedtest

import (
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// 路由追踪：从主机到每个目标跑一次 traceroute（或 tracepath），把每一跳的地址和延迟读出来，
// 再按几家运营商公开的骨干网段推测走的是哪条线路（CN2 GIA / GT、联通 9929 / 4837、移动 CMI 等）。
// 只是推测：网段表是固定的、不联网查询，跳数里不回应的节点也看不到。

// Hop 是路由里的一跳。IP 为空表示这一跳没有回应。
type Hop struct {
	N   int     `json:"n"`
	IP  string  `json:"ip,omitempty"`
	MS  float64 `json:"ms,omitempty"`
	Net string  `json:"net,omitempty"` // 认出来的骨干网，见 backbones
}

// Route 是到一个目标的路由追踪结果。
type Route struct {
	Target string   `json:"target"`
	Name   string   `json:"name,omitempty"` // 记录时按当时的设置填上的名字
	Hops   []Hop    `json:"hops"`
	Nets   []string `json:"nets"`           // 按出现的顺序，去重
	Line   string   `json:"line,omitempty"` // 推测的线路
	Error  string   `json:"error,omitempty"`
}

// backbone 是一段公开的运营商骨干网段。
type backbone struct {
	prefix netip.Prefix
	net    string
}

// backbones 是用来认线路的骨干网段（各运营商公开的骨干地址段，不是任何部署的地址）。
var backbones = func() []backbone {
	list := [][2]string{
		{"59.43.0.0/16", "CN2"},     // 电信 CN2（AS4809） hygiene:allow
		{"202.97.0.0/16", "163"},    // 电信 163 骨干（AS4134） hygiene:allow
		{"218.105.0.0/16", "9929"},  // 联通 9929 / CUII（AS9929） hygiene:allow
		{"210.51.0.0/16", "9929"},   // 联通 9929 / CUII（AS9929） hygiene:allow
		{"219.158.0.0/16", "4837"},  // 联通 169 骨干（AS4837） hygiene:allow
		{"223.118.0.0/15", "CMI"},   // 移动国际 CMI（AS58453） hygiene:allow
		{"223.120.0.0/16", "CMI"},   // 移动国际 CMI（AS58453） hygiene:allow
		{"221.176.0.0/13", "CMNET"}, // 移动 CMNET 骨干（AS9808） hygiene:allow
	}
	out := make([]backbone, 0, len(list))
	for _, l := range list {
		out = append(out, backbone{netip.MustParsePrefix(l[0]), l[1]})
	}
	return out
}()

// netOf 认出一个地址属于哪段骨干网；内网地址标成 "LAN"。
func netOf(a netip.Addr) string {
	if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || netip.MustParsePrefix("100.64.0.0/10").Contains(a) { // 运营商级 NAT 地址段 hygiene:allow
		return "LAN"
	}
	for _, b := range backbones {
		if b.prefix.Contains(a) {
			return b.net
		}
	}
	return ""
}

// lineOf 按每一跳认出的骨干网（按跳数顺序，可以重复）推测线路。
//
// 电信的 CN2 GIA 和 CN2 GT 在出入境段都走 CN2，区别在国内段：GIA 一直走 CN2，GT 在国内换成 163 骨干。
// 所以只有 163 骨干出现在 CN2 那一段的外侧（第一个 CN2 之前，或最后一个 CN2 之后）才算 GT；
// 夹在两段 CN2 中间的零星 163 地址多半是路由器用别的接口回应，不算换网。
// 只按自治系统号认出的“电信”（CHINANET，含各省城域网）不算 163 骨干。
// 反过来也一样：夹在 163（或电信）中间的单个 CN2 地址不算走了 CN2，见 dropStrays。
func lineOf(seq []string) string {
	seq = dropStrays(seq)
	has := map[string]bool{}
	first, last := -1, -1
	for i, n := range seq {
		has[n] = true
		if n == "CN2" {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	// 163 骨干要紧挨着 CN2 那一段（中间只隔着电信自己的地址）：隔了别家运营商的网（比如 CN2 先交给移动），
	// 后面再出现的 163 和这条电信线路无关。
	gt := false
	if first >= 0 {
		for i := last + 1; i < len(seq) && telecomFamily(seq[i]) == "163"; i++ {
			gt = gt || seq[i] == "163"
		}
		for i := first - 1; i >= 0 && telecomFamily(seq[i]) == "163"; i-- {
			gt = gt || seq[i] == "163"
		}
	}
	var out []string
	switch {
	case has["CN2"] && gt:
		out = append(out, "电信 CN2 GT")
	case has["CN2"]:
		out = append(out, "电信 CN2 GIA")
	case has["CTGNet"]:
		out = append(out, "电信 CTGNet")
	case has["163"] || has["CHINANET"]:
		out = append(out, "电信 163") // 不走 CN2 的电信线路就是 163
	}
	switch {
	case has["9929"]:
		out = append(out, "联通 9929")
	case has["4837"]:
		out = append(out, "联通 4837")
	}
	switch {
	case has["CMIN2"]:
		out = append(out, "移动 CMIN2")
	case has["CMI"]:
		out = append(out, "移动 CMI")
	case has["CMNET"]:
		out = append(out, "移动 CMNET")
	}
	return strings.Join(out, " + ")
}

// telecomFamily 把电信的几种标签归成一类，用来判断“前后是不是同一张网”：163 骨干和只按自治系统号认出的电信地址算一类。
func telecomFamily(n string) string {
	if n == "CHINANET" {
		return "163"
	}
	return n
}

// dropStrays 去掉零星的一跳：只有一跳的骨干网，前后两侧是同一张网时，多半是路由器用别的接口回应、
// 或者两张网的互联点，不代表线路换了网（比如 163 163 CN2 163，或者 CN2 163 CN2）。
// 按顺序和已经留下的上一跳比，所以交替出现的 CN2 163 CN2 163 只去掉夹在中间的那一个。
func dropStrays(seq []string) []string {
	out := make([]string, 0, len(seq))
	for i, n := range seq {
		if len(out) > 0 && i+1 < len(seq) {
			prev, next := telecomFamily(out[len(out)-1]), telecomFamily(seq[i+1])
			if prev == next && telecomFamily(n) != prev {
				continue
			}
		}
		out = append(out, n)
	}
	return out
}

// hopRe 认 traceroute（" 3  192.0.2.1  1.234 ms"、" 4  *"）和 tracepath（" 3:  192.0.2.1  1.234ms"、" 4:  no reply"）的一行。
var hopRe = regexp.MustCompile(`^\s*(\d{1,3})\??:?\s+(\S+)(?:\s+([0-9.]+)\s?ms)?`)

// addRouteLine 把一行 "speedtest route <目标> <原始行>" 的内容并进 routes。
func addRouteLine(routes []Route, rest string) []Route {
	target, line, _ := strings.Cut(rest, " ")
	if !ValidTarget(target) {
		return routes
	}
	i := len(routes) - 1
	if i < 0 || routes[i].Target != target {
		if len(routes) >= MaxRoutes {
			return routes
		}
		routes = append(routes, Route{Target: target, Hops: []Hop{}, Nets: []string{}})
		i++
	}
	r := &routes[i]
	if strings.TrimSpace(line) == "error=no-traceroute" {
		r.Error = "主机上没有 traceroute 或 tracepath"
		return routes
	}
	m := hopRe.FindStringSubmatch(line)
	if m == nil {
		return routes
	}
	n, _ := strconv.Atoi(m[1])
	if n <= 0 || n > 64 {
		return routes
	}
	h := Hop{N: n}
	if a, err := netip.ParseAddr(m[2]); err == nil {
		h.IP = a.String()
		h.Net = netOf(a)
		h.MS, _ = strconv.ParseFloat(m[3], 64)
		h.MS = round(h.MS)
	}
	// tracepath 同一跳可能打两行（先探 MTU）；有地址的那行为准。
	if k := len(r.Hops) - 1; k >= 0 && r.Hops[k].N == n {
		if r.Hops[k].IP == "" {
			r.Hops[k] = h
		}
	} else if len(r.Hops) < 64 {
		r.Hops = append(r.Hops, h)
	}
	return routes
}

// finishRoutes 补上每条路由出现过的骨干网和推测的线路。
func finishRoutes(routes []Route) {
	for i := range routes {
		r := &routes[i]
		seen := map[string]bool{}
		var seq []string
		for _, h := range r.Hops {
			if h.Net == "" || h.Net == "LAN" {
				continue
			}
			seq = append(seq, h.Net)
			if !seen[h.Net] {
				seen[h.Net] = true
				r.Nets = append(r.Nets, h.Net)
			}
		}
		r.Line = lineOf(seq)
		if r.Error == "" && len(r.Hops) == 0 {
			r.Error = "没有拿到路由"
		}
	}
}
