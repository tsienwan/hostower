package speedtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// 节点地区：打开测速报告时，面板把路由里各跳的公网地址发给 GeoService，查出大致的国家、地区、城市、运营商和自治系统号。
// 结果只在内存里缓存一天，不写进测速记录（测速记录只存主机上实际看到的东西）。
// 所有者可以在测速页关掉这项查询；关掉后报告里只有地址。内网地址从来不发出去。
// 自治系统号也用来认线路：比地址段表更全，查到了就和地址段一起参与推测。

// GeoService 是查地区用的服务（免费接口只有 HTTP，按来源 IP 限速：每分钟 15 次批量查询）。
const GeoService = "ip-api.com"

const (
	geoURL    = "http://ip-api.com/batch?fields=status,country,countryCode,regionName,city,isp,org,as,query&lang="
	geoTTL    = 24 * time.Hour
	geoMax    = 4096 // 缓存最多记这么多条，满了整个清掉
	geoBatch  = 100  // 接口一次最多查 100 个
	geoKey    = "speedtest.geo"
	geoFields = 80 // 每个字段最多留这么多字符
)

// GeoInfo 是一个公网地址的大致位置和运营商。都是第三方给的文字，只按纯文本显示。
type GeoInfo struct {
	Country string `json:"country,omitempty"`
	// CC 是国家或地区的两位代码。页面用它按界面语言显示国家名：查询服务返回的中文名有简有繁，不统一。
	CC     string `json:"cc,omitempty"`
	Region string `json:"region,omitempty"`
	City   string `json:"city,omitempty"`
	ISP    string `json:"isp,omitempty"`
	AS     string `json:"as,omitempty"`  // 例如 "AS4809 China Telecom Next Generation Carrier Network"
	Net    string `json:"net,omitempty"` // 按自治系统号认出来的骨干网，见 asNets
}

// asNets 是用来认线路的自治系统号（各运营商公开的骨干网编号）。
// AS4134 不等于 163 骨干：它涵盖整个中国电信，各省的城域网也在里面，所以只标成“电信”（CHINANET），
// 判断是不是走了 163 骨干只认骨干网段（见 backbones 里的 202.97.0.0/16）。 hygiene:allow
var asNets = map[int]string{
	4809:  "CN2",
	4134:  "CHINANET",
	23764: "CTGNet", // 中国电信国际（CTG）的海外网络
	9929:  "9929",
	10099: "9929", // 联通国际（CUG），和 9929 一样算精品网
	4837:  "4837",
	58453: "CMI",
	58807: "CMIN2", // 移动国际的精品网
	9808:  "CMNET",
}

type geoEntry struct {
	info GeoInfo
	at   time.Time
}

type geoCache struct {
	mu     sync.Mutex
	m      map[string]geoEntry // 键是 "语言 地址"
	client *http.Client
	url    string
}

func newGeoCache() *geoCache {
	return &geoCache{m: map[string]geoEntry{}, client: &http.Client{Timeout: 8 * time.Second}, url: geoURL}
}

// GeoEnabled 报告所有者有没有打开节点地区查询。没设过时是打开的。
func (s *Service) GeoEnabled() bool {
	var v string
	if s.db.QueryRow("SELECT value FROM settings WHERE key = ?", geoKey).Scan(&v) == nil {
		return v != "off"
	}
	return true
}

// SetGeoEnabled 打开或关掉节点地区查询。
func (s *Service) SetGeoEnabled(ctx context.Context, on bool) error {
	v := "off"
	if on {
		v = "on"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, geoKey, v)
	return err
}

// publicAddr 报告一个地址能不能拿去查地区：内网、回环、链路本地、运营商级 NAT、组播都不查。
func publicAddr(ip string) bool {
	a, err := netip.ParseAddr(ip)
	return err == nil && a.IsGlobalUnicast() && netOf(a) != "LAN"
}

// Geo 查一组地址的地区。lang 是 ip-api 认识的语言代码（见语言清单里的 geo）。查不到的地址不在结果里。
func (s *Service) Geo(ctx context.Context, ips []string, lang string) (map[string]GeoInfo, error) {
	g := s.geo
	out := map[string]GeoInfo{}
	var ask []string
	seen := map[string]bool{}
	g.mu.Lock()
	for _, ip := range ips {
		if seen[ip] || !publicAddr(ip) {
			continue
		}
		seen[ip] = true
		if e, ok := g.m[lang+" "+ip]; ok && time.Since(e.at) < geoTTL {
			out[ip] = e.info
		} else {
			ask = append(ask, ip)
		}
	}
	g.mu.Unlock()
	for len(ask) > 0 {
		n := min(len(ask), geoBatch)
		got, err := g.fetch(ctx, ask[:n], lang)
		if err != nil {
			return out, err
		}
		g.mu.Lock()
		if len(g.m)+len(got) > geoMax {
			g.m = map[string]geoEntry{}
		}
		for ip, info := range got {
			g.m[lang+" "+ip] = geoEntry{info, time.Now()}
			out[ip] = info
		}
		g.mu.Unlock()
		ask = ask[n:]
	}
	return out, nil
}

func (g *geoCache) fetch(ctx context.Context, ips []string, lang string) (map[string]GeoInfo, error) {
	body, _ := json.Marshal(ips)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url+lang, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, errors.New("查询节点地区失败（连不上 " + GeoService + "）")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, errors.New("查询节点地区太频繁，过一分钟再试")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("查询节点地区失败（" + GeoService + " 返回 " + strconv.Itoa(resp.StatusCode) + "）")
	}
	var list []struct {
		Status, Country, CountryCode, RegionName, City, ISP, Org, AS, Query string
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list); err != nil {
		return nil, errors.New("查询节点地区失败（返回的内容看不懂）")
	}
	want := map[string]bool{}
	for _, ip := range ips {
		want[ip] = true
	}
	out := map[string]GeoInfo{}
	for _, x := range list {
		if x.Status != "success" || !want[x.Query] {
			continue
		}
		isp := x.ISP
		if isp == "" {
			isp = x.Org
		}
		cc := strings.ToUpper(strings.TrimSpace(x.CountryCode))
		if len(cc) != 2 {
			cc = ""
		}
		info := GeoInfo{Country: clean(x.Country), CC: cc, Region: clean(x.RegionName), City: clean(x.City), ISP: clean(isp), AS: clean(x.AS)}
		if f := strings.Fields(info.AS); len(f) > 0 {
			if n, err := strconv.Atoi(strings.TrimPrefix(f[0], "AS")); err == nil {
				info.Net = asNets[n]
			}
		}
		out[x.Query] = info
	}
	return out, nil
}

// clean 只留可打印字符，截短。
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > geoFields {
		s = string(r[:geoFields])
	}
	return s
}

// Relabel 用查到的自治系统号补上地址段表认不出的骨干网，重新推测每条路由的线路。
func Relabel(routes []Route, geo map[string]GeoInfo) []Route {
	out := make([]Route, len(routes))
	for i, r := range routes {
		r.Hops = append([]Hop(nil), r.Hops...)
		for j := range r.Hops {
			h := &r.Hops[j]
			switch g := geo[h.IP].Net; {
			case h.Net == "":
				h.Net = g
			case h.Net == "CMI" && g == "CMIN2":
				// 移动国际的普通网和精品网共用 223.120.0.0/16 这些地址段，只能靠自治系统号（AS58807）分开。 hygiene:allow
				h.Net = g
			}
		}
		r.Nets = []string{}
		out[i] = r
	}
	finishRoutes(out)
	return out
}
