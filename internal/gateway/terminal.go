package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/tsienwan/hostower/internal/kernel"
)

// openTerminal 提交一个“开终端”的请求。它是 L2：要在请求上批准之后才能连。
func (s *Server) openTerminal(w http.ResponseWriter, r *http.Request) {
	var in struct{ Host string }
	if !readJSON(w, r, &in) {
		return
	}
	req, err := s.Kernel.SubmitTerminal(r.Context(), actor(sessionOf(r)), in.Host)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

const terminalIdle = 30 * time.Minute

// terminalWS 把浏览器和终端接起来：二进制帧是键盘输入和终端输出，文本帧是控制消息（调整大小）。
func (s *Server) terminalWS(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	q := r.URL.Query()
	cols, _ := strconv.Atoi(q.Get("cols"))
	rows, _ := strconv.Atoi(q.Get("rows"))
	if cols < 20 || cols > 500 {
		cols = 100
	}
	if rows < 5 || rows > 200 {
		rows = 30
	}
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(64 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	term, err := s.Kernel.AttachTerminal(ctx, r.PathValue("id"), actor(sess), cols, rows)
	if err != nil {
		c.Close(websocket.StatusPolicyViolation, truncate(err.Error(), 120))
		return
	}
	defer term.Close()

	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	token := ""
	if ck, err := r.Cookie(cookieName); err == nil {
		token = ck.Value
	}
	// 终端 → 浏览器
	go func() {
		defer cancel()
		buf := make([]byte, 32<<10)
		for {
			n, err := term.Read(buf)
			if n > 0 {
				wctx, wcancel := context.WithTimeout(ctx, 15*time.Second)
				werr := c.Write(wctx, websocket.MessageBinary, buf[:n])
				wcancel()
				if werr != nil {
					return
				}
			}
			if err != nil {
				c.Close(websocket.StatusNormalClosure, "会话已结束")
				return
			}
		}
	}()
	// 看门狗：会话被吊销、塔台被暂停、长时间没有输入时断开。
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cur, err := s.Auth.Lookup(token)
				switch {
				case err != nil || cur.User.ID != sess.User.ID:
					c.Close(websocket.StatusPolicyViolation, "登录已失效")
				case s.Kernel.Paused():
					c.Close(websocket.StatusPolicyViolation, "执行已暂停")
				case time.Since(time.Unix(0, last.Load())) > terminalIdle:
					c.Close(websocket.StatusNormalClosure, "长时间没有操作")
				default:
					continue
				}
				cancel()
				return
			}
		}
	}()
	// 浏览器 → 终端
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		last.Store(time.Now().UnixNano())
		if typ == websocket.MessageBinary {
			if _, err := term.Write(data); err != nil {
				return
			}
			continue
		}
		var msg struct{ Cols, Rows int }
		if json.Unmarshal(data, &msg) == nil && msg.Cols >= 20 && msg.Cols <= 500 && msg.Rows >= 5 && msg.Rows <= 200 {
			term.Resize(msg.Cols, msg.Rows)
		}
	}
}

// terminalRecording 下载一次终端会话的录像（asciicast v2，可以用 asciinema 回放）。
func (s *Server) terminalRecording(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.Kernel.RecordingPath(id)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-asciicast")
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.cast"`)
	http.ServeFile(w, r, p)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

var _ = kernel.ErrNotFound
