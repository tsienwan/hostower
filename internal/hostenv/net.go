package hostenv

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// 网络这一半：出口 IP 的归属和类型、各家模型接口、常用网站和流媒体。
// 所有请求并在一条只读命令里、在主机上并行发出（每个最多等 8 秒），每个结果占一行，前面带着它是谁。
// 一条命令只占一个会话：分开查的话，一台主机一下子要开二十多个 SSH 会话，会超过 sshd 默认的上限。

// Site 是一个被检测的网站。
type Site struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Group string `json:"group"` // search | social | video | ai | dev
	URL   string `json:"-"`
	// OKCodes 是除了 2xx、3xx 之外也算“能打开”的状态码（比如只接受 GET 的站点对 HEAD 回 405）。
	OKCodes []int `json:"-"`
	// Mode 是特殊的检测办法：netflix（看一部非自制剧能不能看、落在哪个区）、youtube（另外读所在的区）。
	Mode string `json:"-"`
}

// Sites 是检测的网站，按显示顺序。
var Sites = []Site{
	{ID: "google", Title: "Google", Group: "search", URL: "https://www.google.com/generate_204"},
	{ID: "youtube", Title: "YouTube", Group: "video", URL: "https://www.youtube.com/", Mode: "youtube"},
	{ID: "netflix", Title: "Netflix", Group: "video", URL: "https://www.netflix.com/title/81280792", Mode: "netflix"}, // hygiene:allow 被检测的公开网站
	{ID: "disney", Title: "Disney+", Group: "video", URL: "https://www.disneyplus.com/"},                              // hygiene:allow 被检测的公开网站
	{ID: "primevideo", Title: "Prime Video", Group: "video", URL: "https://www.primevideo.com/", OKCodes: []int{405}}, // hygiene:allow 被检测的公开网站
	{ID: "spotify", Title: "Spotify", Group: "video", URL: "https://open.spotify.com/"},                               // hygiene:allow 被检测的公开网站
	{ID: "tiktok", Title: "TikTok", Group: "social", URL: "https://www.tiktok.com/"},                                  // hygiene:allow 被检测的公开网站
	{ID: "facebook", Title: "Facebook", Group: "social", URL: "https://www.facebook.com/"},                            // hygiene:allow 被检测的公开网站
	{ID: "instagram", Title: "Instagram", Group: "social", URL: "https://www.instagram.com/"},                         // hygiene:allow 被检测的公开网站
	{ID: "x", Title: "X（Twitter）", Group: "social", URL: "https://x.com/"},                                            // hygiene:allow 被检测的公开网站
	{ID: "telegram", Title: "Telegram", Group: "social", URL: "https://telegram.org/"},                                // hygiene:allow 被检测的公开网站
	{ID: "discord", Title: "Discord", Group: "social", URL: "https://discord.com/"},                                   // hygiene:allow 被检测的公开网站
	{ID: "reddit", Title: "Reddit", Group: "social", URL: "https://www.reddit.com/"},                                  // hygiene:allow 被检测的公开网站
	{ID: "chatgpt", Title: "ChatGPT（网页）", Group: "ai", URL: "https://chatgpt.com/"},                                   // hygiene:allow 被检测的公开网站
	{ID: "gemini-web", Title: "Gemini（网页）", Group: "ai", URL: "https://gemini.google.com/"},
	{ID: "github", Title: "GitHub", Group: "dev", URL: "https://github.com/"},
	{ID: "dockerhub", Title: "Docker Hub", Group: "dev", URL: "https://hub.docker.com/"},
	{ID: "npm", Title: "npm", Group: "dev", URL: "https://registry.npmjs.org/-/ping", OKCodes: []int{405}}, // hygiene:allow 被检测的公开网站
	{ID: "wikipedia", Title: "Wikipedia", Group: "search", URL: "https://www.wikipedia.org/"},              // hygiene:allow 被检测的公开网站
}

// NetCommand 是网络检测的只读脚本。必须是 L0。
var NetCommand = buildNetCommand()

