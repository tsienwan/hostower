// Package auth 负责身份：登录（Passkey、密码 + 可选的动态码、单点登录由网关校验后调 LoginExternal）、
// 服务端会话、身份验证（管理账号安全设置和批准高危操作之前）。
// 首次启动时自动创建所有者并生成初始密码（见 factors.go 的 EnsureOwner）。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	ceremonyTTL = 5 * time.Minute
)

var (
	ErrBadToken      = errors.New("令牌不正确")
	ErrCeremony      = errors.New("验证流程已过期，请重试")
	ErrNotDev        = errors.New("只在开发模式下可用")
	ErrUnauthorized  = errors.New("未登录")
	ErrVerification  = errors.New("Passkey 验证失败")
	ErrSetupRequired = errors.New("尚未初始化")
)

// User 是一个塔台用户。
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	creds []webauthn.Credential
}

func (u *User) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u *User) WebAuthnName() string                       { return u.Name }
func (u *User) WebAuthnDisplayName() string                { return u.Name }
func (u *User) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// Session 是一个已登录的会话。
type Session struct {
	User      *User
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
	Dev       bool   // 开发模式下用令牌登录，没有经过 Passkey
	Method    string // 登录方式：passkey、password、password+totp、oidc、dev
}

// 登录方式。
const (
	MethodPasskey = "passkey"
	MethodOIDC    = "oidc"
	MethodDev     = "dev"
)

type ceremony struct {
	kind    string // login | reauth | add
	data    webauthn.SessionData
	user    *User
	expires time.Time
}

// Service 是认证服务。
type Service struct {
	db  *sql.DB
	wa  *webauthn.WebAuthn
	dev bool

	secrets Secrets
	dataDir string

	mu          sync.Mutex
	setupToken  string // 只在开发模式下有：启动令牌
	ceremonies  map[string]*ceremony
	verified    map[string]time.Time // 会话 → 最近一次验证身份的时间（管理账号安全设置用，5 分钟内有效）
	l2          map[string]time.Time // 会话 → 最近一次能批 L2 的验证（Passkey 或动态码，只能用一次）
	devTokens   map[string]bool      // 开发模式会话
	fails       map[string]*failure  // 账号 → 连续失败次数
	tickets     map[string]*ticketState
	pendingTOTP map[string]*pendingTOTP
	ttl         time.Duration // 会话时长
}

// SetSessionTTL 设置之后新建会话的时长。
func (s *Service) SetSessionTTL(d time.Duration) {
	s.mu.Lock()
	s.ttl = d
	s.mu.Unlock()
}

// SessionTTL 返回会话时长。
func (s *Service) SessionTTL() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ttl
}

// New 创建认证服务。publicURL 决定 WebAuthn 的 RP ID 和允许的来源。
func New(db *sql.DB, publicURL string, dev bool) (*Service, error) {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("HOSTOWER_PUBLIC_URL %q 无效", publicURL)
	}
	// 浏览器只允许在域名上用 Passkey（RP ID 不能是 IP 地址）。用 IP 访问时不启用 Passkey，密码和动态码照常可用。
	var wa *webauthn.WebAuthn
	if net.ParseIP(u.Hostname()) == nil {
		if wa, err = webauthn.New(&webauthn.Config{
			RPDisplayName: "Hostower",
			RPID:          u.Hostname(),
			RPOrigins:     []string{u.Scheme + "://" + u.Host},
		}); err != nil {
			return nil, err
		}
	}
	s := &Service{db: db, wa: wa, dev: dev, ceremonies: map[string]*ceremony{}, verified: map[string]time.Time{}, l2: map[string]time.Time{},
		devTokens: map[string]bool{}, fails: map[string]*failure{}, tickets: map[string]*ticketState{}, pendingTOTP: map[string]*pendingTOTP{}, ttl: 12 * time.Hour}
	if dev {
		s.setupToken = randomToken(18)
	}
	return s, nil
}

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// SetupToken 返回开发模式的启动令牌（只写进启动日志）。不是开发模式时为空。
func (s *Service) SetupToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setupToken
}

// NeedsSetup 报告是否还没有任何用户。
func (s *Service) NeedsSetup() (bool, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n == 0, err
}

func (s *Service) checkToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setupToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.setupToken)) == 1
}

func (s *Service) putCeremony(c *ceremony) string {
	id := randomToken(16)
	c.expires = time.Now().Add(ceremonyTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.ceremonies {
		if time.Now().After(v.expires) {
			delete(s.ceremonies, k)
		}
	}
	s.ceremonies[id] = c
	return id
}

func (s *Service) takeCeremony(id, kind string) (*ceremony, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.ceremonies[id]
	delete(s.ceremonies, id)
	if !ok || c.kind != kind || time.Now().After(c.expires) {
		return nil, ErrCeremony
	}
	return c, nil
}

var regOpts = []webauthn.RegistrationOption{
	webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
		ResidentKey:      protocol.ResidentKeyRequirementRequired,
		UserVerification: protocol.VerificationRequired,
	}),
}

