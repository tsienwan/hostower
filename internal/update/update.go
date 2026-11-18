// Package update 是 Hostower 的自我更新：从发布页取新版本的可执行文件，验签后换上去。
//
// 它不经过执行器，也不碰任何主机：换的是 Hostower 进程自己。所以它和内核的“唯一一条路”无关，
// 镜像、容器的启动参数、挂载都不用动——新版本的可执行文件放在数据目录里，启动时由镜像里的那份验签后接力过去。
//
// 信任的根是编译进可执行文件的发布公钥：没有它，或签名对不上，就什么都不换。
// GitHub 账号或下载通道被攻破，也发不出一份能通过验签的更新。
package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	settingAuto   = "update.auto"
	defaultAPI    = "https://api.github.com"
	maxBinarySize = 256 << 20
	checkEvery    = 6 * time.Hour
	// HandoffEnv 标记“这个进程是从数据目录里的新版本启动的”，避免接力时套娃。
	HandoffEnv = "HOSTOWER_HANDOFF"
)

// Config 是更新器需要的全部东西。Repo 和 PubKey 在发布构建时编译进来。
type Config struct {
	Version string
	Repo    string // owner/name
	PubKey  string // base64 的 ed25519 公钥
	API     string // 发布信息的来源；空表示 GitHub
	DataDir string
	DB      *sql.DB
	Log     *slog.Logger
	// Audit 记一条审计。
	Audit func(typ, message string, detail map[string]any)
	// Restart 在新版本就位后调用：优雅停下服务，再切换到 path。
	Restart func(path string)
}