func buildNetCommand() string {
	const trace = "https://www.cloudflare.com/cdn-cgi/trace" // hygiene:allow 公开的 trace 接口：只返回请求方自己的地址和位置
	var jobs []string
	// 出口：IPv4 和 IPv6 分开查（只查一次的话，有 IPv6 的主机默认走 IPv6，就看不到 IPv4 出口了）。
	// 每个出口：地址、国家或地区、Cloudflare 接入点、是不是走 WARP；再拿地址去公开的 IP 信息库查归属和类型。
	for _, v := range []string{"4", "6"} {
		jobs = append(jobs,
			`echo "egress`+v+` $(curl -`+v+` -s -m 8 `+trace+` 2>/dev/null | grep -E '^(ip|loc|colo|warp)=' | tr '\n' ' ')"`,
			`{ ip="$(curl -`+v+` -s -m 8 `+trace+` 2>/dev/null | sed -n 's/^ip=//p')"; [ -n "$ip" ] && echo "ipinfo`+v+` $(curl -s -m 8 "https://proxycheck.io/v2/$ip?vpn=1&asn=1&risk=1" 2>/dev/null | tr -d '\n' | tail -c 1500)"; }`) // hygiene:allow 公开的 IP 信息库
	}
	// AI 服务看到的地区：ChatGPT 和 Claude 的网页都在 Cloudflare 后面，它们自己的 trace 接口说明它们把这台主机当成哪个区。
	for _, a := range aiSeen {
		jobs = append(jobs, `echo "seen `+a.id+` $(curl -s -m 8 '`+a.url+`' 2>/dev/null | grep -E '^loc=' | head -n 1)"`)
	}
	// 风控信号：ChatGPT 网页有没有对这台主机要求人机验证；Google 搜索有没有把它当成“异常流量”。
	jobs = append(jobs,
		`echo "risk chatgpt $(curl -sI -m 8 -A 'Mozilla/5.0' 'https://chatgpt.com/' 2>/dev/null | grep -i -E '^(HTTP|cf-mitigated)' | tr -d '\r' | tr '\n' ' ')"`, // hygiene:allow 被检测的公开网站
		`echo "risk google $(curl -sI -m 8 -A 'Mozilla/5.0' -w '|%{http_code}|%{redirect_url}' 'https://www.google.com/search?q=hostower' 2>/dev/null | tail -n 1)"`)
	for _, s := range Services {
		jobs = append(jobs, `echo "svc `+s.ID+` $(curl -s -m 8 -w '|code=%{http_code}' '`+s.URL+`' 2>/dev/null | tail -c 600 | tr '\n' ' ')"`)
	}
	for _, s := range Sites {
		switch s.Mode {
		case "netflix":
			// 跟着跳转走到底：最后的地址里带着区域（/sg/title/…），最后的状态码说明这部非自制剧在这个区能不能看。
			jobs = append(jobs, `echo "site `+s.ID+` $(curl -s -L -m 8 -w '|%{http_code}|%{url_effective}' '`+s.URL+`' 2>/dev/null | tail -c 200 | tr '\n' ' ')"`)
		default:
			jobs = append(jobs, `echo "site `+s.ID+` $(curl -sI -m 8 -w '|%{http_code}|%{redirect_url}' '`+s.URL+`' 2>/dev/null | tail -n 1)"`)
		}
		if s.Mode == "youtube" {
			jobs = append(jobs, `echo "region `+s.ID+` $(curl -s -m 8 'https://www.youtube.com/premium' 2>/dev/null | grep -o '"countryCode":"[A-Z]*"' | head -n 1)"`)
		}
	}
	return strings.Join(jobs, " & ") + " & wait; echo '== end'"
}

// aiSeen 是查“AI 服务把这台主机当成哪个区”的地址（服务自己域名下的 Cloudflare trace）。
var aiSeen = []struct{ id, url string }{
	{"chatgpt", "https://chatgpt.com/cdn-cgi/trace"}, // hygiene:allow 被检测的公开网站
	{"claude", "https://claude.ai/cdn-cgi/trace"},    // hygiene:allow 被检测的公开网站
}

