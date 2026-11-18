package gateway

import (
	"encoding/json"
	"github.com/tsienwan/hostower/internal/apps"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/tsienwan/hostower/internal/snippet"
)

// 容器的配置详情：一条只读的 docker inspect，整理成页面要的字段。
// 环境变量里常有口令，所以值在服务端就遮掉，页面和接口响应里都看不到原文。

type ctrPort struct {
	Container string `json:"container"`
	Proto     string `json:"proto"`
	HostIP    string `json:"hostIp,omitempty"`
	HostPort  string `json:"hostPort,omitempty"`
}

type ctrMount struct {
	Type        string `json:"type"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readOnly"`
}

type ctrEnv struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Masked bool   `json:"masked"`
	// FromImage 表示这个变量是镜像自带的、没有被改过：重建容器时不用再写一遍。
	FromImage bool `json:"fromImage"`
}

type ctrNetwork struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

type ctrDetail struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Image        string       `json:"image"`
	ImageID      string       `json:"imageId"`
	Created      string       `json:"created"`
	Status       string       `json:"status"`
	Health       string       `json:"health,omitempty"`
	StartedAt    string       `json:"startedAt,omitempty"`
	FinishedAt   string       `json:"finishedAt,omitempty"`
	ExitCode     int          `json:"exitCode"`
	OOMKilled    bool         `json:"oomKilled"`
	RestartCount int          `json:"restartCount"`
	Restart      string       `json:"restart"`
	Command      []string     `json:"command"`
	WorkDir      string       `json:"workDir,omitempty"`
	User         string       `json:"user,omitempty"`
	Privileged   bool         `json:"privileged"`
	NetworkMode  string       `json:"networkMode,omitempty"`
	MemoryLimit  int64        `json:"memoryLimit"`
	CPULimit     float64      `json:"cpuLimit"`
	Project      string       `json:"project,omitempty"`
	Service      string       `json:"service,omitempty"`
	Ports        []ctrPort    `json:"ports"`
	Mounts       []ctrMount   `json:"mounts"`
	Env          []ctrEnv     `json:"env"`
	Networks     []ctrNetwork `json:"networks"`
	Labels       int          `json:"labels"`
	// App 是应用市场的实例名（容器带应用标签时）：这种容器要到应用页改设置，在容器页重建会让它脱离管理。
	App string `json:"app,omitempty"`
	// rawEnv 只在服务端用来和镜像自带的变量比对，不输出。
	rawEnv []string
}

var (
	secretKeyRe = regexp.MustCompile(`(?i)(pass|secret|token|key|credential|auth|dsn|private|cookie|session)`)
	urlCredRe   = regexp.MustCompile(`(://[^:/@\s]*):[^@\s]+@`)
)

// maskEnv 遮掉看起来像口令的环境变量；连接串里的口令也遮掉。这是尽力而为，不保证认得出所有口令。
func maskEnv(kv string) ctrEnv {
	k, v, _ := strings.Cut(kv, "=")
	if v != "" && secretKeyRe.MatchString(k) {
		return ctrEnv{Key: k, Value: "", Masked: true}
	}
	if urlCredRe.MatchString(v) {
		return ctrEnv{Key: k, Value: urlCredRe.ReplaceAllString(v, "$1:••••••@"), Masked: true}
	}
	return ctrEnv{Key: k, Value: v}
}

