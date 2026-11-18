package executor

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tsienwan/hostower/internal/inventory"
)

// 模拟主机上的应用市场：记住装过哪些应用（app.json）和它们的容器，好在开发模式里演示安装、更新、启停、卸载。

type simApp struct {
	record   string   // app.json 的内容
	config   string   // config.json 的内容（AI 命令行工具的配置）
	services []string // 容器应用的服务名
	images   []string // 容器应用用到的镜像
	stopped  bool
}

var (
	simRecordRe   = regexp.MustCompile(`printf '%s\\n' '(\{[^']*\})' > app\.json`)
	simNpmRecRe   = regexp.MustCompile(`printf '(\{[^']*\})\\n' "\$iv"`)
	simNpmPkgRe   = regexp.MustCompile(`--no-audit '?(@?[^@'\s]+)@([^'\s]+)'? (?:</dev/null )?\|\|`)
	simAppDirRe   = regexp.MustCompile(`^cd ~/\.hostower/apps/([a-z0-9-]+) && docker compose -p [a-z0-9-]+ (start|stop|restart|down)`)
	simNpmUninsRe = regexp.MustCompile(`/apps/([a-z0-9-]+)/app\.json" "`)
	simConfigRe   = regexp.MustCompile(`printf '%s\\n' '(\{"instance":"([a-z0-9-]+)"[^']*\})' > "\$a/config\.json"`)
)

// applyConfig 记下 AI 工具的配置（config.json）；返回输出。
func (sh *simHost) applyConfig(cmd string) []string {
	m := simConfigRe.FindStringSubmatch(cmd)
	if m == nil {
		return nil
	}
	a := sh.appMap()[m[2]]
	if a == nil {
		return []string{"hostower: 这台主机上还没有装这个工具"}
	}
	a.config = m[1]
	out := []string{}
	if strings.Contains(cmd, "login --with-api-key") {
		out = append(out, "Successfully logged in")
	}
	return append(out, "hostower: 配置已写入，下次运行时生效")
}

func (sh *simHost) appMap() map[string]*simApp {
	if sh.apps == nil {
		sh.apps = map[string]*simApp{}
	}
	return sh.apps
}

