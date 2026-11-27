// Package app 把各个部件装配起来：存储、凭据、判级引擎、审计、执行器、内核。
package app

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/notify"
	"github.com/tsienwan/hostower/internal/secrets"
	"github.com/tsienwan/hostower/internal/snippet"
	"github.com/tsienwan/hostower/internal/store"
)

// Config 是进程级配置，全部来自环境变量和命令行。
type Config struct {
	DataDir   string
	ConfigDir string
	Listen    string
	PublicURL string
	TLSCert   string
	TLSKey    string
	TLSMode   string // "" | proxy
	Dev       bool
	// LocalMode 控制内置的“本机”主机（HOSTOWER_LOCAL）：
	// 空 = 能用就开；"0" = 关掉；"host" = 容器部署，进入宿主机执行（需要特权容器）。
	LocalMode string
	// SessionMinutes 是登录会话的时长。
	SessionMinutes int
	// FromEnv 记录哪些项是环境变量设的：它们优先于面板设置。
	FromEnv map[string]bool
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ConfigFromEnv 读取配置。
func ConfigFromEnv(dev bool) Config {
	c := Config{
		DataDir:   env("HOSTOWER_DATA", "./data"),
		ConfigDir: os.Getenv("HOSTOWER_CONFIG"),
		Listen:    env("HOSTOWER_LISTEN", "127.0.0.1:8693"),
		PublicURL: os.Getenv("HOSTOWER_PUBLIC_URL"),
		TLSCert:   os.Getenv("HOSTOWER_TLS_CERT"),
		TLSKey:    os.Getenv("HOSTOWER_TLS_KEY"),
		TLSMode:   os.Getenv("HOSTOWER_TLS"),
		Dev:       dev,
		LocalMode: os.Getenv("HOSTOWER_LOCAL"),
	}
	if c.ConfigDir == "" {
		c.ConfigDir = "./config"
		// 开发模式下没有自己的配置时，用仓库里的示例（全是模拟主机和保留地址）。
		if _, err := os.Stat(c.ConfigDir); err != nil && dev {
			c.ConfigDir = "./examples/config"
		}
	}
	c.FromEnv = map[string]bool{}
	for _, k := range PanelKeys {
		c.FromEnv[k.Key] = os.Getenv(k.Env) != ""
	}
	c.SessionMinutes, _ = strconv.Atoi(os.Getenv("HOSTOWER_SESSION_MINUTES"))
	c.finalize()
	return c
}

func isLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// SelfCheck 是启动自检。默认按公网设防：不满足条件就拒绝启动。
func (c Config) SelfCheck() error {
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("HOSTOWER_PUBLIC_URL %q 无效", c.PublicURL)
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("HOSTOWER_TLS_CERT 和 HOSTOWER_TLS_KEY 必须同时设置")
	}
	if c.TLSMode != "" && c.TLSMode != "proxy" {
		return fmt.Errorf("HOSTOWER_TLS 只能是 proxy，得到 %q", c.TLSMode)
	}
	loop := isLoopback(c.Listen)
	localURL := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if c.Dev {
		if !loop {
			return errors.New("拒绝启动：开发模式只能监听本机回环地址（当前 " + c.Listen + "）")
		}
		if !localURL {
			return errors.New("拒绝启动：开发模式的 HOSTOWER_PUBLIC_URL 必须是 localhost")
		}
		return nil
	}
	if loop {
		return nil
	}
	// 监听在非回环地址：必须有 TLS。要么塔台自己终结，要么明确声明前面有代理终结。
	switch {
	case c.TLSCert != "":
		if u.Scheme != "https" {
			return errors.New("拒绝启动：配置了证书，HOSTOWER_PUBLIC_URL 应该是 https://")
		}
	case c.TLSMode == "proxy":
		if u.Scheme != "https" && !localURL {
			return errors.New("拒绝启动：HOSTOWER_TLS=proxy 时，HOSTOWER_PUBLIC_URL 必须是 https://（Passkey 也要求安全来源）")
		}
	default:
		return errors.New("拒绝启动：监听在非回环地址 " + c.Listen + " 但没有 TLS。" +
			"设置 HOSTOWER_TLS_CERT/HOSTOWER_TLS_KEY 让塔台自己终结 TLS，" +
			"或在前面有终结 TLS 的反向代理时设置 HOSTOWER_TLS=proxy；只在本机用就监听 127.0.0.1")
	}
	return nil
}

