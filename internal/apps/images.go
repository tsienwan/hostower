package apps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 镜像仓库查询：同一个标签（比如 louislam/uptime-kuma:2）在仓库里现在指向哪个摘要，以及上游有没有更新的大版本标签。
// 用的是镜像仓库的标准接口（Docker Hub、ghcr.io 等都一样）：匿名取一个只读令牌，再 HEAD 清单拿 Docker-Content-Digest。
// HEAD 请求不计入 Docker Hub 的拉取次数。这是面板自己发出的请求，只带镜像名，不带任何主机或用户的信息。

// ImageRef 是解析后的镜像地址。
type ImageRef struct {
	Registry string // 仓库地址，Docker Hub 是 registry-1.docker.io
	Repo     string // 例如 library/redis、louislam/uptime-kuma
	Tag      string
}

var refPartRe = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)

// ParseImage 解析 compose 里写的镜像地址（必须带标签）。
func ParseImage(s string) (ImageRef, error) {
	// 标签在最后一个冒号后面（仓库地址可以带端口：registry.example.com:5000/x:tag）。
	i := strings.LastIndex(s, ":")
	if i <= 0 || strings.Contains(s[i:], "/") || strings.Contains(s, "@") {
		return ImageRef{}, errors.New("镜像要写明标签")
	}
	name, tag := s[:i], s[i+1:]
	if tag == "" {
		return ImageRef{}, errors.New("镜像要写明标签")
	}
	parts := strings.Split(name, "/")
	ref := ImageRef{Registry: "registry-1.docker.io", Tag: tag}
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		ref.Registry, parts = parts[0], parts[1:]
	}
	if ref.Registry == "registry-1.docker.io" && len(parts) == 1 {
		parts = append([]string{"library"}, parts...)
	}
	for _, p := range parts {
		if !refPartRe.MatchString(p) {
			return ImageRef{}, fmt.Errorf("镜像地址 %q 不合法", s)
		}
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`).MatchString(tag) {
		return ImageRef{}, fmt.Errorf("镜像标签 %q 不合法", tag)
	}
	ref.Repo = strings.Join(parts, "/")
	return ref, nil
}

// NextMajorTag 推出“下一个大版本”的标签：2 → 3，8-alpine → 9-alpine，18-alpine → 19-alpine。
// 其他写法（v3.11.0、11.8、lts）推不出来，返回空。
func NextMajorTag(tag string) string {
	m := regexp.MustCompile(`^(\d+)(-[A-Za-z][A-Za-z0-9.-]*)?$`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return ""
	}
	return strconv.Itoa(n+1) + m[2]
}

// Images 是清单的 compose 里用到的镜像（按清单当前版本）。
func (a *App) Images() []string {
	if a.Type != TypeCompose {
		return nil
	}
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^\s*image:\s*"?([^"\s]+)"?\s*$`).FindAllStringSubmatch(a.Compose, -1) {
		out = append(out, m[1])
	}
	return out
}

type digestEntry struct {
	digest string
	ok     bool // false 表示标签不存在
	at     time.Time
}

const manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json"

var bearerParamRe = regexp.MustCompile(`(\w+)="([^"]*)"`)

// ImageDigest 返回标签在仓库里现在指向的摘要；标签不存在时 ok 为 false。结果缓存 TTL。
func (r *Registry) ImageDigest(ctx context.Context, image string) (digest string, ok bool, err error) {
	ref, err := ParseImage(image)
	if err != nil {
		return "", false, err
	}
	key := ref.Registry + "/" + ref.Repo + ":" + ref.Tag
	r.mu.Lock()
	if r.digests == nil {
		r.digests = map[string]digestEntry{}
	}
	e, hit := r.digests[key]
	r.mu.Unlock()
	if hit && time.Since(e.at) < r.TTL {
		return e.digest, e.ok, nil
	}
	digest, ok, err = r.headManifest(ctx, ref)
	if err != nil {
		return "", false, err
	}
	r.mu.Lock()
	r.digests[key] = digestEntry{digest: digest, ok: ok, at: time.Now()}
	r.mu.Unlock()
	return digest, ok, nil
}

func (r *Registry) registryBase(host string) string {
	if r.RegistryBase != nil {
		if b := r.RegistryBase(host); b != "" {
			return b
		}
	}
	return "https://" + host
}

