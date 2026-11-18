package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/tsienwan/hostower/internal/secrets"
	"github.com/tsienwan/hostower/internal/store"
)

// fakeIdP 是一个最小的身份提供方：发现文档、公钥、令牌端点。它会检查 PKCE。
type fakeIdP struct {
	srv       *httptest.Server
	key       *rsa.PrivateKey
	mu        sync.Mutex
	challenge string // 授权请求里的 code_challenge
	nonce     string
	email     string
	verified  any
	sub       string
}

func newIdP(t *testing.T) *fakeIdP {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	f := &fakeIdP{key: key, sub: "user-123", email: "ops@example.com", verified: true}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/auth", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri": f.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		r.ParseForm()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		f.mu.Lock()
		defer f.mu.Unlock()
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("code") != "good-code" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		claims, _ := json.Marshal(map[string]any{"iss": f.srv.URL, "aud": "hostower", "sub": f.sub, "nonce": f.nonce,
			"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "email": f.email, "email_verified": f.verified})
		jws, _ := signer.Sign(claims)
		idt, _ := jws.CompactSerialize()
		json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": idt, "expires_in": 3600})
	})
	return f
}

func (f *fakeIdP) authorize(t *testing.T, authURL string) string {
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != "hostower" {
		t.Fatalf("授权请求缺少 PKCE 或客户端编号：%s", authURL)
	}
	f.mu.Lock()
	f.challenge, f.nonce = q.Get("code_challenge"), q.Get("nonce")
	f.mu.Unlock()
	return q.Get("state")
}

func setup(t *testing.T) (*Service, *fakeIdP) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := make([]byte, 32)
	rand.Read(key)
	s, err := New(db, secrets.New(db, key), func() string { return "https://hostower.example.com/api/v1/auth/oidc/callback" })
	if err != nil {
		t.Fatal(err)
	}
	f := newIdP(t)
	if _, err := s.Save(context.Background(), Config{Enabled: true, Name: "公司账号", Issuer: f.srv.URL, ClientID: "hostower",
		Allowed: []string{"OPS@example.com", "sub:robot-7"}}, "s3cret", false); err != nil {
		t.Fatal(err)
	}
	return s, f
}

func TestLoginFlow(t *testing.T) {
	s, f := setup(t)
	ctx := context.Background()
	if c := s.Config(); !c.HasSecret || c.Allowed[0] != "ops@example.com" {
		t.Fatalf("设置没存好：%+v", c)
	}

	state, authURL, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if f.authorize(t, authURL) != state {
		t.Fatal("跳转地址里的 state 不对")
	}
	// Cookie 里的 state 和地址里的不一致：拒绝。
	if _, err := s.Finish(ctx, "other", state, "good-code"); !errors.Is(err, ErrState) {
		t.Fatalf("state 不一致应该拒绝：%v", err)
	}
	// 上面那次已经把这个 state 用掉了，重新开始。
	state, authURL, _ = s.Begin(ctx)
	f.authorize(t, authURL)
	id, err := s.Finish(ctx, state, state, "good-code")
	if err != nil || id.Email != "ops@example.com" || id.Subject != "user-123" {
		t.Fatalf("应该登录成功：%+v %v", id, err)
	}
	// 同一个 state 不能再用一次。
	if _, err := s.Finish(ctx, state, state, "good-code"); !errors.Is(err, ErrState) {
		t.Fatalf("state 只能用一次：%v", err)
	}

	// 邮箱没有经过验证：即使邮箱对得上也不行。
	f.verified = false
	state, authURL, _ = s.Begin(ctx)
	f.authorize(t, authURL)
	if _, err := s.Finish(ctx, state, state, "good-code"); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("没验证的邮箱不应该能登录：%v", err)
	}
	// 按 sub 允许的账号不看邮箱。
	f.sub, f.email = "robot-7", ""
	state, authURL, _ = s.Begin(ctx)
	f.authorize(t, authURL)
	if _, err := s.Finish(ctx, state, state, "good-code"); err != nil {
		t.Fatalf("按 sub 允许的账号应该能登录：%v", err)
	}
	// 授权码不对：换令牌失败。
	state, authURL, _ = s.Begin(ctx)
	f.authorize(t, authURL)
	if _, err := s.Finish(ctx, state, state, "bad-code"); err == nil {
		t.Fatal("授权码不对应该失败")
	}
}

func TestSaveValidation(t *testing.T) {
	s, f := setup(t)
	ctx := context.Background()
	bad := []Config{
		{Enabled: true, Issuer: "http://idp.example.com", ClientID: "x", Allowed: []string{"a@example.com"}}, // 非本机的 http
		{Enabled: true, Issuer: f.srv.URL, ClientID: "", Allowed: []string{"a@example.com"}},
		{Enabled: true, Issuer: f.srv.URL, ClientID: "x"},                              // 谁都不允许
		{Enabled: true, Issuer: f.srv.URL, ClientID: "x", Allowed: []string{"nobody"}}, // 既不是邮箱也不是 sub:
	}
	for i, c := range bad {
		if _, err := s.Save(ctx, c, "", false); err == nil {
			t.Errorf("第 %d 个应该被拒绝：%+v", i, c)
		}
	}
	// 关掉之后 Begin 不能用。
	if _, err := s.Save(ctx, Config{Enabled: false, Issuer: f.srv.URL, ClientID: "hostower"}, "", true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Begin(ctx); !errors.Is(err, ErrDisabled) {
		t.Fatalf("关掉后不能开始登录：%v", err)
	}
	if s.Config().HasSecret {
		t.Fatal("选择清除后客户端密钥应该被删掉")
	}
}