// App 是装配好的塔台。
type App struct {
	// Config 是正在生效的配置（环境变量 + 面板设置）；EnvConfig 只含环境变量。
	EnvConfig Config
	Started   time.Time
	Config    Config
	DB        *sql.DB
	Kernel    *kernel.Kernel
	Secrets   *secrets.Store
	sink      *audit.FileSink
	snips     atomic.Pointer[map[string]*snippet.Snippet]
	log       *slog.Logger
	// local 是本机执行器；这台机器上用不了或被关掉时是 nil。
	local *executor.Local
	// Notify 是通知中心：它是审计的一个外部副本接收端，也听内核的事件。
	Notify *notify.Hub
}

// LoadPacks 加载配置目录下 rules/ 里的规则包。
func LoadPacks(configDir string) ([]risk.PackRule, error) {
	files, _ := filepath.Glob(filepath.Join(configDir, "rules", "*.yaml"))
	sort.Strings(files)
	var out []risk.PackRule
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		rules, err := risk.ParsePack(data)
		if err != nil {
			return nil, fmt.Errorf("%s：%w", filepath.Base(f), err)
		}
		out = append(out, rules...)
	}
	return out, nil
}

// Open 装配塔台。
func Open(cfg Config, log *slog.Logger) (*App, error) {
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("打开数据目录 %s：%w", cfg.DataDir, err)
	}
	a := &App{Config: cfg, EnvConfig: cfg, Started: time.Now(), DB: db, log: log}
	// 面板设置存在数据库里；写错了可以用 HOSTOWER_IGNORE_PANEL=1 跳过它启动。
	if os.Getenv("HOSTOWER_IGNORE_PANEL") != "1" && !cfg.Dev {
		if stored, err := LoadPanel(db); err == nil {
			a.Config = cfg.WithPanel(stored)
		}
	}
	fail := func(err error) (*App, error) {
		a.Close()
		return nil, err
	}
	master, err := secrets.LoadMasterKey()
	if err != nil {
		return fail(err)
	}
	autoKey := ""
	if master == nil {
		// 没有显式配置主密钥时，用数据目录里自动生成的那把，做到开箱即用。
		// 它和数据库放在同一个目录：只拿到数据库文件的人仍然解不开凭据，但拿到整个数据目录（或它的备份）的人可以。
		// 想把两者分开保管，就把这个文件挪出去，用 HOSTOWER_MASTER_KEY_FILE 指向它。
		autoKey = filepath.Join(cfg.DataDir, "master_key")
		if master, err = autoMasterKey(autoKey); err != nil {
			return fail(err)
		}
		log.Warn("使用数据目录里自动生成的主密钥；备份数据目录的人也能解开凭据。要分开保管，把它移出数据目录并用 HOSTOWER_MASTER_KEY_FILE 指向它", "path", autoKey)
	}
	a.Secrets = secrets.New(db, master)
	a.Secrets.SetAutoPath(autoKey)

	packs, err := LoadPacks(cfg.ConfigDir)
	if err != nil {
		return fail(err)
	}
	switch cfg.LocalMode {
	case "0":
	case "host":
		// 明确要求了宿主机模式却进不去，就不要带着一台不能用的本机启动。
		if a.local, err = executor.NewHostLocal(); err != nil {
			return fail(err)
		}
		log.Warn("宿主机模式：本机主机上的命令以 root 身份在宿主机上执行")
	case "":
		a.local = executor.NewLocal()
	default:
		return fail(fmt.Errorf("HOSTOWER_LOCAL 只能是 0 或 host，现在是 %q", cfg.LocalMode))
	}
	inv, err := a.loadInventory()
	if err != nil {
		return fail(err)
	}
	snips, err := a.loadSnippets()
	if err != nil {
		return fail(err)
	}
	a.snips.Store(&snips)

	a.sink, err = audit.NewFileSink(filepath.Join(cfg.DataDir, "audit.jsonl"))
	if err != nil {
		return fail(err)
	}
	// 通知中心也挂在审计的外部副本上：安全相关的记录（登录失败、暂停执行等）由它转成通知。它只往队列里放，不会拖住审计。
	a.Notify, err = notify.New(db, a.Secrets, log, func() string { return a.Config.PublicURL })
	if err != nil {
		return fail(err)
	}
	alog := audit.Open(db, func(err error) { log.Error("审计外部副本写入失败", "err", err) }, a.sink, a.Notify)

	execs := executor.Registry{"ssh": executor.NewSSH(a.Secrets, db)}
	if a.local != nil {
		// 本机执行器：命令以运行 Hostower 的账号执行，同样只能经内核到达。
		execs["local"] = a.local
	}
	if cfg.Dev {
		// 模拟执行器只在开发模式注册：生产环境里写 executor: sim 的主机无法执行。
		execs["sim"] = executor.NewSim()
	}
	a.Kernel, err = kernel.New(kernel.Config{DataDir: cfg.DataDir, Secret: a.Secrets.Get}, db, risk.New(packs...), alog, execs, inv)
	if err != nil {
		return fail(err)
	}
	return a, nil
}

