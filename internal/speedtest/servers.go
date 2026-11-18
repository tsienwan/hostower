package speedtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 测速服务器：搜索 Ookla 的服务器、记下所有者有没有接受 Ookla 的许可协议。
//
// 搜索是面板向 speedtest.net 查询（只发搜索词），结果只在内存里缓存一小时。
// speedtest.net 的搜索接口不返回中国大陆的服务器；这些服务器仍然可以直接填编号来测。

// ServerSearchService 是搜索服务器用的接口所在的网站。
const ServerSearchService = "www.speedtest.net"

const (
	serverSearchURL = "https://www.speedtest.net/api/js/servers?engine=js&limit=30&search="
	serverSearchTTL = time.Hour
	ooklaKey        = "speedtest.ookla.license"
)

// ServerInfo 是搜到的一台测速服务器。都是第三方给的文字，只按纯文本显示。
type ServerInfo struct {
	ID       int    `json:"id"`
	Sponsor  string `json:"sponsor"`
	Name     string `json:"name"`
	Country  string `json:"country"`
	CC       string `json:"cc,omitempty"`
	Host     string `json:"host,omitempty"`
	Distance int    `json:"distance,omitempty"` // 千米，按面板所在位置算的
}

type serverCache struct {
	mu     sync.Mutex
	m      map[string]serverEntry
	client *http.Client
	url    string
}

type serverEntry struct {
	list []ServerInfo
	at   time.Time
}

func newServerCache() *serverCache {
	return &serverCache{m: map[string]serverEntry{}, client: &http.Client{Timeout: 10 * time.Second}, url: serverSearchURL}
}

// SearchServers 按名字、城市或运营商搜索 Ookla 的测速服务器。
func (s *Service) SearchServers(ctx context.Context, q string) ([]ServerInfo, error) {
	q = strings.TrimSpace(q)
	if q == "" || len([]rune(q)) > 40 {
		return nil, errors.New("搜索词应该是 1 到 40 个字")
	}
	key := strings.ToLower(q)
	c := s.servers
	c.mu.Lock()
	if e, ok := c.m[key]; ok && time.Since(e.at) < serverSearchTTL {
		c.mu.Unlock()
		return e.list, nil
	}
	c.mu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+url.QueryEscape(q), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.New("搜索测速服务器失败（连不上 " + ServerSearchService + "）")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("搜索测速服务器失败（" + ServerSearchService + " 返回 " + strconv.Itoa(resp.StatusCode) + "）")
	}
	var raw []struct {
		ID, Sponsor, Name, Country, CC, Host string
		Distance                             float64
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, errors.New("搜索测速服务器失败（返回的内容看不懂）")
	}
	list := []ServerInfo{}
	for _, x := range raw {
		id, err := strconv.Atoi(x.ID)
		if err != nil || id <= 0 || id > MaxServerID {
			continue
		}
		list = append(list, ServerInfo{ID: id, Sponsor: clean(x.Sponsor), Name: clean(x.Name), Country: clean(x.Country), CC: clean(x.CC),
			Host: clean(x.Host), Distance: int(fin(x.Distance))})
	}
	c.mu.Lock()
	if len(c.m) > 512 {
		c.m = map[string]serverEntry{}
	}
	c.m[key] = serverEntry{list, time.Now()}
	c.mu.Unlock()
	return list, nil
}

// OoklaLicense 是所有者接受 Ookla 许可协议和隐私政策的记录。
type OoklaLicense struct {
	Accepted bool   `json:"accepted"`
	By       string `json:"by,omitempty"`
	At       string `json:"at,omitempty"`
	Version  string `json:"version"` // 程序版本
	Terms    string `json:"terms"`
	Privacy  string `json:"privacy"`
}

// OoklaLicense 返回所有者有没有接受 Ookla 的许可协议和隐私政策。
func (s *Service) OoklaLicense() OoklaLicense {
	l := OoklaLicense{}
	var v string
	if s.db.QueryRow("SELECT value FROM settings WHERE key = ?", ooklaKey).Scan(&v) == nil {
		json.Unmarshal([]byte(v), &l)
	}
	l.Version, l.Terms, l.Privacy = OoklaVersion, OoklaTerms, OoklaPrivacy
	return l
}

// SetOoklaLicense 记下所有者接受（或撤回）Ookla 的许可协议和隐私政策。
func (s *Service) SetOoklaLicense(ctx context.Context, accept bool, by string) error {
	l := OoklaLicense{}
	if accept {
		l = OoklaLicense{Accepted: true, By: by, At: time.Now().UTC().Format(time.RFC3339)}
	}
	v, _ := json.Marshal(l)
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, ooklaKey, string(v))
	return err
}
