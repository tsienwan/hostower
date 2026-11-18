package gateway

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/kernel/audit"
)

// 密码登录、动态码、登录方式设置和身份验证。
//
// 改登录方式、密码、动态码、Passkey 之前都要先验证身份（requireVerified）；验证 5 分钟内有效。
// 每次成功和失败都写审计；改动类的操作先写审计，写不进去就不改。

// requireVerified 要求这个会话最近验证过身份。没有时回 403 和固定的提示，前端据此弹出验证框再重试。
func (s *Server) requireVerified(w http.ResponseWriter, r *http.Request) bool {
	if s.Auth.Verified(sessionOf(r)) {
		return true
	}
	writeErr(w, http.StatusForbidden, auth.ErrNeedVerify.Error())
	return false
}

func (s *Server) oidcOn() bool {
	if s.OIDC == nil {
		return false
	}
	on, _ := s.OIDC.Enabled()
	return on
}

// mustAudit 先写审计；写不进去就回 500，调用方不再继续。
func (s *Server) mustAudit(w http.ResponseWriter, r *http.Request, typ, msg string, detail map[string]any) bool {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["ip"] = s.clientIP(r)
	if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: typ, Actor: sessionOf(r).User.Name, Entry: "web", Message: msg, Detail: detail}); err != nil {
		writeErr(w, 500, "写审计失败，没有修改")
		return false
	}
	return true
}

// ---------- 密码登录 ----------

func (s *Server) passwordLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	token, u, ticket, err := s.Auth.PasswordLogin(in.Name, in.Password)
	if err != nil {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: "密码登录：" + err.Error(), Detail: map[string]any{"name": clipText(in.Name, 64)}})
		fail(w, err)
		return
	}
	if ticket != "" {
		writeJSON(w, 200, map[string]any{"totp": true, "ticket": ticket})
		return
	}
	s.audit(r, audit.Entry{Type: "auth.login", Actor: u.Name, Message: "密码登录"})
	s.setSession(w, token)
	writeJSON(w, 200, map[string]any{"user": u})
}

func (s *Server) passwordTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct{ Ticket, Code string }
	if !readJSON(w, r, &in) {
		return
	}
	token, u, err := s.Auth.PasswordTOTP(in.Ticket, in.Code)
	if err != nil {
		s.audit(r, audit.Entry{Type: "auth.login.failed", Message: "密码登录（动态码）：" + err.Error()})
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.login", Actor: u.Name, Message: "密码 + 动态码登录"})
	s.setSession(w, token)
	writeJSON(w, 200, map[string]any{"user": u})
}

// ---------- 账号安全概况 ----------

func (s *Server) authFactors(w http.ResponseWriter, r *http.Request) {
	sess := sessionOf(r)
	list, err := s.Auth.Passkeys(sess.User.ID)
	if err != nil {
		fail(w, err)
		return
	}
	verify, def := s.Auth.VerifyMethods(sess.User.ID, false)
	strong, _ := s.Auth.VerifyMethods(sess.User.ID, true)
	out := map[string]any{"passkeys": list, "factors": s.Auth.Factors(sess.User.ID), "methods": s.Auth.Methods(), "oidc": s.oidcOn(),
		"verify": verify, "strong": strong, "defaultVerify": def, "verified": s.Auth.Verified(sess)}
	if until := s.Auth.VerifiedUntil(sess); !until.IsZero() {
		out["verifiedUntil"] = until.UnixMilli()
	}
	writeJSON(w, 200, out)
}

// ---------- 身份验证 ----------