// AIRestricted 是 OpenAI、Anthropic、Google 的 AI 服务都不对其提供服务的常见地区（内置的参考名单，以各家官方公布的为准）。
var AIRestricted = []string{"CN", "HK", "MO", "RU", "BY", "IR", "KP", "SY", "CU"}

// Egress 是主机的出口：IPv4 和 IPv6 各一个（没有的为空）。
type Egress struct {
	V4   *IPInfo `json:"v4,omitempty"`
	V6   *IPInfo `json:"v6,omitempty"`
	Warp bool    `json:"warp,omitempty"` // 出口走的是 Cloudflare WARP
}

// IPInfo 是一个出口地址的情况。
type IPInfo struct {
	IP      string `json:"ip"`
	Country string `json:"country,omitempty"` // 两个字母的国家或地区代码
	Colo    string `json:"colo,omitempty"`    // Cloudflare 接入点（机场代码），大致是出口附近的城市
	// 下面来自 IP 信息库；查不到时为空。
	ASN      string `json:"asn,omitempty"`
	Provider string `json:"provider,omitempty"` // 运营方
	City     string `json:"city,omitempty"`
	Region   string `json:"region,omitempty"` // 国家或地区的名字
	// Type 是地址的类型：hosting（机房）、residential（家庭宽带）、business（企业专线）、wireless（移动网络）、vpn、其他原样小写。
	Type string `json:"type,omitempty"`
	// Proxy 表示信息库把它标成了代理或 VPN 出口。
	Proxy bool `json:"proxy,omitempty"`
	// Risk 是信息库给的风险分（0 到 100，越高越容易被网站风控）；-1 表示没有查到。
	Risk int `json:"risk"`
}

// SiteStatus 是一个网站的检测结果。
type SiteStatus struct {
	Site
	State string `json:"state"`
	Code  int    `json:"code"`
	// Region 是网站认为这台主机所在的区（两个字母），只有部分网站查得出来。
	Region string `json:"region,omitempty"`
	// Note 是补充结论：netflix 的 full（非自制剧也能看）或 originals（只能看自制剧）。
	Note string `json:"note,omitempty"`
}

// AIRegion 是一家 AI 服务把这台主机当成哪个区。
type AIRegion struct {
	ID     string `json:"id"`     // chatgpt | claude | google
	Region string `json:"region"` // 两个字母；没查到为空
	// Restricted 表示这个区在 AIRestricted 名单里。
	Restricted bool `json:"restricted,omitempty"`
}

// RiskCheck 是一项风控信号。State：ok（没有问题）、warn（有问题）、unknown（没查出来）。
type RiskCheck struct {
	ID    string `json:"id"` // chatgpt-challenge | google-captcha | region-mismatch
	State string `json:"state"`
	Code  int    `json:"code,omitempty"`
}

// NetStatus 是网络检测的全部结果。
type NetStatus struct {
	Egress   Egress          `json:"egress"`
	Services []ServiceStatus `json:"services"`
	Sites    []SiteStatus    `json:"sites"`
	AI       []AIRegion      `json:"ai"`
	Risks    []RiskCheck     `json:"risks"`
}

var (
	ipv4Re    = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,3}){3}$`)
	ipv6Re    = regexp.MustCompile(`^[0-9a-fA-F:]{3,39}$`)
	kvRe      = regexp.MustCompile(`\b(ip|loc|colo|warp)=(\S+)`)
	siteRe    = regexp.MustCompile(`\|([0-9]{3})\|(\S*)\s*$`)
	nfRegion  = regexp.MustCompile(`netflix\.com/([a-z]{2})(-[a-z]{2})?/`)
	ytRegion  = regexp.MustCompile(`"countryCode":"([A-Z]{2})"`)
	asnRe     = regexp.MustCompile(`^AS[0-9]{1,10}$`)
	plainText = regexp.MustCompile(`[^\p{L}\p{N} .,()&'/+-]`)
)

