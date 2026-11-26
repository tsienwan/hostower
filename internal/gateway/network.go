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

// 主机的网络入口：防火墙放行端口、Caddy / Nginx 站点和证书、Fail2ban。没有自己的执行通道：
// 现状是内核的只读查询（检测脚本和看日志是 L0），改动都渲染成一条命令走判级和审批
// （改防火墙和 Fail2ban 是 L2，改站点、申请和续期证书是 L1）。

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
	Action string `json:"action"` // add | remove | install-caddy | install-nginx | start-caddy | start-nginx
	netcfg.SiteRequest
	Preview bool `json:"preview"`
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
		var site netcfg.Site
		cmd, site, err = netcfg.SiteAddCommand(st, in.SiteRequest)
		switch site.Kind {
		case "static":
			intent = "在 " + name + " 上添加静态站点 " + site.Address + "（" + site.Target + "）"
		case "redirect":
			intent = "在 " + name + " 上添加跳转 " + site.Address + " → " + site.Target
		default:
			intent = "在 " + name + " 上添加站点 " + site.Address + " → " + site.Target
		}
	case "remove":
		cmd, err = netcfg.SiteRemoveCommand(st, addr)
		intent = "在 " + name + " 上撤下站点 " + addr
	case "install-caddy":
		cmd, err = netcfg.CaddyInstallCommand(st)
		intent = "在 " + name + " 上安装 Caddy"
	case "install-nginx":
		cmd, err = netcfg.NginxInstallCommand(st)
		intent = "在 " + name + " 上安装 Nginx"
	case "start-caddy":
		cmd, err = netcfg.ServerStartCommand(st, "caddy")
		intent = "在 " + name + " 上启动 Caddy"
	case "start-nginx":
		cmd, err = netcfg.ServerStartCommand(st, "nginx")
		intent = "在 " + name + " 上启动 Nginx"
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

type certBody struct {
	netcfg.CertRequest
	Preview bool `json:"preview"`
}

// hostCerts 续期 certbot 管理的证书（演练、续期快到期的、强制续期一张）。
func (s *Server) hostCerts(w http.ResponseWriter, r *http.Request) {
	var in certBody
	if !readJSON(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	_, st, err := s.detect(r.Context(), actor(sessionOf(r)), name)
	if err != nil {
		fail(w, err)
		return
	}
	cmd, cert, err := netcfg.CertCommand(st, in.CertRequest)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	intent := map[string]string{"dry-run": "在 " + name + " 上演练证书续期", "renew": "在 " + name + " 上续期快到期的证书", "force": "在 " + name + " 上强制续期证书 " + cert}[in.Action]
	s.submitPlan(w, r, []plannedTarget{{host: name, command: cmd}}, in.Preview, kernel.Strategy{}, func(int) string { return intent }, "")
}

type f2bBody struct {
	netcfg.F2BRequest
	Preview bool `json:"preview"`
}

// hostFail2ban 安装 Fail2ban、改设置、解封 IP。都是 L2：它管着防火墙的封禁规则。
func (s *Server) hostFail2ban(w http.ResponseWriter, r *http.Request) {
	var in f2bBody
	if !readJSON(w, r, &in) {
		return
	}
	name := r.PathValue("name")
	_, st, err := s.detect(r.Context(), actor(sessionOf(r)), name)
	if err != nil {
		fail(w, err)
		return
	}
	cmd, err := netcfg.F2BCommand(st, in.F2BRequest)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var intent string
	switch in.Action {
	case "install":
		intent = "在 " + name + " 上安装 Fail2ban"
	case "config":
		intent = "修改 " + name + " 上的 Fail2ban 设置"
	default:
		intent = "在 " + name + " 的 Fail2ban 里解封 " + strings.TrimSpace(in.IP)
	}
	s.submitPlan(w, r, []plannedTarget{{host: name, command: cmd}}, in.Preview, kernel.Strategy{}, func(int) string { return intent }, "")
}

// hostNetLog 看站点或 Fail2ban 的日志：只读查询，最后 200 行。
func (s *Server) hostNetLog(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	a := actor(sessionOf(r))
	q := r.URL.Query()
	if _, ok := s.Kernel.Inventory().Host(name); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	cmd := netcfg.F2BLogCommand
	if q.Get("kind") != "fail2ban" {
		var err error
		if cmd, err = netcfg.SiteLogCommand(q.Get("server"), q.Get("address"), q.Get("kind")); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	out, _, err := s.Kernel.Query(r.Context(), a, name, cmd)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"text": out})
}
