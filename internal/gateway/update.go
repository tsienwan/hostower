package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
)

// 自我更新的接口：看状态、开关自动更新、手动检查、手动更新。改动类的都只有所有者能做。

func (s *Server) updateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Update.Status())
}

func (s *Server) updateSet(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Auto bool `json:"auto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误")
		return
	}
	if err := s.Update.SetAuto(in.Auto); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	msg := "关闭自动更新"
	if in.Auto {
		msg = "打开自动更新"
	}
	s.audit(r, audit.Entry{Type: "system.update.auto", Message: msg, Detail: map[string]any{"auto": in.Auto}})
	s.updateStatus(w, r)
}

func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.Update.Check(ctx); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	s.updateStatus(w, r)
}

func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	s.audit(r, audit.Entry{Type: "system.update.requested", Message: "手动开始更新"})
	// 下载可能要一会儿，而且成功之后进程会被换掉：放到后台做，页面轮询状态。
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := s.Update.Apply(ctx); err != nil {
			s.Log.Warn("更新失败", "err", err)
		}
	}()
	writeJSON(w, 202, s.Update.Status())
}

// restart 重启 Hostower：只有所有者能做，要先当场验证身份，写进审计之后才执行。
// 正在执行的请求会被中断（重启后标成“中断”），所以有请求在跑时先告诉页面，确认后再带 force 提交。
func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) || !s.requireVerified(w, r) {
		return
	}
	if s.Restart == nil {
		writeErr(w, 400, "这个平台不支持从页面重启，请在命令行重启 Hostower")
		return
	}
	var in struct {
		Force bool `json:"force"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	running := 0
	for _, q := range s.Kernel.List(0) {
		if q.State == kernel.StateRunning {
			running++
		}
	}
	if running > 0 && !in.Force {
		writeJSON(w, 409, map[string]any{"error": "有请求正在执行", "running": running})
		return
	}
	if !s.mustAudit(w, r, "system.restart", "从页面重启 Hostower", map[string]any{"running": running}) {
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "started": s.Started.UnixMilli()})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	// 先把响应送出去，再停服务。
	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := s.Restart(); err != nil {
			s.Log.Error("重启失败", "err", err)
		}
	}()
}
