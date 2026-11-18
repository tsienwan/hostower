// Package oidc 让所有者用公司或个人的身份提供方（OIDC：Google、Microsoft Entra、Okta、Authentik、Keycloak……）登录。
//
// 它只替代“登录”这一步：登录后得到的是所有者的会话，权限和 Passkey 登录一样；
// 高危操作（L2）的当场二次验证仍然要用 Passkey，OIDC 绕不过去。
// 只有列在“允许的账号”里的身份能登录：经过身份提供方验证的邮箱，或者 sub:<编号>。
//
// 流程是授权码 + PKCE，带 state 和 nonce；身份令牌（ID Token）的签名、签发方、受众、有效期、nonce 都会校验。
// 客户端密钥存在凭据存储里（oidc/client），只写不读。
package oidc

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/tsienwan/hostower/internal/secrets"
)

// Config 是 OIDC 的设置。客户端密钥不在这里。
type Config struct {
	Enabled  bool     `json:"enabled"`
	Name     string   `json:"name"`     // 登录按钮上显示的名字，例如“公司账号”
	Issuer   string   `json:"issuer"`   // 签发方地址，例如 https://accounts.example.com
	ClientID string   `json:"clientId"` // 客户端编号
	Allowed  []string `json:"allowed"`  // 允许登录的账号：邮箱，或 sub:<编号>
	// HasSecret 报告有没有保存客户端密钥（只用于显示）。
	HasSecret bool `json:"hasSecret"`
}

// Identity 是身份提供方证明了的身份。
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Label 是写进审计的身份描述。
func (i Identity) Label() string {
	if i.Email != "" {
		return i.Email + "（sub:" + i.Subject + "）"
	}
	return "sub:" + i.Subject
}

var (
	// ErrDisabled 表示没有启用 OIDC。
	ErrDisabled = errors.New("没有启用 OIDC 登录")
	// ErrNotAllowed 表示身份提供方证明了身份，但这个账号不在允许列表里。
	ErrNotAllowed = errors.New("这个账号不在允许登录的列表里")
	// ErrState 表示登录流程对不上（过期、重放，或者不是从这里发起的）。
	ErrState = errors.New("登录流程已过期或无效，请重新登录")
)

const (
	settingKey = "oidc"
	secretName = "oidc/client"
	pendingTTL = 10 * time.Minute
)

var (
	emailRe = regexp.MustCompile(`^[^@\s<>,;"]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,24}$`)
	subRe   = regexp.MustCompile(`^sub:[\x21-\x7e]{1,255}$`)
)

type pending struct {
	nonce, verifier string
	exp             time.Time
}

// Service 管理 OIDC 的设置和登录流程。
type Service struct {
	db       *sql.DB
	secrets  *secrets.Store
	redirect func() string // 回调地址：访问地址 + /api/v1/auth/oidc/callback

	mu       sync.Mutex
	cfg      Config
	provider *gooidc.Provider
	provFor  string // provider 是为哪个签发方建的
	pending  map[string]pending
}

// New 读出保存的设置。
func New(db *sql.DB, sec *secrets.Store, redirect func() string) (*Service, error) {
	s := &Service{db: db, secrets: sec, redirect: redirect, pending: map[string]pending{}}
	var v string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", settingKey).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if v != "" {
		json.Unmarshal([]byte(v), &s.cfg)
	}
	return s, nil
}

// Config 返回当前设置。
func (s *Service) Config() Config {
	s.mu.Lock()
	c := s.cfg
	s.mu.Unlock()
	c.Allowed = append([]string{}, c.Allowed...)
	_, err := s.secrets.Get(secretName)
	c.HasSecret = err == nil
	return c
}

// Enabled 报告登录页要不要显示 OIDC 按钮，以及按钮上的名字。
func (s *Service) Enabled() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Enabled, s.cfg.Name
}

// RedirectURL 是要填到身份提供方那边的回调地址。
func (s *Service) RedirectURL() string { return s.redirect() }

