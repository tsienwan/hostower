package gateway

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// 容器页的更新检测：每个容器用的镜像，拿主机上的摘要和镜像仓库里同一标签的摘要比对（只读查询 + 面板去仓库查）。
// 应用市场装的容器另有应用页的版本信息，页面会一起显示；这里只管“镜像本身”。

// ctrUpdatesCommand 列出容器（名字、镜像、Compose 项目）和本机镜像的摘要。必须是 L0。
const ctrUpdatesCommand = `docker ps -a --format '{{.Names}}	{{.Image}}	{{.Label "com.docker.compose.project"}}' 2>/dev/null; echo '---'; ` +
	`docker images --digests --format '{{.Repository}}:{{.Tag}}	{{.Digest}}' 2>/dev/null`

type imageNews struct {
	Image    string `json:"image"`
	Rebuilt  bool   `json:"rebuilt,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	// Newer 是固定版本标签在同一大版本内的新版本（v3.9.0 → v3.11.0），Image 换成它就是升级后的镜像。
	Newer      string `json:"newer,omitempty"`
	NewerImage string `json:"newerImage,omitempty"`
}

var taggedRe = regexp.MustCompile(`:[A-Za-z0-9_][A-Za-z0-9._-]*$`)

// normalizeImage 补上隐含的 latest 标签（docker images 里显示的就是 :latest）；按摘要固定的镜像返回空（不会有“新构建”）。
func normalizeImage(img string) string {
	if strings.Contains(img, "@") || strings.HasPrefix(img, "sha256:") {
		return ""
	}
	last := img[strings.LastIndex(img, "/")+1:]
	if !taggedRe.MatchString(last) {
		return img + ":latest"
	}
	return img
}

func (s *Server) containerUpdates(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	out, _, err := s.Kernel.Query(r.Context(), actor(sessionOf(r)), host, ctrUpdatesCommand)
	if err != nil {
		fail(w, err)
		return
	}
	head, tail, _ := strings.Cut(out, "---\n")
	local := map[string]string{}
	for _, l := range strings.Split(tail, "\n") {
		img, d, ok := strings.Cut(strings.TrimSpace(l), "\t")
		if ok && strings.HasPrefix(d, "sha256:") {
			local[img] = d
		}
	}
	byImage := map[string]*imageNews{}
	ctrs := map[string]string{} // 容器名 → 镜像（规范化后）
	for _, l := range strings.Split(head, "\n") {
		f := strings.Split(strings.TrimRight(l, "\r"), "\t")
		if len(f) < 2 || f[0] == "" {
			continue
		}
		img := normalizeImage(f[1])
		if img == "" {
			continue
		}
		ctrs[f[0]] = img
		if byImage[img] == nil {
			byImage[img] = &imageNews{Image: img}
		}
	}
	imgs := make([]string, 0, len(byImage))
	for k := range byImage {
		imgs = append(imgs, k)
	}
	sort.Strings(imgs)
	for _, img := range imgs {
		n := s.appRegistry.CheckImages(r.Context(), []string{img}, local)
		byImage[img].Rebuilt, byImage[img].Upstream = n.Rebuilt, n.Major
		if n.Newer != "" {
			byImage[img].Newer = n.Newer
			byImage[img].NewerImage = img[:strings.LastIndex(img, ":")+1] + n.Newer
			// 有更新的版本时，“新构建”就不重要了：直接升级到新版本。
			byImage[img].Rebuilt = false
		}
	}
	res := map[string]imageNews{}
	for name, img := range ctrs {
		if n := byImage[img]; n.Rebuilt || n.Upstream != "" || n.Newer != "" {
			res[name] = *n
		}
	}
	writeJSON(w, 200, map[string]any{"containers": res})
}
