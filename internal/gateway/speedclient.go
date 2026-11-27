package gateway

import (
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"strconv"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/speedtest"
)

// “这台设备”测速：把正在看面板的这台设备当成一个节点。
//
// 延迟、下载、上传是浏览器对着下面三个接口自己量的（ping、down、up），只经过面板，不触达任何主机；
// 回程路由照常走内核：对网关从连接上读到的来源地址生成一条固定的追踪命令，判级、批准、执行、审计都和别的测速一样。
// 来源地址不从请求体里读。
//
// 要测“这台设备 ↔ 另一台主机”时是直连测速：命令在那台主机上临时开一个测速服务，浏览器另开一页直接连它，
// 数字由那个服务转述到命令的输出里（见 speedtest.DirectCommand）。面板只负责生成命令和一次性的口令。

// clientSlots 限制同时在跑的下载、上传：测速很耗带宽，几个人同时测就够了。
var clientSlots = make(chan struct{}, 8)

func clientSlot(w http.ResponseWriter) bool {
	select {
	case clientSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "10")
		writeErr(w, http.StatusTooManyRequests, "同时在测速的连接太多了，请稍后再试")
		return false
	}
}

// speedClientInfo 告诉页面：面板看到的来源地址是什么、能不能追踪、前面有没有反向代理，以及可选的数据量。
func (s *Server) speedClientInfo(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	_, err := speedtest.ClientCommand(ip, false)
	writeJSON(w, 200, map[string]any{"ip": ip, "traceable": err == nil, "proxy": s.TrustProxy,
		"sizes": speedtest.ClientSizes, "seconds": speedtest.ClientSeconds, "pings": speedtest.ClientPings,
		"directPort": speedtest.DirectPort, "directWait": speedtest.DirectWait})
}

// speedClientPing 什么都不做：浏览器量的是这一个来回花了多久。
func (s *Server) speedClientPing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// speedClientDown 发一段压不动的随机数据给浏览器量下载速度。
func (s *Server) speedClientDown(w http.ResponseWriter, r *http.Request) {
	mb, _ := strconv.Atoi(r.URL.Query().Get("mb"))
	if mb < 1 || mb > speedtest.ClientSizes[len(speedtest.ClientSizes)-1] {
		writeErr(w, 400, "数据量不对")
		return
	}
	if !clientSlot(w) {
		return
	}
	defer func() { <-clientSlots }()
	var seed [32]byte
	if _, err := crand.Read(seed[:]); err != nil {
		writeErr(w, 500, "生成数据失败")
		return
	}
	n := int64(mb) * 1000 * 1000
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	w.Header().Set("X-Accel-Buffering", "no") // 前面是 nginx 时别先攒着
	io.CopyN(w, mrand.NewChaCha8(seed), n)    // 浏览器到时间会断开：写不出去就结束
}

// clientUpLimit 是一次上传请求最多多少字节（浏览器分块传，每块 1 MB 到 4 MB）。
const clientUpLimit = 4<<20 + 4096

// speedClientUp 收下浏览器传来的数据并丢掉，回报收到多少。
func (s *Server) speedClientUp(w http.ResponseWriter, r *http.Request) {
	if !clientSlot(w) {
		return
	}
	defer func() { <-clientSlots }()
	n, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "一次传得太多了")
		return
	}
	writeJSON(w, 200, map[string]any{"bytes": n})
}

// speedClientRun 发起一次“这台设备”测速里要在主机上做的那部分。
// 默认是回程路由追踪：从一台主机追踪到这台设备。Direct 为真时是直连测速：在那台主机上临时开测速服务，测完再追踪。
func (s *Server) speedClientRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Host string `json:"host"`
		// Install 为真时，主机上没有 traceroute 和 tracepath 就先装上 traceroute。
		Install bool `json:"install"`
		Direct  bool `json:"direct"`
		Port    int  `json:"port"`
		SizeMB  int  `json:"sizeMb"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Host == "" {
		writeErr(w, 400, "没有选中任何主机")
		return
	}
	var cmd, intent string
	out := map[string]any{}
	if in.Direct {
		var raw [16]byte
		if _, err := crand.Read(raw[:]); err != nil {
			writeErr(w, 500, "生成数据失败")
			return
		}
		token := hex.EncodeToString(raw[:])
		c, err := speedtest.DirectCommand(in.Port, token, in.SizeMB, in.Install)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		cmd = c
		intent = "在 " + in.Host + " 上临时开一个测速服务（端口 " + strconv.Itoa(in.Port) + "），测这台设备和它之间的速度并追踪回程路由"
		out["direct"] = map[string]any{"port": in.Port, "token": token}
	} else {
		ip := s.clientIP(r)
		c, err := speedtest.ClientCommand(ip, in.Install)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		cmd = c
		intent = "追踪从 " + in.Host + " 到这台设备（" + ip + "）的回程路由"
		if in.Install {
			intent = "安装缺少的 traceroute 并追踪从 " + in.Host + " 到这台设备（" + ip + "）的回程路由"
		}
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), kernel.CommandSpec{
		Hosts: []string{in.Host}, Commands: map[string]string{in.Host: cmd}, Snippet: speedtest.Tag, Intent: intent})
	if err != nil {
		fail(w, err)
		return
	}
	out["request"] = req
	writeJSON(w, 200, out)
}

// speedClientResult 记下浏览器量到的数字。只认本人发起的“这台设备”测速请求；地址取自那条命令，不取自请求体。
func (s *Server) speedClientResult(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Request string           `json:"request"`
		Measure speedtest.Client `json:"measure"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	req, err := s.Kernel.Get(in.Request)
	if err != nil || req.Snippet != speedtest.Tag || req.Actor.ID != actor(sessionOf(r)).ID || len(req.Targets) != 1 {
		writeErr(w, 404, "没有这条测速请求")
		return
	}
	spec, ok := speedtest.SpecOf(req.Targets[0].Command)
	if !ok || spec.Method != speedtest.MethodClient {
		writeErr(w, 404, "没有这条测速请求")
		return
	}
	if err := s.Speedtest.SaveClient(r.Context(), req.ID, spec.Routes[0], s.TrustProxy, in.Measure); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// speedClientForward 把贴进来的去程路由（在设备那边跑的 tracert / traceroute 的输出）记到一条“这台设备”测速上；内容为空是清掉。
func (s *Server) speedClientForward(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Request string `json:"request"`
		Host    string `json:"host"`
		Text    string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	res, err := s.Speedtest.Get(r.Context(), speedtest.Key{RequestID: in.Request, Host: in.Host})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && res.Method != speedtest.MethodClient && res.Method != speedtest.MethodDirect) {
		writeErr(w, 404, "没有这条测速记录")
		return
	}
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	route, err := s.Speedtest.SaveForward(r.Context(), in.Request, in.Text)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"route": route})
}