func (s *Server) verifyTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code string }
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.Auth.VerifyTOTP(sessionOf(r), in.Code); err != nil {
		s.audit(r, audit.Entry{Type: "auth.verify.failed", Message: "动态码验证：" + err.Error()})
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.verify", Message: "用动态码验证了身份"})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) verifyPassword(w http.ResponseWriter, r *http.Request) {
	var in struct{ Password string }
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.Auth.VerifyPassword(sessionOf(r), in.Password); err != nil {
		s.audit(r, audit.Entry{Type: "auth.verify.failed", Message: "密码验证：" + err.Error()})
		fail(w, err)
		return
	}
	s.audit(r, audit.Entry{Type: "auth.verify", Message: "用密码验证了身份"})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- 登录方式 ----------

func (s *Server) authMethods(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) || !s.requireVerified(w, r) {
		return
	}
	var in auth.Methods
	if !readJSON(w, r, &in) {
		return
	}
	before := s.Auth.Methods()
	if before == in {
		writeJSON(w, 200, map[string]any{"methods": before})
		return
	}
	if err := s.Auth.CheckUsable(in, s.oidcOn()); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.mustAudit(w, r, "auth.methods.updated", "修改登录方式", map[string]any{"before": before, "after": in}) {
		return
	}
	if err := s.Auth.SetMethods(in, s.oidcOn()); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"methods": s.Auth.Methods()})
}

// ---------- 密码 ----------

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	var in struct{ Password string }
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Password) < 12 {
		writeErr(w, 400, auth.ErrWeakPassword.Error())
		return
	}
	sess := sessionOf(r)
	had := s.Auth.Factors(sess.User.ID).Password
	msg := "修改密码"
	if !had {
		msg = "设置密码"
	}
	if !s.mustAudit(w, r, "auth.password.changed", msg, nil) {
		return
	}
	if err := s.Auth.SetPassword(sess.User.ID, in.Password); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// setUsername 修改自己的用户名：先验证身份，再写审计（带旧名字和新名字），写不进去就不改。
func (s *Server) setUsername(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	var in struct{ Name string }
	if !readJSON(w, r, &in) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Name))
	sess := sessionOf(r)
	if name == sess.User.Name {
		writeJSON(w, 200, map[string]any{"name": name})
		return
	}
	if !auth.ValidUsername(name) {
		writeErr(w, 400, auth.ErrBadUsername.Error())
		return
	}
	if !s.mustAudit(w, r, "auth.username.changed", "修改用户名", map[string]any{"before": sess.User.Name, "after": name}) {
		return
	}
	if err := s.Auth.Rename(sess.User.ID, name); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"name": name})
}

// ---------- 动态码 ----------

func (s *Server) totpBegin(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	secret, uri, err := s.Auth.TOTPBegin(sessionOf(r))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// 密钥只在绑定时给这一次，之后只存在凭据存储里，接口不再返回。
	writeJSON(w, 200, map[string]any{"secret": secret, "uri": uri})
}

func (s *Server) totpFinish(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	var in struct{ Code string }
	if !readJSON(w, r, &in) {
		return
	}
	if !s.mustAudit(w, r, "auth.totp.added", "绑定动态码", nil) {
		return
	}
	if err := s.Auth.TOTPFinish(sessionOf(r), in.Code); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) totpRemove(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	if !s.mustAudit(w, r, "auth.totp.removed", "解绑动态码", nil) {
		return
	}
	if err := s.Auth.TOTPRemove(sessionOf(r).User.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- Passkey ----------

func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	if !s.requireVerified(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "没有这把 Passkey")
		return
	}
	sess := sessionOf(r)
	name := ""
	for _, p := range mustList(s.Auth.Passkeys(sess.User.ID)) {
		if p.ID == id {
			name = p.Name
		}
	}
	if name == "" {
		writeErr(w, 404, "没有这把 Passkey")
		return
	}
	if !s.mustAudit(w, r, "auth.passkey.removed", "删除 Passkey："+name, nil) {
		return
	}
	if err := s.Auth.DeletePasskey(sess.User.ID, id, s.oidcOn()); err != nil {
		code := 400
		if !errors.Is(err, auth.ErrLockout) {
			code = 500
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func mustList(list []auth.PasskeyInfo, _ error) []auth.PasskeyInfo { return list }

// loginInfo 是登录页要的：能用哪些方式、默认先显示哪一种。
func (s *Server) loginInfo() map[string]any {
	list, def := s.Auth.LoginMethods(s.oidcOn())
	return map[string]any{"methods": list, "default": def}
}
