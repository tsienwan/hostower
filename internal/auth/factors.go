package auth

// 除了 Passkey 之外的登录和验证方式：密码（可以再加一道动态码）、动态码（TOTP，只作第二道和身份验证，不能单独登录），
// 以及“登录方式”的设置和“身份验证”的状态。
//
// 规则：
//   - 至少要留一种能用的登录方式（开着、并且账号上真的设置了）。关掉或删掉最后一种会被拒绝。
//   - 管理登录方式、密码、动态码、Passkey 之前要先验证身份，验证 5 分钟内有效。
//     账号有 Passkey 或动态码时只能用它们验证；两样都没有时才用密码；什么都没有时（比如只用单点登录），刚登录 5 分钟内算验证过。
//   - 高危操作（L2）的批准要当场用 Passkey 或动态码验证，验证一次只能批一次；密码不算。

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// 登录方式（会话里记的）。
const (
	MethodPassword     = "password"
	MethodPasswordTOTP = "password+totp"
)

// 验证方式。
const (
	VerifyPasskey  = "passkey"
	VerifyTOTP     = "totp"
	VerifyPassword = "password"
)

const (
	verifyWindow = 5 * time.Minute // 管理账号安全设置时，一次验证的有效期
	l2Window     = 2 * time.Minute // 批准 L2 时，一次验证的有效期（只能用一次）
	totpTicket   = 5 * time.Minute // 密码对了、等动态码的那一步
	totpPending  = 10 * time.Minute

	// InitialPasswordFile 是首次启动时写初始密码的文件（在数据目录里）。
	InitialPasswordFile = "initial-password"

	methodsKey = "auth.methods"
	minPassLen = 12
	maxPassLen = 256
)

var (
	ErrBadPassword     = errors.New("用户名或密码不正确")
	ErrBadCode         = errors.New("动态码不正确")
	ErrMethodDisabled  = errors.New("这种登录方式没有开启")
	ErrNeedVerify      = errors.New("需要先验证身份")
	ErrWeakPassword    = fmt.Errorf("密码至少 %d 个字符", minPassLen)
	ErrLockout         = errors.New("这样改之后就没有能用的登录方式了：至少保留一种开着、并且已经设置好的登录方式")
	ErrVerifyNotOffer  = errors.New("这个账号已经有 Passkey 或动态码，请用它们验证")
	ErrNoStrongFactor  = errors.New("高危操作需要用 Passkey 或动态码验证，请先在“设置 → 安全”里添加")
	ErrSecretsMissing  = errors.New("凭据存储不可用，没法保存动态码")
	ErrTOTPExists      = errors.New("已经绑定了动态码，先解绑再重新绑定")
	ErrNoPendingTOTP   = errors.New("绑定流程已过期，请重新开始")
	errUnknownDefaults = errors.New("默认方式不是可选的值")
)

// Secrets 是凭据存储里认证要用到的那部分（动态码的密钥存在这里）。
type Secrets interface {
	Put(name string, plaintext []byte) error
	Get(name string) ([]byte, error)
	Delete(name string) error
	Available() bool
}

// SetSecrets 接上凭据存储。
func (s *Service) SetSecrets(x Secrets) { s.secrets = x }

// SetDataDir 告诉认证服务数据目录在哪（初始密码文件放在这里）。
func (s *Service) SetDataDir(dir string) { s.dataDir = dir }

// ---------- 登录方式的设置 ----------

// Methods 是所有者设定的登录方式。单点登录的开关在它自己的设置里。
type Methods struct {
	Passkey       bool   `json:"passkey"`
	Password      bool   `json:"password"`
	DefaultLogin  string `json:"defaultLogin"`  // passkey | password | oidc；空表示自动
	DefaultVerify string `json:"defaultVerify"` // passkey | totp | password；空表示自动
}

// DefaultMethods 是没有设置过时的值：两种都开，默认方式自动挑。
var DefaultMethods = Methods{Passkey: true, Password: true}

