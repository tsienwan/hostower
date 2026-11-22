// Package gateway 是唯一的网络入口：认证、来源校验、限速，然后把操作交给内核。
// 网关自己不碰主机，也没有绕过内核的接口。
package gateway

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/app"
	"github.com/tsienwan/hostower/internal/apps"
	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/metrics"
	"github.com/tsienwan/hostower/internal/notify"
	"github.com/tsienwan/hostower/internal/oidc"
	"github.com/tsienwan/hostower/internal/secrets"
	"github.com/tsienwan/hostower/internal/selfmon"
	"github.com/tsienwan/hostower/internal/snippet"
	"github.com/tsienwan/hostower/internal/speedtest"
	"github.com/tsienwan/hostower/internal/update"
)

// Options 是网关的依赖和配置。
type Options struct {
	Version string
	// Build 是正在运行的可执行文件的构建封签校验结果（“官方构建”标识）。
	Build update.BuildInfo
	// SourceRepo 是源代码仓库（owner/name）。AGPL 要求通过网络提供服务时告诉用户去哪里拿源代码。
	SourceRepo string
	// Restart 让 Hostower 优雅停下，再用磁盘上同一个位置的可执行文件原地重新启动（重新读取面板设置）。
	// 为 nil 表示这个平台不支持。
	Restart   func() error
	Dev       bool
	PublicURL string
	// TrustProxy 表示前面有一层终结 TLS 的反向代理，X-Forwarded-For 可信。
	TrustProxy bool
	Kernel     *kernel.Kernel
	Auth       *auth.Service
	Sampler    *metrics.Sampler
	Secrets    *secrets.Store
	Snippets   func() map[string]*snippet.Snippet
	Reload     func() error
	// 页面管理的主机。配置文件里的主机在这里是只读的。
	SaveHost   func(h inventory.Host, create bool) error
	DeleteHost func(name string) error
	Executors  func() []string
	// 页面管理的 Snippet。
	SaveSnippet   func(spec snippet.Snippet, create bool) (*snippet.Snippet, error)
	DeleteSnippet func(name string) error
	// 面板设置（监听地址、TLS、会话时长）。
	Panel     func() (app.PanelView, error)
	SavePanel func(map[string]string) (app.Config, error)
	Started   time.Time
	// Self 返回 Hostower 所在这台机器的监测数据。
	Self func() selfmon.Status
	// Update 是自我更新。
	Update *update.Updater
	// Notify 是通知中心。
	Notify *notify.Hub
	// Speedtest 存着解析好的测速结果。
	Speedtest *speedtest.Service
	// OIDC 是用身份提供方登录。
	OIDC *oidc.Service
	// AuditArchiveDir 是审计归档文件放在哪里（数据目录下的 audit-archive）。
	AuditArchiveDir string
	// DB 只给聊天入口保存它的设置和绑定用。
	DB  *sql.DB
	Web fs.FS // 构建好的前端；可以为 nil
	Log *slog.Logger
}

// Server 是 HTTP 网关。
type Server struct {
	Options
	origin  string
	secure  bool
	limiter *limiter
	bot     *chatBot
	// 应用市场：内置目录，以及查 npm 最新版本（检测命令行工具的更新）。
	apps        *apps.Catalog
	appRegistry *apps.Registry
	appAuto     *appAuto
}

const cookieName = "hostower_session"

// New 创建网关。
func New(o Options) (*Server, error) {
	u, err := url.Parse(o.PublicURL)
	if err != nil {
		return nil, err
	}
	s := &Server{Options: o, origin: u.Scheme + "://" + u.Host, secure: u.Scheme == "https", limiter: newLimiter(20, time.Minute)}
	s.bot = newChatBot(s)
	if s.apps, err = apps.Builtin(); err != nil {
		return nil, fmt.Errorf("内置应用目录：%w", err)
	}
	s.appRegistry = apps.NewRegistry()
	s.loadAppAuto()
	return s, nil
}

type ctxKey struct{}