// Snippets 返回当前加载的 Snippet。
func (a *App) Snippets() map[string]*snippet.Snippet { return *a.snips.Load() }

// Reload 重新加载主机清单和 Snippet。规则包只在启动时加载：判级规则不热更新。
func (a *App) Reload() error {
	inv, err := a.loadInventory()
	if err != nil {
		return err
	}
	snips, err := a.loadSnippets()
	if err != nil {
		return err
	}
	a.Kernel.SetInventory(inv)
	a.snips.Store(&snips)
	return nil
}

// autoMasterKey 读出数据目录里自动生成的主密钥；第一次启动时生成它。
// 文件存在但内容不对时直接报错，不重新生成：重新生成会让已经保存的凭据全部解不开。
func autoMasterKey(p string) ([]byte, error) {
	raw, err := os.ReadFile(p)
	if err == nil {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("主密钥文件 %s 已损坏（应该是 base64 编码的 32 字节）；不要删除它，否则已保存的凭据都解不开", p)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取主密钥文件：%w", err)
	}
	text := secrets.GenerateMasterKey()
	// O_EXCL：两个进程同时第一次启动时，不会一个覆盖另一个刚写下的密钥。
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("无法生成主密钥文件：%w", err)
	}
	if _, err := f.WriteString(text + "\n"); err != nil {
		f.Close()
		return nil, fmt.Errorf("无法生成主密钥文件：%w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("无法生成主密钥文件：%w", err)
	}
	return base64.StdEncoding.DecodeString(text)
}

// loadSnippets 读配置目录里的 Snippet，再并上页面管理的。名字冲突时文件优先。
func (a *App) loadSnippets() (map[string]*snippet.Snippet, error) {
	out, err := snippet.LoadDir(filepath.Join(a.EnvConfig.ConfigDir, "snippets"))
	if err != nil {
		return nil, err
	}
	for _, s := range out {
		s.Source = "file"
	}
	rows, err := a.DB.Query("SELECT body FROM snippets ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		s, err := snippet.Parse([]byte(body))
		if err != nil {
			a.log.Warn("跳过一条无法解析的 Snippet 记录", "err", err)
			continue
		}
		if _, clash := out[s.Name]; clash {
			continue
		}
		s.Source, s.YAML = "managed", body
		out[s.Name] = s
	}
	return out, rows.Err()
}