func validIssuer(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("签发方地址不对")
	}
	if u.Scheme == "https" {
		return nil
	}
	// 只在本机测试时允许 http。
	if h := u.Hostname(); u.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1") {
		return nil
	}
	return errors.New("签发方地址必须是 https")
}

// Save 保存设置。secret 为空表示保持原来的客户端密钥；clearSecret 为 true 时删掉它（公开客户端只用 PKCE）。
// 启用时会去签发方读一次发现文档，读不到就不保存。
func (s *Service) Save(ctx context.Context, c Config, secret string, clearSecret bool) (Config, error) {
	c.Name = strings.TrimSpace(c.Name)
	c.Issuer = strings.TrimRight(strings.TrimSpace(c.Issuer), "/")
	c.ClientID = strings.TrimSpace(c.ClientID)
	if c.Name == "" {
		c.Name = "OIDC"
	}
	if len([]rune(c.Name)) > 30 {
		return Config{}, errors.New("按钮名字最长 30 个字")
	}
	var allowed []string
	seen := map[string]bool{}
	for _, a := range c.Allowed {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !strings.HasPrefix(a, "sub:") {
			a = strings.ToLower(a)
			if !emailRe.MatchString(a) {
				return Config{}, fmt.Errorf("%s 不是邮箱，也不是 sub:<编号>", a)
			}
		} else if !subRe.MatchString(a) {
			return Config{}, fmt.Errorf("%s 格式不对", a)
		}
		if !seen[a] {
			seen[a] = true
			allowed = append(allowed, a)
		}
	}
	if len(allowed) > 50 {
		return Config{}, errors.New("允许的账号最多 50 个")
	}
	c.Allowed = allowed
	if c.Enabled || c.Issuer != "" {
		if err := validIssuer(c.Issuer); err != nil {
			return Config{}, err
		}
	}
	if c.Enabled {
		if c.ClientID == "" {
			return Config{}, errors.New("要填客户端编号（Client ID）")
		}
		if len(allowed) == 0 {
			return Config{}, errors.New("至少要允许一个账号，否则谁也登录不了")
		}
	}
	if secret != "" || clearSecret {
		if !s.secrets.Available() {
			return Config{}, errors.New("没有配置主密钥，不能保存客户端密钥")
		}
	}
	var prov *gooidc.Provider
	if c.Enabled {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		p, err := gooidc.NewProvider(cctx, c.Issuer)
		if err != nil {
			return Config{}, fmt.Errorf("读不到签发方的发现文档（%s/.well-known/openid-configuration）：%v", c.Issuer, err)
		}
		prov = p
	}
	switch {
	case clearSecret:
		s.secrets.Delete(secretName)
	case secret != "":
		if err := s.secrets.Put(secretName, []byte(secret)); err != nil {
			return Config{}, err
		}
	}
	c.HasSecret = false
	v, _ := json.Marshal(c)
	if _, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingKey, string(v)); err != nil {
		return Config{}, err
	}
	s.mu.Lock()
	s.cfg, s.provider, s.provFor = c, prov, c.Issuer
	s.mu.Unlock()
	return s.Config(), nil
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// oauth 准备好这次登录要用的客户端设置（第一次用时去读发现文档）。
func (s *Service) oauth(ctx context.Context) (*oauth2.Config, *gooidc.Provider, Config, error) {
	s.mu.Lock()
	c, p, forIssuer := s.cfg, s.provider, s.provFor
	s.mu.Unlock()
	if !c.Enabled {
		return nil, nil, c, ErrDisabled
	}
	if p == nil || forIssuer != c.Issuer {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		np, err := gooidc.NewProvider(cctx, c.Issuer)
		if err != nil {
			return nil, nil, c, fmt.Errorf("连不上身份提供方：%v", err)
		}
		s.mu.Lock()
		s.provider, s.provFor = np, c.Issuer
		s.mu.Unlock()
		p = np
	}
	secret, _ := s.secrets.Get(secretName)
	return &oauth2.Config{ClientID: c.ClientID, ClientSecret: string(secret), Endpoint: p.Endpoint(),
		RedirectURL: s.redirect(), Scopes: []string{gooidc.ScopeOpenID, "email", "profile"}}, p, c, nil
}