func sessionOf(r *http.Request) *auth.Session {
	s, _ := r.Context().Value(ctxKey{}).(*auth.Session)
	return s
}

// actor 把已认证的会话变成内核认识的发起人。等级上限来自角色，调用方无法自报。
func actor(s *auth.Session) kernel.Actor {
	a := kernel.Actor{ID: s.User.ID, Name: s.User.Name, Kind: kernel.Human, Entry: "web"}
	switch s.User.Role {
	case "owner", "ops":
		a.MaxLevel, a.CanApprove = risk.L2, true
	case "operator":
		a.MaxLevel = risk.L0
	default:
		a.MaxLevel = risk.L0
	}
	return a
}

// Handler 返回完整的 HTTP 处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	pub := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.api(h, false)) }
	priv := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.api(h, true)) }

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	pub("GET /api/v1/bootstrap", s.bootstrap)
	pub("POST /api/v1/auth/login/begin", s.limited(s.loginBegin))
	pub("POST /api/v1/auth/login/finish", s.limited(s.loginFinish))
	pub("POST /api/v1/auth/dev", s.limited(s.devLogin))
	pub("POST /api/v1/auth/password/login", s.limited(s.passwordLogin))
	pub("POST /api/v1/auth/password/totp", s.limited(s.passwordTOTP))
	pub("GET /api/v1/auth/oidc/start", s.limited(s.oidcStart))
	pub("GET /api/v1/auth/oidc/callback", s.limited(s.oidcCallback))
	priv("GET /api/v1/auth/oidc", s.oidcSettings)
	priv("PUT /api/v1/auth/oidc", s.oidcSettings)
	priv("POST /api/v1/auth/logout", s.logout)
	priv("POST /api/v1/auth/reauth/begin", s.reauthBegin)
	priv("POST /api/v1/auth/reauth/finish", s.reauthFinish)
	priv("GET /api/v1/auth/passkeys", s.passkeys)
	priv("POST /api/v1/auth/passkeys/begin", s.passkeyBegin)
	priv("POST /api/v1/auth/passkeys/finish", s.passkeyFinish)
	priv("DELETE /api/v1/auth/passkeys/{id}", s.passkeyDelete)
	priv("GET /api/v1/auth/factors", s.authFactors)
	priv("PUT /api/v1/auth/methods", s.authMethods)
	priv("POST /api/v1/auth/password", s.setPassword)
	priv("POST /api/v1/auth/username", s.setUsername)
	priv("POST /api/v1/auth/verify/totp", s.limited(s.verifyTOTP))
	priv("POST /api/v1/auth/verify/password", s.limited(s.verifyPassword))
	priv("POST /api/v1/auth/totp/begin", s.totpBegin)
	priv("POST /api/v1/auth/totp/finish", s.limited(s.totpFinish))
	priv("DELETE /api/v1/auth/totp", s.totpRemove)

	priv("GET /api/v1/hosts", s.hosts)
	priv("GET /api/v1/hosts/{name}/metrics", s.hostMetrics)
	priv("GET /api/v1/hosts/{name}/traffic", s.hostTraffic)
	priv("PUT /api/v1/hosts/{name}/traffic", s.setHostTraffic)
	priv("DELETE /api/v1/hosts/{name}/traffic", s.deleteHostTraffic)
	priv("GET /api/v1/traffic", s.trafficRanking)
	priv("POST /api/v1/hosts", s.createHost)
	priv("PUT /api/v1/hosts/{name}", s.updateHost)
	priv("DELETE /api/v1/hosts/{name}", s.deleteHost)
	priv("POST /api/v1/hosts/{name}/agent", s.hostAgent)
	priv("GET /api/v1/hosts/{name}/docker", s.hostDocker)
	priv("GET /api/v1/hosts/{name}/env", s.hostEnv)
	priv("POST /api/v1/hosts/{name}/docker", s.hostDockerInstall)
	priv("POST /api/v1/hosts/{name}/reconnect", s.hostReconnect)
	priv("GET /api/v1/hosts/{name}/network", s.hostNetwork)
	priv("POST /api/v1/hosts/{name}/firewall", s.hostFirewall)
	priv("POST /api/v1/hosts/{name}/sites", s.hostSites)
	priv("POST /api/v1/hosts/{name}/certs", s.hostCerts)
	priv("POST /api/v1/hosts/{name}/fail2ban", s.hostFail2ban)
	priv("GET /api/v1/hosts/{name}/netlog", s.hostNetLog)
	priv("GET /api/v1/select", s.selectHosts)
	priv("POST /api/v1/classify", s.classify)
	priv("GET /api/v1/requests", s.listRequests)
	priv("POST /api/v1/requests", s.submit)
	priv("GET /api/v1/requests/{id}", s.getRequest)
	priv("POST /api/v1/requests/{id}/approve", s.approve)
	priv("POST /api/v1/requests/{id}/deny", s.deny)
	priv("POST /api/v1/requests/{id}/cancel", s.cancel)
	priv("GET /api/v1/snippets", s.snippets)
	priv("POST /api/v1/snippets/{name}/render", s.renderSnippet)
	priv("POST /api/v1/snippets", s.createSnippet)
	priv("PUT /api/v1/snippets/{name}", s.updateSnippet)
	priv("DELETE /api/v1/snippets/{name}", s.deleteSnippet)
	priv("GET /api/v1/files", s.listDir)
	priv("GET /api/v1/files/content", s.readFile)
	priv("PUT /api/v1/files/content", s.writeFile)
	priv("GET /api/v1/files/download", s.fileDownload)
	priv("GET /api/v1/files/search", s.fileSearch)
	priv("POST /api/v1/files/upload", s.fileUpload)
	priv("POST /api/v1/files/distribute", s.fileDistribute)
	priv("POST /api/v1/agents/task", s.agentTask)
	priv("POST /api/v1/agents/summary", s.agentSummary)
	priv("GET /api/v1/files/collect", s.fileCollect)
	priv("GET /api/v1/files/collect/download", s.fileCollectDownload)
	priv("GET /api/v1/audit", s.auditList)
	priv("GET /api/v1/audit/verify", s.auditVerify)
	priv("GET /api/v1/speedtest", s.speedtestList)
	priv("POST /api/v1/speedtest", s.speedtestRun)
	priv("POST /api/v1/speedtest/check", s.speedtestCheck)
	priv("GET /api/v1/speedtest/retention", s.speedtestRetention)
	priv("PUT /api/v1/speedtest/retention", s.speedtestRetention)
	priv("POST /api/v1/speedtest/delete", s.speedtestDelete)
	priv("GET /api/v1/speedtest/report", s.speedtestReport)
	priv("GET /api/v1/speedtest/servers", s.speedtestServers)
	priv("PUT /api/v1/speedtest/ookla", s.speedtestOokla)
	priv("GET /api/v1/speedtest/geo", s.speedtestGeo)
	priv("PUT /api/v1/speedtest/geo", s.speedtestGeoSetting)
	priv("GET /api/v1/speedtest/routes", s.speedtestTargets)
	priv("PUT /api/v1/speedtest/routes", s.speedtestTargets)
	priv("GET /api/v1/system/retention", s.retention)
	priv("PUT /api/v1/system/retention", s.retention)
	priv("GET /api/v1/audit/archive", s.auditArchiveView)
	priv("PUT /api/v1/audit/archive", s.auditArchiveSet)
	priv("POST /api/v1/audit/archive/run", s.auditArchiveRun)
	priv("GET /api/v1/audit/routine", s.auditRoutine)
	priv("PUT /api/v1/audit/routine", s.auditRoutine)
	priv("GET /api/v1/system", s.system)
	priv("POST /api/v1/system/pause", s.pause)
	priv("POST /api/v1/system/resume", s.resume)
	priv("POST /api/v1/system/revoke-sessions", s.revokeSessions)
	priv("POST /api/v1/system/reload", s.reload)
	priv("GET /api/v1/secrets", s.listSecrets)
	priv("PUT /api/v1/secrets/ssh", s.putSSHSecret)
	priv("DELETE /api/v1/secrets", s.deleteSecret)
	priv("GET /api/v1/system/self", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, s.Self()) })
	priv("GET /api/v1/system/update", s.updateStatus)
	priv("PUT /api/v1/system/update", s.updateSet)
	priv("POST /api/v1/system/update/check", s.updateCheck)
	priv("POST /api/v1/system/update/apply", s.updateApply)
	priv("GET /api/v1/notify", s.notifyList)
	priv("POST /api/v1/notify", s.notifySave)
	priv("PUT /api/v1/notify/{id}", s.notifySave)
	priv("DELETE /api/v1/notify/{id}", s.notifyDelete)
	priv("POST /api/v1/notify/{id}/test", s.notifyTest)
	priv("GET /api/v1/chatbot", s.chatGet)
	priv("PUT /api/v1/chatbot", s.chatPut)
	priv("POST /api/v1/chatbot/pair", s.chatPair)
	priv("DELETE /api/v1/chatbot/binding", s.chatUnbind)
	priv("GET /api/v1/system/panel", s.panel)
	priv("PUT /api/v1/system/panel", s.savePanel)
	priv("POST /api/v1/system/restart", s.restart)
	priv("GET /api/v1/containers", s.containers)
	priv("GET /api/v1/containers/updates", s.containerUpdates)
	priv("GET /api/v1/containers/logs", s.containerLogs)
	priv("GET /api/v1/containers/inspect", s.containerInspect)
	priv("POST /api/v1/containers/action", s.containerAct)
	priv("GET /api/v1/apps", s.appCatalog)
	priv("GET /api/v1/apps/installed", s.appInstalled)
	priv("GET /api/v1/apps/reveal", s.appReveal)
	priv("POST /api/v1/apps/install", s.appInstall)
	priv("POST /api/v1/apps/action", s.appAction)
	priv("POST /api/v1/apps/configure", s.appConfigure)
	priv("GET /api/v1/apps/autoupdate", s.appAutoStatus)
	priv("PUT /api/v1/apps/autoupdate", s.appAutoSet)
	priv("POST /api/v1/apps/autoupdate/check", s.appAutoCheck)
	priv("GET /api/v1/appsecrets", s.listAppSecrets)
	priv("PUT /api/v1/appsecrets", s.putAppSecret)
	priv("DELETE /api/v1/appsecrets", s.deleteAppSecret)
	priv("GET /api/v1/registries", s.listRegistries)
	priv("PUT /api/v1/registries", s.putRegistry)
	priv("DELETE /api/v1/registries", s.deleteRegistry)
	mux.Handle("POST /api/v1/uploads", s.apiN(s.upload, true, maxUpload))
	priv("POST /api/v1/terminal", s.openTerminal)
	priv("GET /api/v1/terminal/{id}/ws", s.terminalWS)
	priv("GET /api/v1/terminal/{id}/recording", s.terminalRecording)
	priv("GET /api/v1/stream", s.stream)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "接口不存在")
	})
	mux.Handle("/", s.static())
	return s.headers(mux)
}