// appCommand 处理应用市场生成的命令；不是这类命令时 ok 为 false。
func (s *Sim) appCommand(h inventory.Host, sh *simHost, cmd string) (lines []string, code int, ok bool) {
	switch {
	case strings.HasPrefix(cmd, "{ ") && strings.Contains(cmd, "\n} && { "):
		// 同一台主机上的几个实例：依次执行，前一个失败就停。
		for _, part := range strings.Split(cmd, "\n} && { ") {
			part = strings.TrimSuffix(strings.TrimPrefix(part, "{ "), "\n}")
			l, c, _ := s.appCommand(h, sh, part)
			lines = append(lines, l...)
			if c != 0 {
				return lines, c, true
			}
		}
		return lines, 0, true

	case strings.HasPrefix(cmd, "cat ~/.hostower/apps/*/app.json"):
		names := make([]string, 0, len(sh.apps))
		for n := range sh.apps {
			names = append(names, n)
		}
		sort.Strings(names)
		var tail []string
		for _, n := range names {
			a := sh.apps[n]
			lines = append(lines, a.record)
			for i, svc := range a.services {
				state, status, ports := "running", "Up 3 minutes", ""
				if a.stopped {
					state, status = "exited", "Exited (0) 1 minute ago"
				}
				if i == 0 && !a.stopped {
					ports = "127.0.0.1:" + simPort(a.record) + "->" + simPort(a.record) + "/tcp"
				}
				tail = append(tail, n+"\t"+n+"-"+svc+"-1\t"+state+"\t"+status+"\t"+ports)
			}
		}
		for _, n := range names {
			if c := sh.apps[n].config; c != "" {
				lines = append(lines, c)
			}
		}
		// 第三段：本机镜像的摘要。模拟的摘要和仓库里的不一样，所以开发模式里会显示“镜像有新构建”。
		tail = append(tail, "---")
		for _, n := range names {
			for _, img := range sh.apps[n].images {
				tail = append(tail, img+"	sha256:0000000000000000000000000000000000000000000000000000000000000000")
			}
		}
		return append(append(lines, "---"), tail...), 0, true

	case strings.HasPrefix(cmd, "docker compose version"):
		if !h.HasTag("docker") {
			return []string{"hostower: 这台主机没有 Docker Compose v2（docker compose 命令）"}, 3, true
		}
		m := simRecordRe.FindStringSubmatch(cmd)
		if m == nil {
			return []string{"模拟主机：看不懂这条应用命令"}, 1, true
		}
		var rec struct{ Instance, Version string }
		json.Unmarshal([]byte(m[1]), &rec)
		_, body, _ := strings.Cut(cmd, "\n")
		body, _, _ = strings.Cut(body, "HOSTOWER_EOF")
		var doc struct {
			Services map[string]struct {
				Image string `yaml:"image"`
			} `yaml:"services"`
		}
		yaml.Unmarshal([]byte(body), &doc)
		var svcs, imgs []string
		for k, v := range doc.Services {
			svcs = append(svcs, k)
			imgs = append(imgs, v.Image)
		}
		sort.Strings(svcs)
		apps := sh.appMap()
		_, existed := apps[rec.Instance]
		apps[rec.Instance] = &simApp{record: m[1], services: svcs, images: imgs}
		lines = []string{"[+] Pulling " + itoa(len(svcs)) + "/" + itoa(len(svcs))}
		for _, svc := range svcs {
			lines = append(lines, " ✔ "+svc+" Pulled                                    2.1s")
		}
		lines = append(lines, "[+] Running "+itoa(len(svcs)+1)+"/"+itoa(len(svcs)+1))
		if !existed {
			lines = append(lines, " ✔ Network "+rec.Instance+"_default  Created              0.1s")
		}
		for _, svc := range svcs {
			verb := "Started"
			if existed {
				verb = "Recreated"
			}
			lines = append(lines, " ✔ Container "+rec.Instance+"-"+svc+"-1  "+verb+"              0.8s")
		}
		return lines, 0, true

	case strings.HasPrefix(cmd, `umask 022; d="$HOME/.hostower"`):
		rm := simNpmRecRe.FindStringSubmatch(cmd)
		pm := simNpmPkgRe.FindStringSubmatch(cmd)
		if rm == nil || pm == nil {
			return []string{"模拟主机：看不懂这条应用命令"}, 1, true
		}
		version := pm[2]
		if version == "latest" {
			version = "99.0.0"
		}
		var rec struct{ Instance string }
		record := strings.Replace(strings.ReplaceAll(rm[1], "%%", "%"), "%s", version, 1)
		json.Unmarshal([]byte(record), &rec)
		if sh.seed > 0.5 {
			lines = append(lines, "hostower: 系统里没有足够新的 Node.js，下载 Node.js 24.22.3")
		}
		lines = append(lines, "", "added 1 package in 6s", "hostower: 已安装 "+pm[1]+" "+version)
		sh.appMap()[rec.Instance] = &simApp{record: record}
		lines = append(lines, sh.applyConfig(cmd)...)
		return lines, 0, true

	case strings.HasPrefix(cmd, `umask 077; a="$HOME/.hostower/apps/`) && strings.Contains(cmd, `config.json"`):
		out := sh.applyConfig(cmd)
		if out == nil {
			return []string{"模拟主机：看不懂这条配置命令"}, 1, true
		}
		if len(out) == 1 && strings.Contains(out[0], "还没有装") {
			return out, 3, true
		}
		return out, 0, true

	case strings.HasPrefix(cmd, `d="$HOME/.hostower"; `) && strings.Contains(cmd, "npm uninstall"):
		if m := simNpmUninsRe.FindStringSubmatch(cmd); m != nil {
			delete(sh.appMap(), m[1])
		}
		return []string{"", "removed 1 package in 1s"}, 0, true
	}
	if m := simAppDirRe.FindStringSubmatch(cmd); m != nil {
		a := sh.appMap()[m[1]]
		if a == nil {
			return []string{"bash: cd: " + "~/.hostower/apps/" + m[1] + ": No such file or directory"}, 1, true
		}
		switch m[2] {
		case "start", "restart":
			a.stopped = false
		case "stop":
			a.stopped = true
		case "down":
			delete(sh.apps, m[1])
		}
		for _, svc := range a.services {
			lines = append(lines, " ✔ Container "+m[1]+"-"+svc+"-1  Done              0.4s")
		}
		return lines, 0, true
	}
	return nil, 0, false
}

func simPort(record string) string {
	var r struct{ Params map[string]string }
	json.Unmarshal([]byte(record), &r)
	if p := r.Params["port"]; p != "" {
		return p
	}
	return "80"
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// appContainers 是应用市场装的容器在 docker ps 里的样子（带应用标签），给容器页用。
func (sh *simHost) appContainers() []string {
	var names []string
	for n := range sh.apps {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		a := sh.apps[n]
		for i, svc := range a.services {
			state, status, ports := "running", "Up 3 minutes", ""
			if a.stopped {
				state, status = "exited", "Exited (0) 1 minute ago"
			} else if i == 0 {
				ports = "127.0.0.1:" + simPort(a.record) + "->" + simPort(a.record) + "/tcp"
			}
			name := n // 和 apps.render 定的容器名一致：一个服务叫实例名，多个叫“实例名-服务名”
			if len(a.services) > 1 {
				name = n + "-" + svc
			}
			labels := "com.docker.compose.project=" + n + ",com.docker.compose.service=" + svc + ",com.hostower.app=" + n
			b, _ := json.Marshal(map[string]string{"ID": "a" + name, "Image": "example/" + svc + ":1", "Names": name, "Ports": ports, "State": state, "Status": status,
				"CreatedAt": "2026-11-17 09:00:00 +0000 UTC", "Labels": labels})
			out = append(out, string(b))
		}
	}
	return out
}

func sortedKeys(m map[string]*simApp) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