func (m Methods) valid() bool {
	switch m.DefaultLogin {
	case "", MethodPasskey, MethodPassword, MethodOIDC:
	default:
		return false
	}
	switch m.DefaultVerify {
	case "", VerifyPasskey, VerifyTOTP, VerifyPassword:
	default:
		return false
	}
	return true
}

// Methods 读当前的登录方式设置。
func (s *Service) Methods() Methods {
	var v string
	if s.db.QueryRow("SELECT value FROM settings WHERE key = ?", methodsKey).Scan(&v) == nil {
		var m Methods
		if json.Unmarshal([]byte(v), &m) == nil && m.valid() {
			return m
		}
	}
	return DefaultMethods
}

// Factors 是一个账号上已经设置好的东西。
type Factors struct {
	Passkeys        int  `json:"passkeys"`
	TOTP            bool `json:"totp"`
	Password        bool `json:"password"`
	PasswordInitial bool `json:"passwordInitial"`
}

// Factors 查一个账号设置了哪些登录和验证方式。
func (s *Service) Factors(userID string) Factors {
	var f Factors
	s.db.QueryRow("SELECT COUNT(*) FROM passkeys WHERE user_id = ?", userID).Scan(&f.Passkeys)
	var initial int
	if s.db.QueryRow("SELECT initial FROM user_passwords WHERE user_id = ?", userID).Scan(&initial) == nil {
		f.Password, f.PasswordInitial = true, initial == 1
	}
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM user_totp WHERE user_id = ?", userID).Scan(&n)
	f.TOTP = n > 0
	return f
}

// usable 报告在这组设置下，所有者还能不能登录进来。
func usable(m Methods, f Factors, oidc bool) bool {
	return (m.Passkey && f.Passkeys > 0) || (m.Password && f.Password) || oidc
}

// CheckUsable 报告所有者在这组设置下还能不能登录（单点登录开关改动前，网关用它检查）。
func (s *Service) CheckUsable(m Methods, oidc bool) error {
	if !usable(m, s.Factors(s.ownerID()), oidc) {
		return ErrLockout
	}
	return nil
}

// SetMethods 保存登录方式设置。会让所有者登录不进来的设置会被拒绝。
func (s *Service) SetMethods(m Methods, oidc bool) error {
	if !m.valid() {
		return errUnknownDefaults
	}
	if err := s.CheckUsable(m, oidc); err != nil {
		return err
	}
	v, _ := json.Marshal(m)
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, methodsKey, string(v))
	return err
}

// LoginMethods 返回登录页上能用的方式（开着、并且所有者真的设置了），以及默认先显示哪一种。
func (s *Service) LoginMethods(oidc bool) (list []string, def string) {
	list = []string{}
	m, f := s.Methods(), s.Factors(s.ownerID())
	if m.Passkey && f.Passkeys > 0 {
		list = append(list, MethodPasskey)
	}
	if m.Password && f.Password {
		list = append(list, MethodPassword)
	}
	if oidc {
		list = append(list, MethodOIDC)
	}
	def = m.DefaultLogin
	for _, x := range list {
		if x == def {
			return list, def
		}
	}
	if len(list) > 0 {
		def = list[0]
	}
	return list, def
}

// VerifyMethods 返回这个账号能用来验证身份的方式，以及默认先用哪一种。
// l2 为真时只算 Passkey 和动态码。
func (s *Service) VerifyMethods(userID string, l2 bool) (list []string, def string) {
	list = []string{} // 编码成 []，不是 null
	f := s.Factors(userID)
	if f.Passkeys > 0 {
		list = append(list, VerifyPasskey)
	}
	if f.TOTP {
		list = append(list, VerifyTOTP)
	}
	if !l2 && len(list) == 0 && f.Password {
		list = append(list, VerifyPassword)
	}
	def = s.Methods().DefaultVerify
	for _, x := range list {
		if x == def {
			return list, def
		}
	}
	if len(list) > 0 {
		def = list[0]
	}
	return list, def
}

