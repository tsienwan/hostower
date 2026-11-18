package risk

import (
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// compose 文件只是“写一个文件”，但 docker compose up 会照着它给容器开权限。
// 所以用 heredoc 写进 compose 文件的内容也要判级：能让容器拿到宿主机级权限的设置一律 L2。
// 这里只会加严：认不出、解析不了、看不全的内容都按 L2 处理。

var composeNameRe = regexp.MustCompile(`^(docker-)?compose(\.[a-z0-9_-]+)?\.ya?ml$`)

// isComposeFile 判断一个路径是不是 compose 文件（compose.yaml、docker-compose.yml、compose.prod.yaml……）。
func isComposeFile(p string) bool { return composeNameRe.MatchString(strings.ToLower(path.Base(p))) }

// 挂进容器后等同交出宿主机的目录。
var sensitiveMountRes = compileAll(
	`^/$`, `^/(etc|root|boot|proc|sys|dev|usr|bin|sbin|lib|lib64|run|var/run|var/lib/docker|home)(/|$)`,
	`docker\.sock$`, `containerd\.sock$`, `podman\.sock$`, `^~/?$`, `^~/\.ssh(/|$)`,
)

// namespace 类设置：值是 host 就和宿主机共用。
var composeHostNS = []string{"network_mode", "pid", "ipc", "uts", "userns_mode", "cgroup"}

var composeHostNSReason = map[string]string{
	"network_mode": "容器直接使用宿主机的网络（能监听主机的任意端口）",
	"pid":          "容器能看到并操作宿主机的进程",
	"ipc":          "容器和宿主机共用进程间通信",
	"uts":          "容器能改宿主机的主机名",
	"userns_mode":  "容器不做用户隔离",
	"cgroup":       "容器和宿主机共用 cgroup",
}

// ComposeFindings 检查一份 compose 文件的内容，返回会让容器拿到宿主机级权限的设置。
// 没有问题时返回空。只读，不会执行任何东西。
func ComposeFindings(body string) []Finding {
	l2 := func(cmd, reason string) Finding {
		return Finding{Command: cmd, Level: L2, Rule: "compose.escape", Reason: reason}
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return []Finding{l2("compose", "compose 文件无法解析，无法判定")}
	}
	var out []Finding
	if _, ok := doc["include"]; ok {
		out = append(out, l2("include", "compose 文件引用了别的文件，看不到全部内容"))
	}
	services, _ := doc["services"].(map[string]any)
	if s, ok := doc["services"]; ok && services == nil && s != nil {
		return append(out, l2("services", "compose 文件的格式无法识别"))
	}
	for name, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			if raw != nil {
				out = append(out, l2(name, "compose 文件的格式无法识别"))
			}
			continue
		}
		at := func(k string) string { return name + "." + k }
		if _, ok := svc["extends"]; ok {
			out = append(out, l2(at("extends"), "compose 文件引用了别的文件，看不到全部内容"))
		}
		switch v := svc["privileged"].(type) {
		case nil:
		case bool:
			if v {
				out = append(out, l2(at("privileged"), "容器以特权模式运行（等同 root）"))
			}
		default:
			out = append(out, l2(at("privileged"), "安全相关的设置用了变量或无法识别的值，无法静态判定"))
		}
		for _, k := range composeHostNS {
			v, ok := svc[k]
			if !ok {
				continue
			}
			s, isStr := v.(string)
			switch {
			case !isStr || strings.Contains(s, "$"):
				out = append(out, l2(at(k), "安全相关的设置用了变量或无法识别的值，无法静态判定"))
			case s == "host":
				out = append(out, l2(at(k)+": host", composeHostNSReason[k]))
			case strings.HasPrefix(s, "container:") || strings.HasPrefix(s, "service:"):
				// 加入别的容器的命名空间：权限不超过那个容器本身。
			}
		}
		if v, ok := svc["cap_add"]; ok && !emptyList(v) {
			out = append(out, l2(at("cap_add"), "给容器增加了内核能力"))
		}
		if v, ok := svc["devices"]; ok && !emptyList(v) {
			out = append(out, l2(at("devices"), "容器直接访问宿主机设备"))
		}
		if v, ok := svc["security_opt"]; ok {
			for _, o := range list(v) {
				s, _ := o.(string)
				if s == "" || strings.Contains(s, "$") || strings.Contains(s, "unconfined") || strings.Contains(s, "disable") {
					out = append(out, l2(at("security_opt"), "关闭了容器的安全隔离"))
					break
				}
			}
		}
		if v, ok := svc["volumes"]; ok {
			for _, m := range list(v) {
				src, ok := mountSource(m)
				switch {
				case !ok:
					out = append(out, l2(at("volumes"), "挂载的写法无法识别，无法静态判定"))
				case strings.Contains(src, "$"):
					out = append(out, l2(at("volumes")+" "+src, "挂载的宿主机路径用了变量，无法静态判定"))
				case strings.HasPrefix(src, "~") && matchAny(sensitiveMountRes, strings.TrimSuffix(src, "/")),
					strings.HasPrefix(src, "/") && matchAny(sensitiveMountRes, path.Clean(src)), strings.Contains(src, ".sock") && matchAny(sensitiveMountRes, src):
					out = append(out, l2(at("volumes")+" "+src, "把宿主机的敏感目录或 Docker 套接字挂进容器（等同 root）"))
				}
			}
		}
	}
	return out
}

func list(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{v}
}

func emptyList(v any) bool {
	l, ok := v.([]any)
	return v == nil || ok && len(l) == 0
}

// mountSource 取出一条挂载的宿主机一侧：短格式 "src:dst[:mode]"，长格式 {type, source, target}。
// 命名卷和相对路径原样返回（它们不会命中敏感目录）。
func mountSource(m any) (string, bool) {
	switch x := m.(type) {
	case string:
		src, _, found := strings.Cut(x, ":")
		if !found {
			return "", true // 只写容器内路径：匿名卷
		}
		return src, true
	case map[string]any:
		if t, _ := x["type"].(string); t != "" && t != "bind" && t != "volume" && t != "tmpfs" && t != "npipe" && t != "cluster" {
			return "", false
		}
		src, _ := x["source"].(string)
		if _, ok := x["source"]; ok && src == "" {
			return "", false
		}
		return src, true
	}
	return "", false
}