// headers 给所有响应加安全头。内容安全策略禁止内联脚本和一切第三方来源。
func (s *Server) headers(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; " +
		"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if s.secure {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// api 包装一个接口：限制请求体、校验来源、（需要时）校验会话。
func (s *Server) api(h http.HandlerFunc, needAuth bool) http.Handler {
	return s.apiN(h, needAuth, 2<<20)
}

// apiN 和 api 一样，只是请求体上限可调（镜像包上传用）。
func (s *Server) apiN(h http.HandlerFunc, needAuth bool, limit int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, limit)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			// 写请求必须来自本站页面：校验 Origin，并要求一个表单发不出来的自定义头。
			if r.Header.Get("Origin") != s.origin || r.Header.Get("X-Hostower") != "1" {
				writeErr(w, http.StatusForbidden, "来源校验失败")
				return
			}
		}
		if needAuth {
			var token string
			if c, err := r.Cookie(cookieName); err == nil {
				token = c.Value
			}
			sess, err := s.Auth.Lookup(token)
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "未登录")
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess))
		}
		h(w, r)
	})
}

func (s *Server) limited(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.allow(s.clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeErr(w, http.StatusTooManyRequests, "尝试过于频繁，请稍后再试")
			return
		}
		h(w, r)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) setSession(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure,
		SameSite: http.SameSiteStrictMode, MaxAge: int(s.Auth.SessionTTL().Seconds()),
	})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