func (r *Registry) headManifest(ctx context.Context, ref ImageRef) (string, bool, error) {
	u := r.registryBase(ref.Registry) + "/v2/" + ref.Repo + "/manifests/" + url.PathEscape(ref.Tag)
	do := func(token string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", manifestAccept)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return r.Client.Do(req)
	}
	resp, err := do("")
	if err != nil {
		return "", false, errors.New("连不上镜像仓库")
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		token, err := r.token(ctx, resp.Header.Get("WWW-Authenticate"))
		if err != nil {
			return "", false, err
		}
		if resp, err = do(token); err != nil {
			return "", false, errors.New("连不上镜像仓库")
		}
		resp.Body.Close()
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", false, nil
	case resp.StatusCode != http.StatusOK:
		return "", false, errors.New("镜像仓库返回 " + resp.Status)
	}
	d := resp.Header.Get("Docker-Content-Digest")
	if !strings.HasPrefix(d, "sha256:") {
		return "", false, errors.New("镜像仓库没有返回摘要")
	}
	return d, true, nil
}

// token 按 401 响应里的 WWW-Authenticate: Bearer realm=…,service=…,scope=… 匿名取一个只读令牌。
func (r *Registry) token(ctx context.Context, challenge string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", errors.New("镜像仓库要求的认证方式不支持")
	}
	p := map[string]string{}
	for _, m := range bearerParamRe.FindAllStringSubmatch(challenge, -1) {
		p[strings.ToLower(m[1])] = m[2]
	}
	realm, err := url.Parse(p["realm"])
	if err != nil || (realm.Scheme != "https" && r.RegistryBase == nil) || realm.Host == "" {
		return "", errors.New("镜像仓库的认证地址不对")
	}
	q := realm.Query()
	if p["service"] != "" {
		q.Set("service", p["service"])
	}
	if p["scope"] != "" {
		q.Set("scope", p["scope"])
	}
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", errors.New("连不上镜像仓库的认证服务")
	}
	defer resp.Body.Close()
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body) != nil {
		return "", errors.New("取不到镜像仓库的令牌")
	}
	if body.Token == "" {
		body.Token = body.AccessToken
	}
	return body.Token, nil
}

// ImageNews 是一个实例的镜像在仓库里的新情况。
type ImageNews struct {
	// Rebuilt 表示同一个标签在仓库里指向了新的构建（本地摘要和仓库不一样）。
	Rebuilt bool
	// Remote 是这些镜像在仓库里的摘要（拼起来），用来区分“同一个新构建”有没有提醒过。
	Remote string
	// Major 是上游出现、但目录还没收录的大版本标签（例如 uptime-kuma 的 3）。
	Major string
	// Newer 是固定版本标签在同一大版本内的新版本（例如 v3.9.0 → v3.11.0）：可以直接换标签升级。
	Newer string
}

// CheckImages 比较一个实例用到的镜像：local 是主机上 “仓库:标签 → 摘要”。主机上没有摘要的镜像（本地构建的、刚好被删的）跳过。
func (r *Registry) CheckImages(ctx context.Context, images []string, local map[string]string) ImageNews {
	var n ImageNews
	var remotes []string
	for _, img := range images {
		ld := local[img]
		if ld == "" {
			continue
		}
		rd, ok, err := r.ImageDigest(ctx, img)
		if err != nil || !ok {
			continue
		}
		remotes = append(remotes, rd)
		if rd != ld {
			n.Rebuilt = true
		}
		if ref, err := ParseImage(img); err == nil && n.Major == "" {
			if next := NextMajorTag(ref.Tag); next != "" {
				name := img[:len(img)-len(ref.Tag)]
				if _, ok, err := r.ImageDigest(ctx, name+next); err == nil && ok {
					n.Major = next
				}
			} else if _, ok := parseSemverTag(ref.Tag); ok {
				// 固定版本标签：读标签列表找更新的版本。
				if tags, err := r.Tags(ctx, img); err == nil {
					same, major := NewerTags(ref.Tag, tags)
					if n.Newer == "" {
						n.Newer = same
					}
					if major != "" {
						n.Major = major
					}
				}
			}
		}
	}
	n.Remote = strings.Join(remotes, ",")
	return n
}
