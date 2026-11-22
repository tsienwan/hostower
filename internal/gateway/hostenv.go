package gateway

import (
	"net/http"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/hostenv"
)

// hostEnv 检测一台主机的 AI 环境：装了哪些命令、登录过没有，出口 IP 的归属和类型，
// 各家模型接口、常用网站和流媒体能不能用。两条只读查询（命令是 hostenv 里的常量，判级必须是 L0），并行执行。
func (s *Server) hostEnv(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.Kernel.Inventory().Host(name); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	a := actor(sessionOf(r))
	var tools, net string
	var terr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		tools, _, terr = s.Kernel.Query(r.Context(), a, name, hostenv.ToolsCommand)
	}()
	go func() {
		defer wg.Done()
		// 网络这一半查不出来（主机上没有 curl、全都超时）不算失败：解析出来就是“连不上”。
		net, _, _ = s.Kernel.Query(r.Context(), a, name, hostenv.NetCommand)
	}()
	wg.Wait()
	if terr != nil {
		fail(w, terr)
		return
	}
	n := hostenv.ParseNet(net)
	writeJSON(w, 200, map[string]any{"tools": hostenv.ParseTools(tools), "services": n.Services, "sites": n.Sites, "egress": n.Egress, "checkedAt": time.Now().UnixMilli()})
}
