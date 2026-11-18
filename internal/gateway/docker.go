package gateway

import (
	"net/http"

	"github.com/tsienwan/hostower/internal/apps"
	"github.com/tsienwan/hostower/internal/kernel"
)

// Docker 的检查和安装。检查是只读查询；安装只是生成一个请求（命令按这台主机的系统拼好），判级、审批、审计照常。

func (s *Server) dockerStatus(w http.ResponseWriter, r *http.Request) (apps.DockerStatus, bool) {
	name := r.PathValue("name")
	if _, ok := s.Kernel.Inventory().Host(name); !ok {
		writeErr(w, 404, "主机不存在")
		return apps.DockerStatus{}, false
	}
	out, _, err := s.Kernel.Query(r.Context(), actor(sessionOf(r)), name, apps.DockerStatusCommand)
	if err != nil {
		fail(w, err)
		return apps.DockerStatus{}, false
	}
	return apps.ParseDockerStatus(out), true
}

// hostDocker 报告这台主机的 Docker 情况，以及能不能由面板安装（不能的话给出原因）。
func (s *Server) hostDocker(w http.ResponseWriter, r *http.Request) {
	st, ok := s.dockerStatus(w, r)
	if !ok {
		return
	}
	resp := map[string]any{"status": st, "ready": st.Ready()}
	if !st.Ready() {
		if _, err := apps.DockerInstallCommand(st); err != nil {
			resp["blocked"] = err.Error()
		}
	}
	writeJSON(w, 200, resp)
}

// hostDockerInstall 生成“安装 Docker”的请求。
func (s *Server) hostDockerInstall(w http.ResponseWriter, r *http.Request) {
	st, ok := s.dockerStatus(w, r)
	if !ok {
		return
	}
	cmd, err := apps.DockerInstallCommand(st)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	name := r.PathValue("name")
	intent := "安装 Docker 和 Docker Compose"
	if st.Docker {
		intent = "安装 Docker Compose"
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), kernel.CommandSpec{
		Hosts: []string{name}, Commands: map[string]string{name: cmd}, Intent: intent,
	})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

// hostReconnect 断开到这台主机的连接再重新登录（登录账号刚加进某个组之后用）。
func (s *Server) hostReconnect(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	if err := s.Kernel.Reconnect(r.Context(), actor(sessionOf(r)), r.PathValue("name")); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
