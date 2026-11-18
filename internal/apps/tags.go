package apps

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 固定版本标签（v3.9.0、1.2.3）的新版本：这种标签发布后不会再变，看“新构建”永远看不到更新，
// 要读仓库的标签列表，找出同一种写法里更新的版本。

var semverTagRe = regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)$`)

type semver struct {
	prefix              string
	major, minor, patch int
}

func parseSemverTag(tag string) (semver, bool) {
	m := semverTagRe.FindStringSubmatch(tag)
	if m == nil {
		return semver{}, false
	}
	a, _ := strconv.Atoi(m[2])
	b, _ := strconv.Atoi(m[3])
	c, _ := strconv.Atoi(m[4])
	return semver{m[1], a, b, c}, true
}

func (v semver) less(o semver) bool {
	if v.major != o.major {
		return v.major < o.major
	}
	if v.minor != o.minor {
		return v.minor < o.minor
	}
	return v.patch < o.patch
}

func (v semver) String() string {
	return v.prefix + strconv.Itoa(v.major) + "." + strconv.Itoa(v.minor) + "." + strconv.Itoa(v.patch)
}

// NewerTags 在标签列表里找比 current 新的版本：同一大版本内最新的（可以直接升级），和更高大版本里最新的（只提示）。
// 只认和 current 同一种写法的标签（带不带 v 前缀一致）；预发布版本（-rc、-beta）不算。current 不是 x.y.z 写法时返回空。
func NewerTags(current string, tags []string) (sameMajor, newerMajor string) {
	cur, ok := parseSemverTag(current)
	if !ok {
		return "", ""
	}
	bestSame, bestNext := cur, cur
	for _, t := range tags {
		v, ok := parseSemverTag(t)
		if !ok || v.prefix != cur.prefix {
			continue
		}
		if v.major == cur.major && bestSame.less(v) {
			bestSame = v
		}
		if v.major > cur.major && bestNext.less(v) {
			bestNext = v
		}
	}
	if bestSame != cur {
		sameMajor = bestSame.String()
	}
	if bestNext != cur {
		newerMajor = bestNext.String()
	}
	return sameMajor, newerMajor
}

type tagsEntry struct {
	tags []string
	at   time.Time
}

// maxTagPages 限制翻页：标签特别多的仓库（几千个）只读前几页，够找出新版本了。
const maxTagPages = 10

var linkNextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// Tags 读一个镜像仓库的标签列表（标准接口 /v2/<repo>/tags/list，按 Link 头翻页）。结果缓存 TTL。
func (r *Registry) Tags(ctx context.Context, image string) ([]string, error) {
	ref, err := ParseImage(image)
	if err != nil {
		return nil, err
	}
	key := ref.Registry + "/" + ref.Repo
	r.mu.Lock()
	if r.tagLists == nil {
		r.tagLists = map[string]tagsEntry{}
	}
	e, hit := r.tagLists[key]
	r.mu.Unlock()
	if hit && time.Since(e.at) < r.TTL {
		return e.tags, nil
	}
	base := r.registryBase(ref.Registry)
	next := base + "/v2/" + ref.Repo + "/tags/list?n=1000"
	token := ""
	var all []string
	for page := 0; next != "" && page < maxTagPages; page++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := r.Client.Do(req)
		if err != nil {
			return nil, errors.New("连不上镜像仓库")
		}
		if resp.StatusCode == http.StatusUnauthorized && token == "" {
			resp.Body.Close()
			if token, err = r.token(ctx, resp.Header.Get("WWW-Authenticate")); err != nil {
				return nil, err
			}
			page--
			continue
		}
		var body struct {
			Tags []string `json:"tags"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&body)
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil {
			return nil, errors.New("读不到镜像仓库的标签列表")
		}
		all = append(all, body.Tags...)
		next = ""
		if m := linkNextRe.FindStringSubmatch(link); m != nil {
			if strings.HasPrefix(m[1], "/") {
				next = base + m[1]
			} else if strings.HasPrefix(m[1], base+"/") {
				next = m[1]
			}
		}
	}
	r.mu.Lock()
	r.tagLists[key] = tagsEntry{tags: all, at: time.Now()}
	r.mu.Unlock()
	return all, nil
}
