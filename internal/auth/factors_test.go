package auth

import (
	"encoding/base32"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/store"
)

type memSecrets map[string][]byte

// 和真的凭据存储用同一条命名规则，名字不合格时一样报错。
var secretNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)+$`)

func (m memSecrets) Put(n string, b []byte) error {
	if !secretNameRe.MatchString(n) {
		return errors.New("凭据名称不合格：" + n)
	}
	m[n] = append([]byte(nil), b...)
	return nil
}
func (m memSecrets) Get(n string) ([]byte, error) {
	if b, ok := m[n]; ok {
		return b, nil
	}
	return nil, errors.New("没有")
}
func (m memSecrets) Delete(n string) error { delete(m, n); return nil }
func (m memSecrets) Available() bool       { return true }

func newService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db, "https://hostower.example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	s.SetDataDir(dir)
	s.SetSecrets(memSecrets{})
	return s, dir
}

func readInitial(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, InitialPasswordFile))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func TestTOTPVectors(t *testing.T) {
	// RFC 6238 附录 B 的 SHA-1 测试向量（取后 6 位）。
	secret := []byte("12345678901234567890")
	for ts, want := range map[int64]string{59: "287082", 1111111109: "081804", 1234567890: "005924", 2000000000: "279037"} {
		if got := totpAt(secret, ts/totpPeriod); got != want {
			t.Fatalf("时间 %d：得到 %s，应该是 %s", ts, got, want)
		}
	}
	now := time.Unix(1111111109, 0)
	if _, ok := matchTOTP(secret, "081804", now.Add(30*time.Second)); !ok {
		t.Fatal("前一个时间步的码也应该认")
	}
	if _, ok := matchTOTP(secret, "081804", now.Add(90*time.Second)); ok {
		t.Fatal("太旧的码不该认")
	}
}

func TestPasswordHash(t *testing.T) {
	h := hashPassword("correct horse battery")
	if !checkPassword(h, "correct horse battery") || checkPassword(h, "correct horse batterY") {
		t.Fatal("密码校验不对")
	}
	if p := GeneratePassword(); len(p) != 32 {
		t.Fatalf("初始密码应该是 32 位：%q", p)
	}
}

func TestInitialPasswordLogin(t *testing.T) {
	s, dir := newService(t)
	path, err := s.EnsureOwner("")
	if err != nil || path == "" {
		t.Fatal("首次启动应该创建所有者并写初始密码文件", err)
	}
	if info, _ := os.Stat(path); info == nil {
		t.Fatal("初始密码文件不存在")
	}
	if again, _ := s.EnsureOwner(""); again != "" {
		t.Fatal("已经有所有者时不该再生成")
	}
	pw := readInitial(t, dir)
	if _, _, _, err := s.PasswordLogin("admin", pw+"x"); !errors.Is(err, ErrBadPassword) {
		t.Fatal("错的密码应该被拒绝", err)
	}
	if _, _, _, err := s.PasswordLogin("nobody", pw); !errors.Is(err, ErrBadPassword) {
		t.Fatal("不存在的用户名应该和密码错一样报错", err)
	}
	tok, u, ticket, err := s.PasswordLogin("Admin", pw)
	if err != nil || tok == "" || ticket != "" || u.Role != "owner" {
		t.Fatal("初始密码应该能登录", err)
	}
	sess, _ := s.Lookup(tok)
	if sess.Method != MethodPassword || !s.Factors(u.ID).PasswordInitial {
		t.Fatal("会话应该记成密码登录，并且标记为初始密码")
	}
	// 什么强验证都没有时，用密码验证身份；但它不能用来批 L2。
	if err := s.VerifyPassword(sess, pw); err != nil || !s.Verified(sess) || s.ConsumeReauth(sess) {
		t.Fatal("密码验证只算管理设置用的验证，不算 L2", err)
	}
	if err := s.SetPassword(u.ID, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatal("太短的密码应该被拒绝")
	}
	if err := s.SetPassword(u.ID, "a much better password"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, InitialPasswordFile)); !os.IsNotExist(err) {
		t.Fatal("改过密码后初始密码文件应该删掉")
	}
	if s.Factors(u.ID).PasswordInitial {
		t.Fatal("改过密码后不再是初始密码")
	}
}

func TestLockoutBackoff(t *testing.T) {
	s, dir := newService(t)
	s.EnsureOwner("")
	pw := readInitial(t, dir)
	for i := 0; i < 5; i++ {
		s.PasswordLogin("admin", "wrong password!")
	}
	if _, _, _, err := s.PasswordLogin("admin", pw); err == nil || errors.Is(err, ErrBadPassword) {
		t.Fatal("连续失败 5 次后应该要等一会儿，连对的密码也先不收", err)
	}
}

func TestTOTPFlow(t *testing.T) {
	s, dir := newService(t)
	s.EnsureOwner("")
	pw := readInitial(t, dir)
	tok, _, _, _ := s.PasswordLogin("admin", pw)
	sess, _ := s.Lookup(tok)
	secret, uri, err := s.TOTPBegin(sess)
	if err != nil || !strings.HasPrefix(uri, "otpauth://totp/Hostower:admin?secret="+secret) {
		t.Fatal("绑定地址不对", uri, err)
	}
	raw, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	now := time.Now().Unix() / totpPeriod
	if err := s.TOTPFinish(sess, "000000"); err == nil && totpAt(raw, now) != "000000" {
		t.Fatal("错的码不能完成绑定")
	}
	if err := s.TOTPFinish(sess, totpAt(raw, now)); err != nil {
		t.Fatal(err)
	}
	// 绑定用过的码不能再用来验证；下一个时间步的码可以，并且能批一次 L2。
	if err := s.VerifyTOTP(sess, totpAt(raw, now)); err == nil {
		t.Fatal("同一个码不能用两次")
	}
	if err := s.VerifyTOTP(sess, totpAt(raw, now+1)); err != nil || !s.ConsumeReauth(sess) || s.ConsumeReauth(sess) {
		t.Fatal("动态码验证应该能批一次 L2，而且只能一次", err)
	}
	// 有了动态码，密码不再能用来验证身份。
	if err := s.VerifyPassword(sess, pw); !errors.Is(err, ErrVerifyNotOffer) {
		t.Fatal("有动态码时不能只用密码验证", err)
	}
	// 密码登录要多一步动态码。
	_, _, ticket, err := s.PasswordLogin("admin", pw)
	if err != nil || ticket == "" {
		t.Fatal("绑了动态码后，密码登录应该要求动态码", err)
	}
	if _, _, err := s.PasswordTOTP(ticket, "12345"); err == nil {
		t.Fatal("错的动态码不能登录")
	}
	s.db.Exec("UPDATE user_totp SET last_step = 0")
	tok2, _, err := s.PasswordTOTP(ticket, totpAt(raw, now))
	if err != nil {
		t.Fatal(err)
	}
	if sess2, _ := s.Lookup(tok2); sess2.Method != MethodPasswordTOTP {
		t.Fatal("会话应该记成密码加动态码")
	}
}

func TestMethodsLockout(t *testing.T) {
	s, _ := newService(t)
	s.EnsureOwner("")
	// 只有密码：关掉密码登录会把自己锁在外面。
	if err := s.SetMethods(Methods{Passkey: true, Password: false}, false); !errors.Is(err, ErrLockout) {
		t.Fatal("没有别的登录方式时不能关掉密码", err)
	}
	// 开着单点登录就可以。
	if err := s.SetMethods(Methods{Passkey: true, Password: false, DefaultLogin: MethodOIDC}, true); err != nil {
		t.Fatal(err)
	}
	if list, def := s.LoginMethods(true); len(list) != 1 || def != MethodOIDC {
		t.Fatal("登录页只该剩单点登录", list, def)
	}
	if err := s.SetMethods(Methods{Passkey: true, Password: true, DefaultLogin: "sms"}, false); err == nil {
		t.Fatal("不认识的默认方式应该被拒绝")
	}
	if _, _, _, err := s.PasswordLogin("admin", "whatever-long-pass"); !errors.Is(err, ErrMethodDisabled) {
		t.Fatal("关掉的方式不能登录", err)
	}
}

func TestResetPassword(t *testing.T) {
	s, dir := newService(t)
	s.EnsureOwner("")
	old := readInitial(t, dir)
	tok, _, _, _ := s.PasswordLogin("admin", old)
	s.SetMethods(Methods{Passkey: true, Password: false, DefaultLogin: MethodOIDC}, true)
	if _, err := s.ResetPassword(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lookup(tok); err == nil {
		t.Fatal("重置密码后旧会话应该失效")
	}
	pw := readInitial(t, dir)
	if pw == old || !s.Methods().Password {
		t.Fatal("应该生成新密码并打开密码登录")
	}
	if _, _, _, err := s.PasswordLogin("admin", pw); err != nil {
		t.Fatal(err)
	}
}

func TestRename(t *testing.T) {
	s, dir := newService(t)
	s.EnsureOwner("")
	pw := readInitial(t, dir)
	var id string
	s.db.QueryRow("SELECT id FROM users").Scan(&id)
	for _, bad := range []string{"", "ab", "Admin!", "1admin", "a b c", strings.Repeat("a", 33)} {
		if !errors.Is(s.Rename(id, bad), ErrBadUsername) {
			t.Fatalf("%q 应该不合规", bad)
		}
	}
	if err := s.Rename(id, "tsien.ops"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := s.PasswordLogin("admin", pw); err == nil {
		t.Fatal("改名后旧名字不该还能登录")
	}
	if _, _, _, err := s.PasswordLogin("tsien.ops", pw); err != nil {
		t.Fatal("改名后应该用新名字登录：", err)
	}
	s.db.Exec("INSERT INTO users (id, name, role, created_at) VALUES ('u2', 'other', 'viewer', '2026-01-01T00:00:00Z')")
	if !errors.Is(s.Rename(id, "Other"), ErrBadUsername) || !errors.Is(s.Rename(id, "other"), ErrUsernameTaken) {
		t.Fatal("重名应该被拒绝")
	}
	if _, err := s.EnsureOwner("Bad Name"); err != nil {
		t.Fatal("已经有用户时不该再检查名字：", err)
	}
}
