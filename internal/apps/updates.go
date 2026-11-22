package apps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Compare 比较两个版本号：数字段按数值比，其余按字符比；带预发布后缀（1.2.3-rc.1）的比正式版小。
// 清单的版本（7.4-1、lts-1）和 npm 的版本（0.169.0、0.2.2-rc.1）都能比。
func Compare(a, b string) int {
	if a == b {
		return 0
	}
	ma, pa := splitPre(a)
	mb, pb := splitPre(b)
	if c := compareTokens(tokens(ma), tokens(mb)); c != 0 {
		return c
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	return compareTokens(tokens(pa), tokens(pb))
}

// splitPre 只把 npm 风格的预发布后缀（x.y.z-…，后缀以字母开头）拆出来；清单的修订号（7.4-1）不算预发布。
func splitPre(v string) (string, string) {
	i := strings.IndexByte(v, '-')
	if i < 0 || i+1 >= len(v) || !unicode.IsLetter(rune(v[i+1])) {
		return v, ""
	}
	return v[:i], v[i+1:]
}

func tokens(v string) []string {
	var out []string
	cur := ""
	digit := false
	for _, r := range v {
		d := unicode.IsDigit(r)
		if !d && !unicode.IsLetter(r) {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		if cur != "" && d != digit {
			out = append(out, cur)
			cur = ""
		}
		cur += string(r)
		digit = d
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func compareTokens(a, b []string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		na, ea := strconv.Atoi(a[i])
		nb, eb := strconv.Atoi(b[i])
		switch {
		case ea == nil && eb == nil:
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
		case ea == nil:
			return 1 // 数字比字母新（1.10 > 1.beta）
		case eb == nil:
			return -1
		default:
			if c := strings.Compare(a[i], b[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}

// Registry 查 npm 上一个包的最新正式版本。结果缓存几个小时，查不到时返回空。
// 这是面板自己发出的请求，只带包名，不带任何主机或用户的信息。
type Registry struct {
	Base   string // 默认 https://registry.npmjs.org
	Client *http.Client
	TTL    time.Duration

	// RegistryBase 只给测试用：把镜像仓库的地址换成测试服务器。
	RegistryBase func(host string) string

	mu      sync.Mutex
	cache   map[string]latestEntry
	digests map[string]digestEntry
	// tagLists 是镜像仓库的标签列表缓存（固定版本标签找新版本用）。
	tagLists map[string]tagsEntry
}

type latestEntry struct {
	version string
	err     error // 查询失败：失败也缓存一段时间（failTTL），仓库连不上时不要每次打开页面都等一个超时
	at      time.Time
}

// failTTL 是查询失败的结果缓存多久。
const failTTL = 10 * time.Minute

// NewRegistry 创建 npm 版本查询。
func NewRegistry() *Registry {
	return &Registry{Base: "https://registry.npmjs.org", Client: &http.Client{Timeout: 10 * time.Second}, TTL: 6 * time.Hour, cache: map[string]latestEntry{}}
}

// Latest 返回包的最新版本（npm 的 latest 标签）。
func (r *Registry) Latest(ctx context.Context, pkg string) (string, error) {
	if !npmPkgRe.MatchString(pkg) {
		return "", errors.New("包名不合法")
	}
	r.mu.Lock()
	e, ok := r.cache[pkg]
	r.mu.Unlock()
	if ok && e.err != nil && time.Since(e.at) < failTTL {
		return "", e.err
	}
	if ok && e.err == nil && time.Since(e.at) < r.TTL {
		return e.version, nil
	}
	v, err := r.fetchLatest(ctx, pkg)
	if err != nil && ctx.Err() != nil {
		return "", err // 是调用方取消了，不算仓库的问题，不缓存
	}
	r.mu.Lock()
	r.cache[pkg] = latestEntry{version: v, err: err, at: time.Now()}
	r.mu.Unlock()
	return v, err
}

func (r *Registry) fetchLatest(ctx context.Context, pkg string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Base+"/"+pkg+"/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", errors.New("连不上 npm 仓库")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errors.New("npm 仓库返回 " + resp.Status)
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil || !VersionRe.MatchString(body.Version) || body.Version == "latest" {
		return "", errors.New("npm 仓库返回的版本号不对")
	}
	return body.Version, nil
}

// Target 是一个应用应该更新到的版本：容器应用是目录里的版本；命令行工具取目录版本和 npm 最新版本里较新的那个。
func (r *Registry) Target(ctx context.Context, a *App) string {
	if a.Type != TypeNpm {
		return a.Version
	}
	v := a.Npm.Version
	if r != nil {
		if l, err := r.Latest(ctx, a.Npm.Package); err == nil && Compare(l, v) > 0 {
			v = l
		}
	}
	return v
}