// clean 把信息库里的名字收成一行普通文字（页面按纯文本显示，这里再去掉控制字符和奇怪的符号）。
func clean(s string) string {
	s = strings.TrimSpace(plainText.ReplaceAllString(s, ""))
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

// ParseNet 解析 NetCommand 的输出。
func ParseNet(out string) NetStatus {
	lines := map[string]string{}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		kind, rest, ok := strings.Cut(l, " ")
		if !ok {
			continue
		}
		switch kind {
		case "egress4", "egress6", "ipinfo4", "ipinfo6":
			lines[kind] = rest
		case "svc", "site", "region", "seen", "risk":
			id, body, _ := strings.Cut(rest, " ")
			lines[kind+" "+id] = body
		}
	}
	st := NetStatus{Services: make([]ServiceStatus, 0, len(Services)), Sites: make([]SiteStatus, 0, len(Sites)), AI: []AIRegion{}, Risks: []RiskCheck{}}
	st.Egress.V4 = parseEgress(lines["egress4"], lines["ipinfo4"], ipv4Re, &st.Egress.Warp)
	st.Egress.V6 = parseEgress(lines["egress6"], lines["ipinfo6"], ipv6Re, &st.Egress.Warp)
	for _, s := range Services {
		st.Services = append(st.Services, ParseService(s, lines["svc "+s.ID]))
	}
	for _, s := range Sites {
		st.Sites = append(st.Sites, parseSite(s, lines["site "+s.ID], lines["region "+s.ID]))
	}
	parseAI(&st, lines)
	return st
}

// parseEgress 解析一个出口（trace 的那一行和 IP 信息库的回答）；没有这个出口时返回 nil。
func parseEgress(trace, info string, ipRe *regexp.Regexp, warp *bool) *IPInfo {
	e := &IPInfo{Risk: -1}
	for _, m := range kvRe.FindAllStringSubmatch(trace, -1) {
		switch {
		case m[1] == "ip" && ipRe.MatchString(m[2]):
			e.IP = m[2]
		case m[1] == "loc" && validRegion(m[2]):
			e.Country = m[2]
		case m[1] == "colo" && len(m[2]) == 3 && asLetters(m[2]):
			e.Colo = m[2]
		case m[1] == "warp":
			*warp = *warp || m[2] == "on" || m[2] == "plus"
		}
	}
	if e.IP == "" {
		return nil
	}
	parseIPInfo(e, info)
	return e
}

func validRegion(s string) bool { return len(s) == 2 && asLetters(s) }

func restricted(region string) bool {
	for _, r := range AIRestricted {
		if r == region {
			return true
		}
	}
	return false
}

var (
	locRe       = regexp.MustCompile(`\bloc=([A-Z]{2})\b`)
	challengeRe = regexp.MustCompile(`(?i)cf-mitigated:\s*challenge`)
	httpRe      = regexp.MustCompile(`HTTP/[0-9.]+ ([0-9]{3})`)
)