func parseInspect(out string) (*ctrDetail, bool) {
	var raw []struct {
		ID           string `json:"Id"`
		Name         string
		Created      string
		Image        string
		RestartCount int
		State        struct {
			Status, StartedAt, FinishedAt string
			ExitCode                      int
			OOMKilled                     bool
			Health                        *struct{ Status string }
		}
		Config struct {
			Image      string
			Env        []string
			Cmd        []string
			Entrypoint []string
			WorkingDir string
			User       string
			Labels     map[string]string
		}
		HostConfig struct {
			RestartPolicy struct{ Name string }
			Privileged    bool
			NetworkMode   string
			Memory        int64
			NanoCpus      int64
		}
		Mounts []struct {
			Type, Name, Source, Destination string
			RW                              bool
		}
		NetworkSettings struct {
			Ports    map[string][]struct{ HostIp, HostPort string }
			Networks map[string]struct {
				IPAddress string
				Aliases   []string
			}
		}
	}
	if json.Unmarshal([]byte(out), &raw) != nil || len(raw) == 0 {
		return nil, false
	}
	c := raw[0]
	d := &ctrDetail{
		ID: c.ID, Name: strings.TrimPrefix(c.Name, "/"), Image: c.Config.Image, ImageID: c.Image, Created: c.Created,
		Status: c.State.Status, StartedAt: c.State.StartedAt, FinishedAt: c.State.FinishedAt, ExitCode: c.State.ExitCode,
		OOMKilled: c.State.OOMKilled, RestartCount: c.RestartCount, Restart: c.HostConfig.RestartPolicy.Name,
		Command: append(append([]string{}, c.Config.Entrypoint...), c.Config.Cmd...), WorkDir: c.Config.WorkingDir, User: c.Config.User,
		Privileged: c.HostConfig.Privileged, NetworkMode: c.HostConfig.NetworkMode, MemoryLimit: c.HostConfig.Memory,
		CPULimit: float64(c.HostConfig.NanoCpus) / 1e9, Project: c.Config.Labels["com.docker.compose.project"],
		Service: c.Config.Labels["com.docker.compose.service"], Labels: len(c.Config.Labels), App: c.Config.Labels[apps.LabelKey],
		Ports: []ctrPort{}, Mounts: []ctrMount{}, Env: []ctrEnv{}, Networks: []ctrNetwork{},
	}
	if c.State.Health != nil {
		d.Health = c.State.Health.Status
	}
	for spec, binds := range c.NetworkSettings.Ports {
		port, proto, _ := strings.Cut(spec, "/")
		if len(binds) == 0 {
			d.Ports = append(d.Ports, ctrPort{Container: port, Proto: proto})
		}
		for _, b := range binds {
			d.Ports = append(d.Ports, ctrPort{Container: port, Proto: proto, HostIP: b.HostIp, HostPort: b.HostPort})
		}
	}
	sort.Slice(d.Ports, func(i, j int) bool {
		a, b := d.Ports[i], d.Ports[j]
		if (a.HostPort != "") != (b.HostPort != "") {
			return a.HostPort != ""
		}
		if len(a.Container) != len(b.Container) {
			return len(a.Container) < len(b.Container)
		}
		if a.Container != b.Container {
			return a.Container < b.Container
		}
		return a.HostIP < b.HostIP
	})
	for _, m := range c.Mounts {
		src := m.Source
		if m.Type == "volume" && m.Name != "" {
			src = m.Name
		}
		d.Mounts = append(d.Mounts, ctrMount{Type: m.Type, Source: src, Destination: m.Destination, ReadOnly: !m.RW})
	}
	sort.Slice(d.Mounts, func(i, j int) bool { return d.Mounts[i].Destination < d.Mounts[j].Destination })
	for _, e := range c.Config.Env {
		d.Env = append(d.Env, maskEnv(e))
		d.rawEnv = append(d.rawEnv, e)
	}
	sort.Strings(d.rawEnv)
	sort.Slice(d.Env, func(i, j int) bool { return d.Env[i].Key < d.Env[j].Key })
	for name, n := range c.NetworkSettings.Networks {
		d.Networks = append(d.Networks, ctrNetwork{Name: name, IP: n.IPAddress, Aliases: n.Aliases})
	}
	sort.Slice(d.Networks, func(i, j int) bool { return d.Networks[i].Name < d.Networks[j].Name })
	return d, true
}

func (s *Server) containerInspect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("name")
	if !ctrNameRe.MatchString(name) {
		writeErr(w, 400, "容器名不合法")
		return
	}
	out, code, err := s.Kernel.Query(r.Context(), actor(sessionOf(r)), q.Get("host"), "docker inspect --type container "+snippet.Quote(name))
	if err != nil {
		fail(w, err)
		return
	}
	d, ok := parseInspect(out)
	if code != 0 || !ok {
		writeErr(w, 404, "读不到这个容器的配置")
		return
	}
	// 再看一眼镜像自带哪些环境变量：和它一模一样的就是没改过的默认值。
	if imageIDRe.MatchString(d.ImageID) {
		if img, icode, ierr := s.Kernel.Query(r.Context(), actor(sessionOf(r)), q.Get("host"), "docker image inspect "+d.ImageID+" --format '{{json .Config.Env}}'"); ierr == nil && icode == 0 {
			markImageEnv(d, img)
		}
	}
	writeJSON(w, 200, d)
}

var imageIDRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func markImageEnv(d *ctrDetail, out string) {
	var base []string
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &base) != nil {
		return
	}
	same := map[string]bool{}
	for _, e := range base {
		same[e] = true
	}
	for _, raw := range d.rawEnv {
		if !same[raw] {
			continue
		}
		k, _, _ := strings.Cut(raw, "=")
		for i := range d.Env {
			if d.Env[i].Key == k {
				d.Env[i].FromImage = true
			}
		}
	}
}