func (s *Service) ownerID() string {
	var id string
	s.db.QueryRow("SELECT id FROM users WHERE role = 'owner' ORDER BY created_at LIMIT 1").Scan(&id)
	return id
}

// ---------- 密码 ----------

// Argon2id 参数（OWASP 推荐的下限：19 MiB、2 轮、1 线程）。
const (
	argonTime   = 2
	argonMemory = 19 * 1024
	argonLanes  = 1
	argonLen    = 32
)

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonLanes, argonLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonLanes,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func checkPassword(encoded, pw string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// dummyHash 用来在用户名不对时也算一次哈希，让“用户名错”和“密码错”花的时间一样。
var dummyHash = hashPassword("hostower-dummy-password")

const passChars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789-_.!@#%^*+="

// GeneratePassword 生成一个 32 位的随机复杂密码。
func GeneratePassword() string {
	var b strings.Builder
	max := big.NewInt(int64(len(passChars)))
	for b.Len() < 32 {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		b.WriteByte(passChars[n.Int64()])
	}
	return b.String()
}

func (s *Service) storePassword(userID, pw string, initial bool) error {
	if len(pw) < minPassLen {
		return ErrWeakPassword
	}
	if len(pw) > maxPassLen {
		return fmt.Errorf("密码最多 %d 个字符", maxPassLen)
	}
	flag := 0
	if initial {
		flag = 1
	}
	_, err := s.db.Exec(`INSERT INTO user_passwords (user_id, hash, initial, updated_at) VALUES (?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET hash=excluded.hash, initial=excluded.initial, updated_at=excluded.updated_at`,
		userID, hashPassword(pw), flag, time.Now().UTC().Format(time.RFC3339))
	return err
}

// SetPassword 设置或修改密码（调用方先确认验证过身份）。改完后初始密码文件随之删除。
func (s *Service) SetPassword(userID, pw string) error {
	if err := s.storePassword(userID, pw, false); err != nil {
		return err
	}
	s.removeInitialFile()
	return nil
}

func (s *Service) removeInitialFile() {
	if s.dataDir != "" {
		os.Remove(filepath.Join(s.dataDir, InitialPasswordFile))
	}
}

func (s *Service) writeInitialFile(pw string) (string, error) {
	if s.dataDir == "" {
		return "", errors.New("没有数据目录")
	}
	path := filepath.Join(s.dataDir, InitialPasswordFile)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(pw + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// DefaultOwnerName 是首次启动时所有者账号的默认用户名（HOSTOWER_ADMIN_USER 可以换掉，登录后也能在“设置 → 安全”里改）。
const DefaultOwnerName = "admin"

// usernameRe 是用户名允许的样子：小写字母开头，3 到 32 个字符，只有小写字母、数字、点、下划线和连字符。
var usernameRe = regexp.MustCompile(`^[a-z][a-z0-9._-]{2,31}$`)

// ErrBadUsername 和 ErrUsernameTaken 是改用户名时的错误。
var (
	ErrBadUsername   = errors.New("用户名要以小写字母开头，3 到 32 个字符，只能有小写字母、数字、点、下划线和连字符")
	ErrUsernameTaken = errors.New("这个用户名已经有人用了")
)

// ValidUsername 报告用户名是否合规。
func ValidUsername(name string) bool { return usernameRe.MatchString(name) }

// Rename 修改一个账号的用户名。账号编号不变：会话、动态码、Passkey 都挂在编号上，不受影响；
// 审计里已经写下的旧名字保持原样（审计只追加）。
func (s *Service) Rename(userID, name string) error {
	if !ValidUsername(name) {
		return ErrBadUsername
	}
	var other string
	if s.db.QueryRow("SELECT id FROM users WHERE lower(name) = lower(?)", name).Scan(&other) == nil && other != userID {
		return ErrUsernameTaken
	}
	res, err := s.db.Exec("UPDATE users SET name = ? WHERE id = ?", name, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnauthorized
	}
	return nil
}

// EnsureOwner 在还没有任何用户时创建所有者，生成初始密码并写进数据目录里的文件（权限 0600，不写日志）。
// name 是用户名，空的时候用 DefaultOwnerName。返回文件路径；已经有用户时返回空。
func (s *Service) EnsureOwner(name string) (string, error) {
	need, err := s.NeedsSetup()
	if err != nil || !need {
		return "", err
	}
	if name == "" {
		name = DefaultOwnerName
	}
	if !ValidUsername(name) {
		return "", ErrBadUsername
	}
	pw := GeneratePassword()
	u := &User{ID: "u_" + randomToken(9), Name: name, Role: "owner"}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec("INSERT INTO users (id, name, role, created_at) VALUES (?,?,?,?)", u.ID, u.Name, u.Role, now); err != nil {
		return "", err
	}
	if _, err := tx.Exec("INSERT INTO user_passwords (user_id, hash, initial, updated_at) VALUES (?,?,1,?)", u.ID, hashPassword(pw), now); err != nil {
		return "", err
	}
	path, err := s.writeInitialFile(pw)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// ResetPassword 给所有者重新生成一个初始密码（命令行 hostower auth reset-password 用）：
// 写进初始密码文件、打开密码登录、吊销所有会话。审计由命令行写。
func (s *Service) ResetPassword() (string, error) {
	id := s.ownerID()
	if id == "" {
		return "", ErrSetupRequired
	}
	pw := GeneratePassword()
	if err := s.storePassword(id, pw, true); err != nil {
		return "", err
	}
	m := s.Methods()
	m.Password = true
	v, _ := json.Marshal(m)
	s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, methodsKey, string(v))
	s.db.Exec("DELETE FROM sessions")
	return s.writeInitialFile(pw)
}

// ---------- 失败计数 ----------

// 同一个账号连续失败 5 次之后开始等待：1 分钟，之后每次翻倍，最多 15 分钟。成功一次就清零。
type failure struct {
	n     int
	until time.Time
}

func (s *Service) locked(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.fails[userID]; f != nil && time.Now().Before(f.until) {
		return fmt.Errorf("失败次数太多，请 %d 秒后再试", int(time.Until(f.until).Seconds())+1)
	}
	return nil
}

func (s *Service) failed(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.fails[userID]
	if f == nil {
		f = &failure{}
		s.fails[userID] = f
	}
	f.n++
	if f.n >= 5 {
		d := time.Minute << min(f.n-5, 4)
		if d > 15*time.Minute {
			d = 15 * time.Minute
		}
		f.until = time.Now().Add(d)
	}
}

func (s *Service) succeeded(userID string) {
	s.mu.Lock()
	delete(s.fails, userID)
	s.mu.Unlock()
}

// ---------- 密码登录 ----------

// PasswordLogin 校验用户名和密码。账号绑了动态码时不直接登录，返回一张 5 分钟内有效的票，用它和动态码换会话。
func (s *Service) PasswordLogin(name, pw string) (token string, u *User, ticket string, err error) {
	if !s.Methods().Password {
		return "", nil, "", ErrMethodDisabled
	}
	u = &User{}
	var hash string
	qerr := s.db.QueryRow(`SELECT u.id, u.name, u.role, p.hash FROM users u JOIN user_passwords p ON p.user_id = u.id WHERE lower(u.name) = lower(?)`,
		strings.TrimSpace(name)).Scan(&u.ID, &u.Name, &u.Role, &hash)
	if qerr != nil {
		checkPassword(dummyHash, pw)
		return "", nil, "", ErrBadPassword
	}
	if err := s.locked(u.ID); err != nil {
		return "", nil, "", err
	}
	if !checkPassword(hash, pw) {
		s.failed(u.ID)
		return "", nil, "", ErrBadPassword
	}
	if s.Factors(u.ID).TOTP {
		ticket = randomToken(16)
		s.mu.Lock()
		s.tickets[ticket] = &ticketState{user: u, expires: time.Now().Add(totpTicket)}
		s.mu.Unlock()
		return "", u, ticket, nil
	}
	s.succeeded(u.ID)
	token, err = s.newSession(u.ID, MethodPassword)
	return token, u, "", err
}

type ticketState struct {
	user    *User
	expires time.Time
	tries   int
}

// PasswordTOTP 用密码那一步拿到的票和动态码换会话。同一张票最多试 5 次。
func (s *Service) PasswordTOTP(ticket, code string) (string, *User, error) {
	s.mu.Lock()
	t := s.tickets[ticket]
	if t == nil || time.Now().After(t.expires) || t.tries >= 5 {
		delete(s.tickets, ticket)
		s.mu.Unlock()
		return "", nil, ErrCeremony
	}
	t.tries++
	s.mu.Unlock()
	if err := s.locked(t.user.ID); err != nil {
		return "", nil, err
	}
	if !s.checkTOTP(t.user.ID, code) {
		s.failed(t.user.ID)
		return "", nil, ErrBadCode
	}
	s.mu.Lock()
	delete(s.tickets, ticket)
	s.mu.Unlock()
	s.succeeded(t.user.ID)
	token, err := s.newSession(t.user.ID, MethodPasswordTOTP)
	return token, t.user, err
}

// ---------- 动态码（TOTP，RFC 6238：SHA-1、6 位、30 秒） ----------

const totpPeriod = 30

func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000)
}

// matchTOTP 在前后各一个时间步的范围里找对得上的码，返回对上的时间步。
func matchTOTP(secret []byte, code string, now time.Time) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	cur := now.Unix() / totpPeriod
	for _, step := range []int64{cur - 1, cur, cur + 1} {
		if subtle.ConstantTimeCompare([]byte(totpAt(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// totpRef 是动态码密钥在凭据存储里的名字。凭据名只能用小写，用户编号里有大写，所以取它的哈希。
func totpRef(userID string) string { return "auth/totp/" + hashToken(userID)[:24] }

// checkTOTP 校验动态码；同一个码（同一个时间步或更早的）不能用第二次。
func (s *Service) checkTOTP(userID, code string) bool {
	if s.secrets == nil {
		return false
	}
	var ref string
	if s.db.QueryRow("SELECT secret_ref FROM user_totp WHERE user_id = ?", userID).Scan(&ref) != nil {
		return false
	}
	secret, err := s.secrets.Get(ref)
	if err != nil {
		return false
	}
	step, ok := matchTOTP(secret, code, time.Now())
	if !ok {
		return false
	}
	res, err := s.db.Exec("UPDATE user_totp SET last_step = ? WHERE user_id = ? AND last_step < ?", step, userID, step)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

type pendingTOTP struct {
	secret  []byte
	expires time.Time
}

// TOTPBegin 开始绑定动态码：生成密钥，返回给 App 扫的地址和手动输入用的密钥。确认之前不保存。
func (s *Service) TOTPBegin(sess *Session) (secret, uri string, err error) {
	if s.secrets == nil || !s.secrets.Available() {
		return "", "", ErrSecretsMissing
	}
	if s.Factors(sess.User.ID).TOTP {
		return "", "", ErrTOTPExists
	}
	raw := make([]byte, 20)
	rand.Read(raw)
	s.mu.Lock()
	s.pendingTOTP[sess.TokenHash] = &pendingTOTP{secret: raw, expires: time.Now().Add(totpPending)}
	s.mu.Unlock()
	secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	label := url.PathEscape("Hostower:" + sess.User.Name)
	uri = "otpauth://totp/" + label + "?secret=" + secret + "&issuer=Hostower&algorithm=SHA1&digits=6&period=" + strconv.Itoa(totpPeriod)
	return secret, uri, nil
}

// TOTPFinish 用 App 上显示的码确认绑定，然后把密钥存进凭据存储。
func (s *Service) TOTPFinish(sess *Session, code string) error {
	s.mu.Lock()
	p := s.pendingTOTP[sess.TokenHash]
	s.mu.Unlock()
	if p == nil || time.Now().After(p.expires) {
		return ErrNoPendingTOTP
	}
	step, ok := matchTOTP(p.secret, code, time.Now())
	if !ok {
		return ErrBadCode
	}
	ref := totpRef(sess.User.ID)
	if err := s.secrets.Put(ref, p.secret); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT INTO user_totp (user_id, secret_ref, last_step, created_at) VALUES (?,?,?,?)`,
		sess.User.ID, ref, step, time.Now().UTC().Format(time.RFC3339)); err != nil {
		s.secrets.Delete(ref)
		return err
	}
	s.mu.Lock()
	delete(s.pendingTOTP, sess.TokenHash)
	s.mu.Unlock()
	return nil
}

// TOTPRemove 解绑动态码。
func (s *Service) TOTPRemove(userID string) error {
	if _, err := s.db.Exec("DELETE FROM user_totp WHERE user_id = ?", userID); err != nil {
		return err
	}
	if s.secrets != nil {
		s.secrets.Delete(totpRef(userID))
	}
	return nil
}

// ---------- 身份验证 ----------

func (s *Service) markVerified(sess *Session, l2 bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.verified[sess.TokenHash] = now
	if l2 {
		s.l2[sess.TokenHash] = now
	}
}

// VerifyTOTP 用动态码验证身份。算强验证：之后可以批一次 L2。
func (s *Service) VerifyTOTP(sess *Session, code string) error {
	if err := s.locked(sess.User.ID); err != nil {
		return err
	}
	if !s.checkTOTP(sess.User.ID, code) {
		s.failed(sess.User.ID)
		return ErrBadCode
	}
	s.succeeded(sess.User.ID)
	s.markVerified(sess, true)
	return nil
}

// VerifyPassword 用密码验证身份。只有账号上既没有 Passkey 也没有动态码时才能用，并且不能用来批 L2。
func (s *Service) VerifyPassword(sess *Session, pw string) error {
	f := s.Factors(sess.User.ID)
	if f.Passkeys > 0 || f.TOTP {
		return ErrVerifyNotOffer
	}
	if err := s.locked(sess.User.ID); err != nil {
		return err
	}
	var hash string
	if s.db.QueryRow("SELECT hash FROM user_passwords WHERE user_id = ?", sess.User.ID).Scan(&hash) != nil || !checkPassword(hash, pw) {
		s.failed(sess.User.ID)
		return ErrBadPassword
	}
	s.succeeded(sess.User.ID)
	s.markVerified(sess, false)
	return nil
}

// Verified 报告这个会话最近 5 分钟内是否验证过身份（管理登录方式、密码、动态码、Passkey 之前要求）。
// 账号上什么都没设置（比如只用单点登录）时，刚登录 5 分钟内算验证过；开发模式的令牌会话直接放行。
func (s *Service) Verified(sess *Session) bool {
	if sess.Dev {
		return s.dev
	}
	f := s.Factors(sess.User.ID)
	if f.Passkeys == 0 && !f.TOTP && !f.Password && time.Since(sess.CreatedAt) < verifyWindow {
		return true
	}
	return !s.VerifiedUntil(sess).IsZero()
}

// VerifiedUntil 返回这次验证到什么时候失效；没有验证过返回零值。
func (s *Service) VerifiedUntil(sess *Session) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.verified[sess.TokenHash]
	if !ok || time.Since(at) >= verifyWindow {
		return time.Time{}
	}
	return at.Add(verifyWindow)
}

// DeletePasskey 删掉一把 Passkey（调用方先确认验证过身份）。删了会让所有者登录不进来时拒绝。
func (s *Service) DeletePasskey(userID string, id int64, oidc bool) error {
	f := s.Factors(userID)
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM passkeys WHERE rowid = ? AND user_id = ?", id, userID).Scan(&n); err != nil || n == 0 {
		return errors.New("没有这把 Passkey")
	}
	f.Passkeys--
	if !usable(s.Methods(), f, oidc) {
		return ErrLockout
	}
	_, err := s.db.Exec("DELETE FROM passkeys WHERE rowid = ? AND user_id = ?", id, userID)
	return err
}