// SaveSnippet 新增或修改一个页面管理的 Snippet。
func (a *App) SaveSnippet(spec snippet.Snippet, create bool) (*snippet.Snippet, error) {
	s, data, err := snippet.FromSpec(spec)
	if err != nil {
		return nil, err
	}
	cur, exists := a.Snippets()[s.Name]
	switch {
	case exists && cur.Source == "file":
		return nil, fmt.Errorf("Snippet %s 来自配置文件，请在文件里修改（或另存为一个新名字）", s.Name)
	case exists && create:
		return nil, fmt.Errorf("Snippet %s 已存在", s.Name)
	case !exists && !create:
		return nil, fmt.Errorf("Snippet %s 不存在", s.Name)
	}
	if _, err := a.DB.Exec(`INSERT INTO snippets (name, body, updated_at) VALUES (?,?,?)
		ON CONFLICT(name) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at`,
		s.Name, string(data), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return s, a.Reload()
}

// DeleteSnippet 删除一个页面管理的 Snippet。
func (a *App) DeleteSnippet(name string) error {
	cur, exists := a.Snippets()[name]
	if !exists {
		return fmt.Errorf("Snippet %s 不存在", name)
	}
	if cur.Source == "file" {
		return fmt.Errorf("Snippet %s 来自配置文件，请在文件里删除", name)
	}
	if _, err := a.DB.Exec("DELETE FROM snippets WHERE name = ?", name); err != nil {
		return err
	}
	return a.Reload()
}

// loadInventory 读配置文件里的清单，再并上页面管理的主机。
func (a *App) loadInventory() (*inventory.Inventory, error) {
	inv, err := inventory.Load(filepath.Join(a.Config.ConfigDir, "inventory.yaml"))
	if err != nil {
		return nil, err
	}
	rows, err := a.DB.Query("SELECT body FROM hosts ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var managed []inventory.Host
	var local *inventory.Host
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var h inventory.Host
		if json.Unmarshal([]byte(body), &h) != nil || inventory.Normalize(&h) != nil {
			a.log.Warn("跳过一条无法解析的主机记录")
			continue
		}
		if a.local != nil && h.Name == inventory.LocalName && (h.Executor == "local" || (h.Executor == "ssh" && inventory.Loopback(h.Addr))) {
			// 这一条不是另一台主机，而是页面上对内置本机的修改（包括把它改成经 SSH 连回本机）。
			local = &h
			continue
		}
		managed = append(managed, h)
	}
	if a.local != nil {
		// 内置的本机排在最后：配置文件或页面上有同名主机时让给它们。
		b := inventory.Builtin()
		if local != nil {
			b.Tags, b.Sensitivity, b.Vars, b.Manual = local.Tags, local.Sensitivity, local.Vars, local.Manual
			if local.Executor == "ssh" {
				b.Addr, b.Executor, b.Credential = local.Addr, "ssh", local.Credential
			}
		}
		managed = append(managed, b)
	}
	return inv.WithManaged(managed), rows.Err()
}

// Executors 返回当前可用的执行器名字。
func (a *App) Executors() []string {
	if a.Config.Dev {
		return []string{"ssh", "sim"}
	}
	return []string{"ssh"}
}

// SaveHost 新增或修改一台页面管理的主机。配置文件里的主机不能在页面上改。
func (a *App) SaveHost(h inventory.Host, create bool) error {
	cur, exists := a.Kernel.Inventory().Host(h.Name)
	builtin := exists && cur.Source == "builtin"
	if builtin {
		// 内置的本机：标签、敏感度可以改；连接方式要么是本机直连，要么是经 SSH 连回本机
		// （直连的进程提不了权时，用一个能 sudo 的账号来管防火墙和站点）。地址只能是回环地址：它始终是这台机器。
		if h.Executor == "ssh" {
			if a.Config.LocalMode == "host" {
				return errors.New("宿主机模式下本机上的命令已经以 root 身份执行，不需要改成 SSH")
			}
			if !inventory.Loopback(h.Addr) {
				return errors.New("本机改成 SSH 连接时，地址只能是 127.0.0.1 或 localhost（可以带端口）")
			}
		} else {
			b := inventory.Builtin()
			h.Addr, h.Executor, h.Credential = b.Addr, b.Executor, ""
		}
	}
	if err := inventory.Normalize(&h); err != nil {
		return err
	}
	ok := builtin && h.Executor == "local"
	for _, e := range a.Executors() {
		ok = ok || e == h.Executor
	}
	if !ok {
		return fmt.Errorf("执行器 %q 不可用", h.Executor)
	}
	if h.Executor == "ssh" {
		if _, err := secrets.RefName(h.Credential); err != nil {
			return errors.New("SSH 主机需要选择一条凭据")
		}
	}
	switch {
	case exists && cur.Source == "file":
		return fmt.Errorf("主机 %s 来自配置文件，请在文件里修改", h.Name)
	case exists && create:
		return fmt.Errorf("主机 %s 已存在", h.Name)
	case !exists && !create:
		return fmt.Errorf("主机 %s 不存在", h.Name)
	}
	h.Source, h.Manual = "", ""
	body, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if _, err := a.DB.Exec(`INSERT INTO hosts (name, body, updated_at) VALUES (?,?,?)
		ON CONFLICT(name) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at`,
		h.Name, string(body), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return a.Reload()
}

// DeleteHost 删除一台页面管理的主机。
func (a *App) DeleteHost(name string) error {
	cur, exists := a.Kernel.Inventory().Host(name)
	if !exists {
		return fmt.Errorf("主机 %s 不存在", name)
	}
	if cur.Source == "file" {
		return fmt.Errorf("主机 %s 来自配置文件，请在文件里删除", name)
	}
	if cur.Source == "builtin" {
		return fmt.Errorf("主机 %s 是内置的本机，不能删除；设置 HOSTOWER_LOCAL=0 可以关掉它", name)
	}
	if _, err := a.DB.Exec("DELETE FROM hosts WHERE name = ?", name); err != nil {
		return err
	}
	return a.Reload()
}

// Close 关闭数据库和审计副本。
func (a *App) Close() {
	if a.sink != nil {
		a.sink.Close()
	}
	if a.DB != nil {
		a.DB.Close()
	}
}
