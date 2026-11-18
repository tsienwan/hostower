package gateway

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/netcfg"
)

// 主机的网络入口：防火墙放行端口、Caddy 反向代理站点。没有自己的执行通道：
// 现状是内核的只读查询（检测脚本是 L0），改动都渲染成一条命令走判级和审批（改防火墙是 L2，改站点是 L1）。

// sshPortOf 是 Hostower 连这台主机用的 SSH 端口；本机（不走 SSH）返回 0，由检测脚本从 sshd_config 里读。
func sshPortOf(h inventory.Host) int {
	if h.Executor != "ssh" {
		return 0
	}
	if _, port, err := net.SplitHostPort(h.Addr); err == nil {
		if n, err := strconv.Atoi(port); err == nil {
			return n
		}
	}
	return 22
}

// keepPorts 是关不得的端口：SSH；Hostower 就装在这台主机上时，还有浏览器访问面板用的端口。
func (s *Server) keepPorts(h inventory.Host, st netcfg.Status) []netcfg.Port {
	keep := []netcfg.Port{{Port: st.SSHPort, Proto: "tcp"}}
	if h.Name == inventory.LocalName {
		if u, err := url.Parse(s.PublicURL); err == nil && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
			port := u.Port()
			if port == "" {
				port = map[string]string{"https": "443", "http": "80"}[u.Scheme]
			}
			if n, err := strconv.Atoi(port); err == nil && n != st.SSHPort {
				keep = append(keep, netcfg.Port{Port: n, Proto: "tcp"})
			}
		}
	}
	return keep
}

// detect 在主机上跑只读的检测脚本。
func (s *Server) detect(ctx context.Context, a kernel.Actor, name string) (inventory.Host, netcfg.Status, error) {
	h, ok := s.Kernel.Inventory().Host(name)
	if !ok {
		return h, netcfg.Status{}, kernel.ErrNotFound
	}
	out, _, err := s.Kernel.Query(ctx, a, name, netcfg.DetectCommand)
	if err != nil {
		return h, netcfg.Status{}, err
	}
	return h, netcfg.Parse(out, sshPortOf(h)), nil
}

func (s *Server) hostNetwork(w http.ResponseWriter, r *http.Request) {
	h, st, err := s.detect(r.Context(), actor(sessionOf(r)), r.PathValue("name"))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": st, "keep": s.keepPorts(h, st)})
}

type firewallBody struct {
	netcfg.FirewallRequest
	Preview bool `json:"preview"`
}

func (s *Server) hostFirewall(w http.ResponseWriter, r *http.Request) {
	var in firewallBody
	if !readJSON(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	// 以执行前一刻的现状为准（重新检测），不信页面传来的状态。
	h, st, err := s.detect(r.Context(), actor(sessionOf(r)), name)
	if err != nil {
		fail(w, err)
		return
	}
	keep := s.keepPorts(h, st)
	cmd, tool, err := netcfg.FirewallCommand(st, in.FirewallRequest, keep)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ports := make([]string, 0, len(in.Ports)+len(keep))
	list := in.Ports
	if in.Action == "setup" {
		list = append(append([]netcfg.Port{}, keep...), in.Ports...)
	}
	seen := map[string]bool{}
	for _, p := range list {
		if !seen[p.String()] {
			seen[p.String()] = true
			ports = append(ports, p.String())
		}
	}
	var intent string
	switch in.Action {
	case "setup":
		intent = "在 " + name + " 上启用防火墙 " + tool + "（放行 " + strings.Join(ports, "、") + "）"
	case "allow":
		intent = "在 " + name + " 的防火墙上放行 " + strings.Join(ports, "、")
	default:
		intent = "在 " + name + " 的防火墙上关闭 " + strings.Join(ports, "、")
	}
	s.submitPlan(w, r, []plannedTarget{{host: name, command: cmd}}, in.Preview, kernel.Strategy{}, func(int) string { return intent }, "")
}

type siteBody struct {
	Action  string `json:"action"` // add | remove | install-caddy
	Address string `json:"address"`
	Port    int    `json:"port"`
	Preview bool   `json:"preview"`
}

func (s *Server) hostSites(w http.ResponseWriter, r *http.Request) {
	var in siteBody
	if !readJSON(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	_, st, err := s.detect(r.Context(), actor(sessionOf(r)), name)
	if err != nil {
		fail(w, err)
		return
	}
	var cmd, intent string
	addr := strings.ToLower(strings.TrimSpace(in.Address))
	switch in.Action {
	case "add":
		cmd, err = netcfg.SiteAddCommand(st, addr, in.Port)
		intent = "在 " + name + " 上添加站点 " + addr + " → 127.0.0.1:" + strconv.Itoa(in.Port)
	case "remove":
		cmd, err = netcfg.SiteRemoveCommand(st, addr)
		intent = "在 " + name + " 上撤下站点 " + addr
	case "install-caddy":
		cmd, err = netcfg.CaddyInstallCommand(st)
		intent = "在 " + name + " 上安装 Caddy"
	default:
		writeErr(w, 400, "未知的操作")
		return
	}
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.submitPlan(w, r, []plannedTarget{{host: name, command: cmd}}, in.Preview, kernel.Strategy{}, func(int) string { return intent }, "")
}