// Status 是给设置页看的状态。
type Status struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
	Current   string `json:"current"`
	Auto      bool   `json:"auto"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	Newer     bool   `json:"newer"`
	CheckedAt int64  `json:"checkedAt,omitempty"`
	// State：idle | checking | downloading | restarting
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

// Updater 检查并应用更新。
type Updater struct {
	cfg    Config
	key    ed25519.PublicKey
	reason string

	mu     sync.Mutex
	st     Status
	asset  string // 最新版本里本平台的可执行文件
	sig    string
	busy   bool
	client *http.Client
}

// AssetName 是发布页上本平台可执行文件的名字。
func AssetName() string { return "hostower_" + runtime.GOOS + "_" + runtime.GOARCH }

// New 创建更新器。这个构建不支持更新时仍然返回一个可用的对象，只是 Status 里说明原因。
func New(cfg Config) *Updater {
	if cfg.API == "" {
		cfg.API = defaultAPI
	}
	u := &Updater{cfg: cfg, client: &http.Client{Timeout: 10 * time.Minute}}
	key, err := ParseKey(cfg.PubKey)
	switch {
	case runtime.GOOS != "linux":
		u.reason = "自动更新只支持 Linux"
	case cfg.Repo == "" || err != nil:
		u.reason = "这个构建没有配置更新来源（自己编译的版本需要自己更新）"
	case !validVersion(cfg.Version):
		u.reason = "开发版本不参与自动更新"
	case !allowedAPI(cfg.API):
		u.reason = "更新来源必须是 https 地址"
	default:
		u.key = key
	}
	u.st = Status{Supported: u.reason == "", Reason: u.reason, Current: cfg.Version, State: "idle"}
	if cfg.DB != nil {
		var v string
		if cfg.DB.QueryRow("SELECT value FROM settings WHERE key = ?", settingAuto).Scan(&v) == nil {
			u.st.Auto = v == "1"
		}
	}
	return u
}

// allowedAPI：发布信息只能走 https；明文只允许指向本机（测试用）。
func allowedAPI(raw string) bool {
	p, err := url.Parse(raw)
	if err != nil {
		return false
	}
	h := p.Hostname()
	return p.Scheme == "https" || (p.Scheme == "http" && (h == "127.0.0.1" || h == "localhost" || h == "::1"))
}

// ParseKey 解析 base64 的 ed25519 公钥。
func ParseKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("发布公钥不合法")
	}
	return ed25519.PublicKey(b), nil
}

// Status 返回当前状态。
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

// SetAuto 打开或关闭自动更新。
func (u *Updater) SetAuto(on bool) error {
	if on && u.reason != "" {
		return errors.New(u.reason)
	}
	v := "0"
	if on {
		v = "1"
	}
	if _, err := u.cfg.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, settingAuto, v); err != nil {
		return err
	}
	u.mu.Lock()
	u.st.Auto = on
	u.mu.Unlock()
	return nil
}

func (u *Updater) set(f func(*Status)) {
	u.mu.Lock()
	f(&u.st)
	u.mu.Unlock()
}

// begin 保证同一时间只有一个检查或更新在跑。
func (u *Updater) begin(state string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.reason != "" {
		return errors.New(u.reason)
	}
	if u.busy {
		return errors.New("正在检查或更新，请稍等")
	}
	u.busy, u.st.State, u.st.Error = true, state, ""
	return nil
}

func (u *Updater) end(err error) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.busy = false
	if u.st.State != "restarting" {
		u.st.State = "idle"
	}
	if err != nil {
		u.st.Error = err.Error()
	}
	return err
}

// Check 去发布页看最新版本。
func (u *Updater) Check(ctx context.Context) error {
	if err := u.begin("checking"); err != nil {
		return err
	}
	return u.end(u.check(ctx))
}

func (u *Updater) check(ctx context.Context) error {
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	body, err := u.get(ctx, strings.TrimRight(u.cfg.API, "/")+"/repos/"+u.cfg.Repo+"/releases/latest", 1<<20)
	if err != nil {
		return fmt.Errorf("读取发布信息失败：%w", err)
	}
	if json.Unmarshal(body, &rel) != nil || !validVersion(rel.TagName) {
		return errors.New("发布信息格式不对")
	}
	asset, sig := "", ""
	for _, a := range rel.Assets {
		switch a.Name {
		case AssetName():
			asset = a.URL
		case AssetName() + ".sig":
			sig = a.URL
		}
	}
	u.mu.Lock()
	u.st.Latest, u.st.URL, u.st.CheckedAt = rel.TagName, rel.HTMLURL, time.Now().UnixMilli()
	u.st.Newer = newer(rel.TagName, u.cfg.Version)
	u.asset, u.sig = asset, sig
	u.mu.Unlock()
	return nil
}

func (u *Updater) get(ctx context.Context, link string, limit int64) ([]byte, error) {
	if !allowedAPI(link) {
		return nil, errors.New("下载地址必须是 https")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "hostower-updater")
	req.Header.Set("Accept", "application/vnd.github+json, application/octet-stream")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("服务器返回 %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("文件太大")
	}
	return b, nil
}

// Apply 下载最新版本、验签、放进数据目录，然后请求重启切换过去。
func (u *Updater) Apply(ctx context.Context) error {
	if err := u.begin("downloading"); err != nil {
		return err
	}
	return u.end(u.apply(ctx))
}

func (u *Updater) apply(ctx context.Context) error {
	if err := u.check(ctx); err != nil {
		return err
	}
	u.mu.Lock()
	latest, asset, sig, isNewer := u.st.Latest, u.asset, u.sig, u.st.Newer
	u.mu.Unlock()
	if !isNewer {
		return nil
	}
	if asset == "" || sig == "" {
		return fmt.Errorf("%s 的发布里没有 %s 和它的签名", latest, AssetName())
	}
	bin, err := u.get(ctx, asset, maxBinarySize)
	if err != nil {
		return fmt.Errorf("下载新版本失败：%w", err)
	}
	sigText, err := u.get(ctx, sig, 4096)
	if err != nil {
		return fmt.Errorf("下载签名失败：%w", err)
	}
	if err := Verify(u.key, bin, sigText); err != nil {
		u.audit("system.update.rejected", "更新被拒绝：签名验证失败", map[string]any{"version": latest})
		return err
	}
	path, err := stage(u.cfg.DataDir, bin, sigText)
	if err != nil {
		return err
	}
	// 签名只证明“这是我们发布过的文件”，还要确认它就是页面上说的那个版本，防止拿旧版本冒充新版本。
	got, err := binaryVersion(path)
	if err != nil || got != latest || !newer(got, u.cfg.Version) {
		Discard(u.cfg.DataDir)
		if err != nil {
			return fmt.Errorf("新版本无法运行（数据目录是否禁止执行？）：%w", err)
		}
		return fmt.Errorf("下载到的是 %s，不是发布页上的 %s", got, latest)
	}
	u.audit("system.update.applied", "更新到 "+latest, map[string]any{"from": u.cfg.Version, "to": latest, "sha256": sum(bin)})
	u.set(func(s *Status) { s.State = "restarting" })
	if u.cfg.Restart != nil {
		u.cfg.Restart(path)
	}
	return nil
}

func (u *Updater) audit(typ, msg string, detail map[string]any) {
	if u.cfg.Audit != nil {
		u.cfg.Audit(typ, msg, detail)
	}
}

// Run 定期检查；打开了自动更新就直接应用。
func (u *Updater) Run(ctx context.Context) {
	if u.reason != "" {
		return
	}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		var err error
		if u.Status().Auto {
			err = u.Apply(ctx)
		} else {
			err = u.Check(ctx)
		}
		if err != nil && u.cfg.Log != nil && ctx.Err() == nil {
			u.cfg.Log.Warn("检查更新失败", "err", err)
		}
		timer.Reset(checkEvery)
	}
}

// ---------- 签名 ----------

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// signedMessage 是被签名的内容：带用途前缀的文件摘要。
func signedMessage(bin []byte) []byte { return []byte("hostower-release-v1\n" + sum(bin)) }

// Sign 给一个发布文件签名，返回 base64 文本。
func Sign(priv ed25519.PrivateKey, bin []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, signedMessage(bin)))
}

// Verify 检查签名。
func Verify(key ed25519.PublicKey, bin, sigText []byte) error {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigText)))
	if err != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, signedMessage(bin), sig) {
		return errors.New("签名验证失败：这不是用发布私钥签过的文件")
	}
	return nil
}

// ---------- 版本号 ----------

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 || !strings.HasPrefix(v, "v") {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || strconv.Itoa(n) != p {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// validVersion：只认 v1.2.3 这种正式版本；带后缀的预发布和开发版本都不算。
func validVersion(v string) bool { _, ok := parseVersion(v); return ok }

// newer 报告 a 是否比 b 新。
func newer(a, b string) bool {
	x, ok1 := parseVersion(a)
	y, ok2 := parseVersion(b)
	if !ok1 || !ok2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

// ---------- 数据目录里的新版本 ----------

func dir(dataDir string) string         { return filepath.Join(dataDir, "update") }
func stagedPath(dataDir string) string  { return filepath.Join(dir(dataDir), "hostower") }
func sigPath(dataDir string) string     { return filepath.Join(dir(dataDir), "hostower.sig") }
func pendingPath(dataDir string) string { return filepath.Join(dir(dataDir), "pending") }

// stage 把验过签的新版本写进数据目录（先写临时文件再改名）。
func stage(dataDir string, bin, sig []byte) (string, error) {
	if err := os.MkdirAll(dir(dataDir), 0o700); err != nil {
		return "", err
	}
	tmp := stagedPath(dataDir) + ".tmp"
	if err := os.WriteFile(tmp, bin, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(sigPath(dataDir), sig, 0o600); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, stagedPath(dataDir)); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return stagedPath(dataDir), nil
}

// Discard 丢掉数据目录里的新版本。
func Discard(dataDir string) {
	os.Remove(stagedPath(dataDir))
	os.Remove(sigPath(dataDir))
	os.Remove(pendingPath(dataDir))
}

// binaryVersion 运行“<文件> version”取它的版本号。只对验过签的文件调用。
func binaryVersion(path string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return "", err
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return "", errors.New("版本输出格式不对")
	}
	return f[1], nil
}

// MarkPending 记下“正要切换到新版本”。新版本启动成功后会清掉它；
// 如果下次从镜像里的版本启动时它还在，说明新版本没起来，就放弃这次更新。
func MarkPending(dataDir string) error {
	return os.WriteFile(pendingPath(dataDir), []byte(time.Now().UTC().Format(time.RFC3339)), 0o600)
}

// MarkHealthy 在服务成功启动后调用。
func MarkHealthy(dataDir string) { os.Remove(pendingPath(dataDir)) }

// Staged 检查数据目录里有没有一份可以接力的新版本：签名对、版本比当前新、上次切换没有失败。
// 不合格的直接删掉。返回它的路径和版本。
func Staged(dataDir, current, pubKey string) (path, version string, ok bool) {
	p := stagedPath(dataDir)
	bin, err := os.ReadFile(p)
	if err != nil {
		return "", "", false
	}
	key, kerr := ParseKey(pubKey)
	sig, serr := os.ReadFile(sigPath(dataDir))
	if kerr != nil || serr != nil || Verify(key, bin, sig) != nil {
		Discard(dataDir)
		return "", "", false
	}
	if _, err := os.Stat(pendingPath(dataDir)); err == nil {
		// 上一次切换过去之后没有成功启动。
		Discard(dataDir)
		return "", "", false
	}
	v, err := binaryVersion(p)
	if err != nil || !newer(v, current) {
		Discard(dataDir)
		return "", "", false
	}
	return p, v, true
}
