package gateway

import (
	"context"
	"net/http"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/notify"
)

// 通知渠道的管理。看列表任何登录的人都可以；新增、修改、删除、发测试只有所有者能做，并且写审计。
// 令牌、Webhook 地址、密码只写不读：列表里只有“有没有设置”。

func (s *Server) notifyList(w http.ResponseWriter, r *http.Request) {
	channels, status := s.Notify.Channels()
	writeJSON(w, 200, map[string]any{"channels": channels, "status": status, "events": notify.Events,
		"defaults": notify.DefaultEvents(), "available": s.Secrets.Available()})
}

func (s *Server) notifySave(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		notify.Channel
		Secret notify.Secret `json:"secret"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Channel.ID = r.PathValue("id")
	create := in.Channel.ID == ""
	c, err := s.Notify.Save(in.Channel, in.Secret)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	typ, msg := "notify.channel.updated", "修改通知渠道 "+c.Name
	if create {
		typ, msg = "notify.channel.created", "新建通知渠道 "+c.Name
	}
	s.audit(r, audit.Entry{Type: typ, Message: msg, Detail: map[string]any{"kind": c.Kind, "events": c.Events, "secretChanged": in.Secret != notify.Secret{}}})
	writeJSON(w, 200, c)
}

func (s *Server) notifyDelete(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	id := r.PathValue("id")
	name := id
	channels, _ := s.Notify.Channels()
	for _, c := range channels {
		if c.ID == id {
			name = c.Name
		}
	}
	if err := s.Notify.Delete(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "notify.channel.deleted", Message: "删除通知渠道 " + name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	err := s.Notify.Test(ctx, r.PathValue("id"))
	msg := "发送测试通知成功"
	if err != nil {
		msg = "发送测试通知失败：" + err.Error()
	}
	s.audit(r, audit.Entry{Type: "notify.test", Message: msg})
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