// AddBegin 给已登录用户再注册一把 Passkey（建议至少两把）。
// ErrNoPasskey 表示当前访问地址是 IP，用不了 Passkey。
var ErrNoPasskey = errors.New("用 IP 地址访问时用不了 Passkey（浏览器只允许在域名上使用）：请用密码加动态码，或者改用域名访问")

// PasskeyAvailable 报告当前访问地址能不能用 Passkey。
func (s *Service) PasskeyAvailable() bool { return s.wa != nil }

func (s *Service) AddBegin(u *User) (string, any, error) {
	if s.wa == nil {
		return "", nil, ErrNoPasskey
	}
	full, err := s.loadUser(u.ID)
	if err != nil {
		return "", nil, err
	}
	var exclude []protocol.CredentialDescriptor
	for _, c := range full.creds {
		exclude = append(exclude, c.Descriptor())
	}
	opts, data, err := s.wa.BeginRegistration(full, append(regOpts, webauthn.WithExclusions(exclude))...)
	if err != nil {
		return "", nil, err
	}
	return s.putCeremony(&ceremony{kind: "add", data: *data, user: full}), opts, nil
}

// AddFinish 保存新注册的 Passkey。
func (s *Service) AddFinish(u *User, ceremonyID, name string, body io.Reader) error {
	c, err := s.takeCeremony(ceremonyID, "add")
	if err != nil || c.user.ID != u.ID {
		return ErrCeremony
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(body)
	if err != nil {
		return ErrVerification
	}
	cred, err := s.wa.CreateCredential(c.user, c.data, parsed)
	if err != nil {
		return ErrVerification
	}
	if name == "" {
		name = "Passkey"
	}
	credJSON, _ := json.Marshal(cred)
	_, err = s.db.Exec("INSERT INTO passkeys (id, user_id, name, credential, created_at) VALUES (?,?,?,?,?)",
		cred.ID, u.ID, name, string(credJSON), time.Now().UTC().Format(time.RFC3339))
	return err
}

// LoginBegin 开始 Passkey 登录（不需要先输入用户名）。
func (s *Service) LoginBegin() (string, any, error) {
	if s.wa == nil {
		return "", nil, ErrNoPasskey
	}
	opts, data, err := s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, err
	}
	return s.putCeremony(&ceremony{kind: "login", data: *data}), opts, nil
}

// LoginFinish 校验 Passkey 断言并创建会话。
func (s *Service) LoginFinish(ceremonyID string, body io.Reader) (string, *User, error) {
	c, err := s.takeCeremony(ceremonyID, "login")
	if err != nil {
		return "", nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(body)
	if err != nil {
		return "", nil, ErrVerification
	}
	var found *User
	cred, err := s.wa.ValidateDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		u, err := s.loadUser(string(userHandle))
		found = u
		return u, err
	}, c.data, parsed)
	if err != nil || found == nil {
		return "", nil, ErrVerification
	}
	s.updateCredential(cred)
	token, err := s.newSession(found.ID, MethodPasskey)
	return token, found, err
}

// ReauthBegin 开始一次用 Passkey 的身份验证（批准 L2、管理账号安全设置之前）。
func (s *Service) ReauthBegin(u *User) (string, any, error) {
	if s.wa == nil {
		return "", nil, ErrNoPasskey
	}
	full, err := s.loadUser(u.ID)
	if err != nil {
		return "", nil, err
	}
	if len(full.creds) == 0 {
		return "", nil, errors.New("这个账号没有 Passkey")
	}
	opts, data, err := s.wa.BeginLogin(full, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, err
	}
	return s.putCeremony(&ceremony{kind: "reauth", data: *data, user: full}), opts, nil
}