// parseAI 解析 AI 服务看到的地区和风控信号。
func parseAI(st *NetStatus, lines map[string]string) {
	for _, a := range aiSeen {
		r := AIRegion{ID: a.id}
		if m := locRe.FindStringSubmatch(lines["seen "+a.id]); m != nil {
			r.Region = m[1]
		}
		r.Restricted = restricted(r.Region)
		st.AI = append(st.AI, r)
	}
	// Google 没有 trace 接口：用 YouTube 认为的区（同一家的地区判断）。
	g := AIRegion{ID: "google"}
	for _, s := range st.Sites {
		if s.ID == "youtube" {
			g.Region = s.Region
		}
	}
	g.Restricted = restricted(g.Region)
	st.AI = append(st.AI, g)

	// ChatGPT 网页：Cloudflare 回 cf-mitigated: challenge 就是要求人机验证（这个地址的信誉不好）。
	c := RiskCheck{ID: "chatgpt-challenge", State: "unknown"}
	line := lines["risk chatgpt"]
	if m := httpRe.FindStringSubmatch(line); m != nil {
		c.Code, _ = strconv.Atoi(m[1])
	}
	switch {
	case challengeRe.MatchString(line):
		c.State = "warn"
	case c.Code >= 200 && c.Code < 400:
		c.State = "ok"
	}
	st.Risks = append(st.Risks, c)

	// Google 搜索：429 或者被带到 /sorry/ 就是“异常流量”，要做人机验证才能搜。
	g2 := RiskCheck{ID: "google-captcha", State: "unknown"}
	if m := siteRe.FindStringSubmatch(lines["risk google"]); m != nil {
		g2.Code, _ = strconv.Atoi(m[1])
		switch {
		case g2.Code == 429 || strings.Contains(m[2], "/sorry/"):
			g2.State = "warn"
		case g2.Code >= 200 && g2.Code < 400:
			g2.State = "ok"
		}
	}
	st.Risks = append(st.Risks, g2)

	// 地区不一致：IP 信息库说在 A，AI 服务却当成 B（比如 IP 在新加坡、Google 当成香港），常常会被当成“不支持的地区”。
	m := RiskCheck{ID: "region-mismatch", State: "unknown"}
	if st.Egress.V4 != nil && st.Egress.V4.Country != "" {
		m.State = "ok"
		for _, a := range st.AI {
			if a.Region != "" && a.Region != st.Egress.V4.Country {
				m.State = "warn"
			}
		}
	}
	st.Risks = append(st.Risks, m)
}

func asLetters(s string) bool {
	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// parseIPInfo 解析 IP 信息库的回答：{"status":"ok","<地址>":{…}}。
func parseIPInfo(e *IPInfo, body string) {
	i := strings.Index(body, "{")
	if i < 0 {
		return
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(body[i:]), &raw) != nil {
		return
	}
	for k, v := range raw {
		if k == "status" || k == "message" || k == "query time" {
			continue
		}
		var d struct {
			ASN      string `json:"asn"`
			Provider string `json:"provider"`
			Country  string `json:"country"`
			City     string `json:"city"`
			Proxy    string `json:"proxy"`
			Type     string `json:"type"`
			Risk     *int   `json:"risk"`
		}
		if json.Unmarshal(v, &d) != nil {
			continue
		}
		if asnRe.MatchString(d.ASN) {
			e.ASN = d.ASN
		}
		e.Provider, e.City, e.Region = clean(d.Provider), clean(d.City), clean(d.Country)
		e.Proxy = strings.EqualFold(d.Proxy, "yes")
		e.Type = strings.ToLower(clean(d.Type))
		if d.Risk != nil && *d.Risk >= 0 && *d.Risk <= 100 {
			e.Risk = *d.Risk
		}
		return
	}
}

func parseSite(s Site, line, region string) SiteStatus {
	st := SiteStatus{Site: s, State: StateUnreachable}
	m := siteRe.FindStringSubmatch(line)
	if m == nil {
		return st
	}
	st.Code, _ = strconv.Atoi(m[1])
	if st.Code == 0 {
		return st
	}
	target := strings.ToLower(m[2])
	ok := st.Code >= 200 && st.Code < 400
	for _, c := range s.OKCodes {
		ok = ok || st.Code == c
	}
	switch {
	case strings.Contains(target, "unavailable") || strings.Contains(target, "unsupported") || strings.Contains(target, "/sorry/"):
		// 被带到“你所在的地区不提供服务”或人机验证页。
		st.State = StateBlocked
	case ok:
		st.State = StateOK
	case st.Code == 403 || st.Code == 451:
		st.State = StateBlocked
	default:
		st.State = StateUnknown
	}
	switch s.Mode {
	case "netflix":
		if r := nfRegion.FindStringSubmatch(target); r != nil {
			st.Region = strings.ToUpper(r[1])
		}
		// 这一部不是自制剧：200 说明这个区的片库能看；404 说明这个区只看得到自制剧。
		switch st.Code {
		case 200:
			st.Note = "full"
		case 404:
			st.State, st.Note = StateOK, "originals"
		}
	case "youtube":
		if r := ytRegion.FindStringSubmatch(region); r != nil {
			st.Region = r[1]
		}
	}
	return st
}