// auditErr 和 audit 一样，但把写入失败交给调用方：要求“写不进审计就中止”的操作用它。
func (s *Server) auditErr(r *http.Request, e audit.Entry) error {
	if e.Entry == "" {
		e.Entry = "web"
	}
	if sess := sessionOf(r); sess != nil && e.Actor == "" {
		e.Actor = sess.User.Name
	}
	if e.Detail == nil {
		e.Detail = map[string]any{}
	}
	e.Detail["ip"] = s.clientIP(r)
	_, err := s.Kernel.Audit().Append(r.Context(), e)
	if err != nil {
		s.Log.Error("写审计失败", "err", err)
	}
	return err
}

func (s *Server) audit(r *http.Request, e audit.Entry) {
	if e.Entry == "" {
		e.Entry = "web"
	}
	if sess := sessionOf(r); sess != nil && e.Actor == "" {
		e.Actor = sess.User.Name
	}
	if e.Detail == nil {
		e.Detail = map[string]any{}
	}
	e.Detail["ip"] = s.clientIP(r)
	if _, err := s.Kernel.Audit().Append(r.Context(), e); err != nil {
		s.Log.Error("写审计失败", "err", err)
	}
}

// static 提供内嵌的前端；未知路径回退到 index.html（单页应用）。
func (s *Server) static() http.Handler {
	if s.Web == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("前端还没有构建：在 web/ 目录下运行 npm install && npm run build，然后重新编译。\n"))
		})
	}
	files := http.FileServerFS(s.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.Web, p); err != nil {
			p = "index.html"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + p
		if p == "index.html" {
			// FileServer 会把 /index.html 重定向到 /，这里直接读出来。
			data, err := fs.ReadFile(s.Web, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		files.ServeHTTP(w, r2)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// fail 把内核和认证的错误映射成状态码。
func fail(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, kernel.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, kernel.ErrForbidden), errors.Is(err, kernel.ErrNotAllowed), errors.Is(err, kernel.ErrHumanOnly),
		errors.Is(err, kernel.ErrL2WebOnly), errors.Is(err, kernel.ErrSelfApproval), errors.Is(err, auth.ErrBadToken), errors.Is(err, auth.ErrNotDev), errors.Is(err, auth.ErrMethodDisabled):
		status = http.StatusForbidden
	case errors.Is(err, kernel.ErrPaused):
		status = http.StatusServiceUnavailable
	case errors.Is(err, kernel.ErrNotPending), errors.Is(err, kernel.ErrExpired), errors.Is(err, kernel.ErrDigest):
		status = http.StatusConflict
	case errors.Is(err, kernel.ErrReauth):
		status = http.StatusPreconditionRequired
	case errors.Is(err, auth.ErrUnauthorized), errors.Is(err, auth.ErrVerification), errors.Is(err, auth.ErrCeremony),
		errors.Is(err, auth.ErrBadPassword), errors.Is(err, auth.ErrBadCode):
		status = http.StatusUnauthorized
	}
	writeErr(w, status, err.Error())
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "请求格式错误")
		return false
	}
	return true
}

// limiter 是按来源的固定窗口限速。
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*hit
}

type hit struct {
	n     int
	reset time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, hits: map[string]*hit{}}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 {
		for k, h := range l.hits {
			if now.After(h.reset) {
				delete(l.hits, k)
			}
		}
	}
	h := l.hits[key]
	if h == nil || now.After(h.reset) {
		l.hits[key] = &hit{n: 1, reset: now.Add(l.window)}
		return true
	}
	h.n++
	return h.n <= l.max
}

func contextWithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
