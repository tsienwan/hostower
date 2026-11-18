package gateway

import (
	"errors"
	"net/http"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/oidc"
)

// OIDC 登录。开始和回调都是浏览器的整页跳转（GET），所以走 pub；state 放在只给回调路径用的 Cookie 里，
// 和地址里的 state 对上才继续。失败时跳回登录页，地址里只带一个简短的错误代码，不带任何身份信息。

const oidcCookie = "hostower_oidc"
const oidcPath = "/api/v1/auth/oidc/"

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	if need, _ := s.Auth.NeedsSetup(); need {
		http.Redirect(w, r, "/?login_error=setup", http.StatusFound)
		return
	}
	state, to, err := s.OIDC.Begin(r.Context())
	if err != nil {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: "OIDC 登录：" + err.Error()})
		code := "unavailable"
		if errors.Is(err, oidc.ErrDisabled) {
			code = "disabled"
		}
		http.Redirect(w, r, "/?login_error="+code, http.StatusFound)
		return
	}
	// SameSite=Lax：从身份提供方跳回来是跨站的整页跳转，Strict 的 Cookie 带不过来。
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: state, Path: oidcPath, HttpOnly: true, Secure: s.secure,
		SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, to, http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: "", Path: oidcPath, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: "OIDC 登录：身份提供方返回 " + clipText(e+" "+q.Get("error_description"), 200)})
		http.Redirect(w, r, "/?login_error=provider", http.StatusFound)
		return
	}
	var cookieState string
	if c, err := r.Cookie(oidcCookie); err == nil {
		cookieState = c.Value
	}
	id, err := s.OIDC.Finish(r.Context(), cookieState, q.Get("state"), q.Get("code"))
	if err != nil {
		msg := "OIDC 登录：" + err.Error()
		code := "failed"
		switch {
		case errors.Is(err, oidc.ErrNotAllowed):
			msg, code = "OIDC 登录：账号 "+id.Label()+" 不在允许列表里", "not_allowed"
			if id.Email != "" && !id.EmailVerified {
				msg += "（邮箱没有经过身份提供方验证）"
			}
		case errors.Is(err, oidc.ErrState):
			code = "expired"
		}
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: msg})
		http.Redirect(w, r, "/?login_error="+code, http.StatusFound)
		return
	}
	token, u, err := s.Auth.LoginExternal()
	if err != nil {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: "OIDC 登录：" + err.Error()})
		http.Redirect(w, r, "/?login_error=failed", http.StatusFound)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.login", Actor: u.Name, Message: "OIDC 登录：" + id.Label(),
		Detail: map[string]any{"method": "oidc", "sub": id.Subject, "email": id.Email}})
	s.setSession(w, token)
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcSettings 读或改 OIDC 设置。改只有所有者能做；先写审计，写不进去就不改。
func (s *Server) oidcSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		if !ownerOnly(w, r) || !s.requireVerified(w, r) {
			return
		}
		var in struct {
			oidc.Config
			Secret      string `json:"secret"`
			ClearSecret bool   `json:"clearSecret"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		before := s.OIDC.Config()
		// 关掉单点登录前确认还有别的登录方式能用。
		if before.Enabled && !in.Enabled {
			if err := s.Auth.CheckUsable(s.Auth.Methods(), false); err != nil {
				writeErr(w, 400, err.Error())
				return
			}
		}
		sess := sessionOf(r)
		if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "auth.oidc.updated", Actor: sess.User.Name, Entry: "web",
			Message: "修改 OIDC 登录设置",
			Detail: map[string]any{"ip": s.clientIP(r), "enabled": in.Enabled, "issuer": in.Issuer, "clientId": in.ClientID, "allowed": in.Allowed,
				"secretChanged": in.Secret != "" || in.ClearSecret, "wasEnabled": before.Enabled}}); err != nil {
			writeErr(w, 500, "写审计失败，没有修改")
			return
		}
		if _, err := s.OIDC.Save(r.Context(), in.Config, in.Secret, in.ClearSecret); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"config": s.OIDC.Config(), "redirectUrl": s.OIDC.RedirectURL(), "secretsAvailable": s.Secrets.Available()})
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