// Begin 开始一次登录：返回 state（放进只给回调路径用的 Cookie）和要跳转去的地址。
func (s *Service) Begin(ctx context.Context) (state, authURL string, err error) {
	oc, _, _, err := s.oauth(ctx)
	if err != nil {
		return "", "", err
	}
	state, nonce, verifier := random(32), random(32), oauth2.GenerateVerifier()
	s.mu.Lock()
	now := time.Now()
	for k, p := range s.pending {
		if now.After(p.exp) {
			delete(s.pending, k)
		}
	}
	if len(s.pending) > 1000 {
		s.mu.Unlock()
		return "", "", errors.New("同时进行中的登录太多，请稍后再试")
	}
	s.pending[state] = pending{nonce: nonce, verifier: verifier, exp: now.Add(pendingTTL)}
	s.mu.Unlock()
	return state, oc.AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), nil
}

// Finish 用回调带回来的 code 换令牌并校验身份令牌。cookieState 是浏览器 Cookie 里的 state，必须和地址里的一致。
// 返回的身份即使不在允许列表里也会返回（同时返回 ErrNotAllowed），方便写审计。
func (s *Service) Finish(ctx context.Context, cookieState, state, code string) (Identity, error) {
	if state == "" || cookieState == "" || state != cookieState || code == "" {
		return Identity{}, ErrState
	}
	s.mu.Lock()
	p, ok := s.pending[state]
	delete(s.pending, state) // 每个 state 只能用一次
	s.mu.Unlock()
	if !ok || time.Now().After(p.exp) {
		return Identity{}, ErrState
	}
	oc, prov, c, err := s.oauth(ctx)
	if err != nil {
		return Identity{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tok, err := oc.Exchange(cctx, code, oauth2.VerifierOption(p.verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("用授权码换令牌失败：%v", errText(err))
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Identity{}, errors.New("身份提供方没有返回身份令牌（ID Token）")
	}
	idt, err := prov.Verifier(&gooidc.Config{ClientID: c.ClientID}).Verify(cctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("身份令牌校验失败：%v", err)
	}
	if idt.Nonce != p.nonce {
		return Identity{}, errors.New("身份令牌校验失败：nonce 对不上")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		Preferred     string `json:"preferred_username"`
	}
	if err := idt.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("读不出身份令牌里的信息：%v", err)
	}
	id := Identity{Subject: idt.Subject, Email: strings.ToLower(strings.TrimSpace(claims.Email)), Name: claims.Name}
	if id.Name == "" {
		id.Name = claims.Preferred
	}
	// 有的提供方把 email_verified 写成字符串 "true"。
	switch v := claims.EmailVerified.(type) {
	case bool:
		id.EmailVerified = v
	case string:
		id.EmailVerified = v == "true"
	}
	if !allowedID(c.Allowed, id) {
		return id, ErrNotAllowed
	}
	return id, nil
}

// allowedID：sub 精确匹配；邮箱不分大小写匹配，并且必须是身份提供方验证过的邮箱（否则谁都能在提供方那边填一个别人的邮箱）。
func allowedID(list []string, id Identity) bool {
	for _, a := range list {
		if sub, ok := strings.CutPrefix(a, "sub:"); ok {
			if sub == id.Subject && id.Subject != "" {
				return true
			}
			continue
		}
		if id.EmailVerified && id.Email != "" && strings.EqualFold(a, id.Email) {
			return true
		}
	}
	return false
}

// errText 去掉错误里可能带着的地址参数，只留原因。
func errText(err error) string {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorCode != "" {
			return re.ErrorCode + " " + re.ErrorDescription
		}
		return fmt.Sprintf("对方返回 %d", re.Response.StatusCode)
	}
	return err.Error()
}