// ReauthFinish 校验二次验证，并在短时间内把会话标记为“刚验证过”。
func (s *Service) ReauthFinish(sess *Session, ceremonyID string, body io.Reader) error {
	c, err := s.takeCeremony(ceremonyID, "reauth")
	if err != nil || c.user.ID != sess.User.ID {
		return ErrCeremony
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(body)
	if err != nil {
		return ErrVerification
	}
	cred, err := s.wa.ValidateLogin(c.user, c.data, parsed)
	if err != nil {
		return ErrVerification
	}
	s.updateCredential(cred)
	s.markVerified(sess, true)
	return nil
}

// ConsumeReauth 报告会话是否刚用 Passkey 或动态码验证过（批准 L2 用）；一次验证只能批一次。
// 开发模式的令牌会话直接放行（界面上有明显标记）。
func (s *Service) ConsumeReauth(sess *Session) bool {
	if sess.Dev {
		return s.dev
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.l2[sess.TokenHash]
	delete(s.l2, sess.TokenHash)
	return ok && time.Since(at) < l2Window
}

func (s *Service) updateCredential(cred *webauthn.Credential) {
	if cred == nil {
		return
	}
	b, _ := json.Marshal(cred)
	s.db.Exec("UPDATE passkeys SET credential = ? WHERE id = ?", string(b), cred.ID)
}

func (s *Service) loadUser(id string) (*User, error) {
	u := &User{}
	err := s.db.QueryRow("SELECT id, name, role FROM users WHERE id = ?", id).Scan(&u.ID, &u.Name, &u.Role)
	if err != nil {
		return nil, ErrUnauthorized
	}
	rows, err := s.db.Query("SELECT credential FROM passkeys WHERE user_id = ?", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c webauthn.Credential
		if json.Unmarshal([]byte(raw), &c) == nil {
			u.creds = append(u.creds, c)
		}
	}
	return u, rows.Err()
}

// DevLogin 用启动日志里的令牌登录。只在 --dev 且只监听本机时可用。
func (s *Service) DevLogin(token string) (string, *User, error) {
	if !s.dev {
		return "", nil, ErrNotDev
	}
	if !s.checkToken(token) {
		return "", nil, ErrBadToken
	}
	u := &User{}
	err := s.db.QueryRow("SELECT id, name, role FROM users ORDER BY created_at LIMIT 1").Scan(&u.ID, &u.Name, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		u = &User{ID: "u_dev", Name: "dev-owner", Role: "owner"}
		_, err = s.db.Exec("INSERT INTO users (id, name, role, created_at) VALUES (?,?,?,?)", u.ID, u.Name, u.Role, time.Now().UTC().Format(time.RFC3339))
	}
	if err != nil {
		return "", nil, err
	}
	t, err := s.newSession(u.ID, MethodDev)
	if err == nil {
		s.mu.Lock()
		s.devTokens[hashToken(t)] = true
		s.mu.Unlock()
	}
	return t, u, err
}

// LoginExternal 给所有者开一个会话。只能在外部身份（OIDC）已经由网关校验过、并且在允许列表里之后调用。
// 这个会话和其他方式登录的一样；L2 的当场验证仍然要 Passkey 或动态码。
func (s *Service) LoginExternal() (string, *User, error) {
	u := &User{}
	err := s.db.QueryRow("SELECT id, name, role FROM users WHERE role = 'owner' ORDER BY created_at LIMIT 1").Scan(&u.ID, &u.Name, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrSetupRequired
	}
	if err != nil {
		return "", nil, err
	}
	t, err := s.newSession(u.ID, MethodOIDC)
	return t, u, err
}

func (s *Service) newSession(userID, method string) (string, error) {
	token := randomToken(32)
	now := time.Now().UTC()
	_, err := s.db.Exec("INSERT INTO sessions (token_hash, user_id, created_at, expires_at, method) VALUES (?,?,?,?,?)",
		hashToken(token), userID, now.Format(time.RFC3339), now.Add(s.SessionTTL()).Format(time.RFC3339), method)
	s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", now.Format(time.RFC3339))
	return token, err
}

// Lookup 按会话令牌找会话。
func (s *Service) Lookup(token string) (*Session, error) {
	if token == "" {
		return nil, ErrUnauthorized
	}
	h := hashToken(token)
	var u User
	var exp, created, method string
	err := s.db.QueryRow(`SELECT u.id, u.name, u.role, s.expires_at, s.created_at, s.method FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ?`, h).
		Scan(&u.ID, &u.Name, &u.Role, &exp, &created, &method)
	if err != nil {
		return nil, ErrUnauthorized
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil || time.Now().After(t) {
		return nil, ErrUnauthorized
	}
	s.mu.Lock()
	dev := s.devTokens[h]
	s.mu.Unlock()
	c, _ := time.Parse(time.RFC3339, created)
	return &Session{User: &u, TokenHash: h, ExpiresAt: t, CreatedAt: c, Dev: dev, Method: method}, nil
}

// User 按编号找用户（不带 Passkey）。聊天入口用它确认绑定的账号还在、角色没变。
func (s *Service) User(id string) (*User, error) {
	var u User
	if err := s.db.QueryRow("SELECT id, name, role FROM users WHERE id = ?", id).Scan(&u.ID, &u.Name, &u.Role); err != nil {
		return nil, ErrUnauthorized
	}
	return &u, nil
}

// Logout 结束一个会话。
func (s *Service) Logout(sess *Session) {
	s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", sess.TokenHash)
}

// RevokeAll 吊销所有会话（紧急开关）。
func (s *Service) RevokeAll() error {
	_, err := s.db.Exec("DELETE FROM sessions")
	s.mu.Lock()
	s.verified = map[string]time.Time{}
	s.l2 = map[string]time.Time{}
	s.devTokens = map[string]bool{}
	s.mu.Unlock()
	return err
}

// PasskeyInfo 是一把 Passkey 的元数据。
type PasskeyInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// Passkeys 列出用户的 Passkey。
func (s *Service) Passkeys(userID string) ([]PasskeyInfo, error) {
	rows, err := s.db.Query("SELECT rowid, name, created_at FROM passkeys WHERE user_id = ? ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PasskeyInfo{}
	for rows.Next() {
		var p PasskeyInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
