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
	// 出口：地址、国家或地区、Cloudflare 接入点、是不是走 WARP；IPv6 出口另查一次。
	jobs = append(jobs,
		`echo "egress $(curl -s -m 8 `+trace+` 2>/dev/null | grep -E '^(ip|loc|colo|warp)=' | tr '\n' ' ')"`,
		`echo "egress6 $(curl -6 -s -m 6 `+trace+` 2>/dev/null | grep -E '^ip=' | tr '\n' ' ')"`,
		// 出口 IP 的归属和类型：先问出自己的出口地址，再拿它去查公开的 IP 信息库。
		`{ ip="$(curl -s -m 8 `+trace+` 2>/dev/null | sed -n 's/^ip=//p')"; echo "ipinfo $(curl -s -m 8 "https://proxycheck.io/v2/$ip?vpn=1&asn=1&risk=1" 2>/dev/null | tr -d '\n' | tail -c 1500)"; }`) // hygiene:allow 公开的 IP 信息库
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

// Egress 是主机的出口。
type Egress struct {
	IP      string `json:"ip,omitempty"`
	IPv6    string `json:"ipv6,omitempty"`    // 有 IPv6 出口时的地址
	Country string `json:"country,omitempty"` // 两个字母的国家或地区代码
	Colo    string `json:"colo,omitempty"`    // Cloudflare 接入点（机场代码），大致是出口附近的城市
	Warp    bool   `json:"warp,omitempty"`    // 出口走的是 Cloudflare WARP
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

// NetStatus 是网络检测的全部结果。
type NetStatus struct {
	Egress   Egress          `json:"egress"`
	Services []ServiceStatus `json:"services"`
	Sites    []SiteStatus    `json:"sites"`
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
		case "egress", "egress6", "ipinfo":
			lines[kind] = rest
		case "svc", "site", "region":
			id, body, _ := strings.Cut(rest, " ")
			lines[kind+" "+id] = body
		}
	}
	st := NetStatus{Egress: Egress{Risk: -1}, Services: make([]ServiceStatus, 0, len(Services)), Sites: make([]SiteStatus, 0, len(Sites))}
	e := &st.Egress
	for _, m := range kvRe.FindAllStringSubmatch(lines["egress"], -1) {
		switch {
		case m[1] == "ip" && (ipv4Re.MatchString(m[2]) || ipv6Re.MatchString(m[2])):
			e.IP = m[2]
		case m[1] == "loc" && len(m[2]) == 2 && strings.ToUpper(m[2]) == m[2] && asLetters(m[2]):
			e.Country = m[2]
		case m[1] == "colo" && len(m[2]) == 3 && asLetters(m[2]):
			e.Colo = m[2]
		case m[1] == "warp":
			e.Warp = m[2] == "on" || m[2] == "plus"
		}
	}
	for _, m := range kvRe.FindAllStringSubmatch(lines["egress6"], -1) {
		if m[1] == "ip" && strings.Contains(m[2], ":") && ipv6Re.MatchString(m[2]) {
			e.IPv6 = m[2]
		}
	}
	parseIPInfo(e, lines["ipinfo"])
	for _, s := range Services {
		st.Services = append(st.Services, ParseService(s, lines["svc "+s.ID]))
	}
	for _, s := range Sites {
		st.Sites = append(st.Sites, parseSite(s, lines["site "+s.ID], lines["region "+s.ID]))
	}
	return st
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
func parseIPInfo(e *Egress, body string) {
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
