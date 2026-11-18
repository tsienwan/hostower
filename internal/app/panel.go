package app

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// 面板设置：监听地址、访问地址、TLS、会话时长。
// 它们存在数据库里，重启后生效；同名环境变量优先，这样部署方式（容器、systemd）始终说了算，
// 也保证写错了设置时总能用环境变量把塔台救回来。

// PanelKey 是一项面板设置。
type PanelKey struct {
	Key string `json:"key"`
	Env string `json:"env"`
}

// PanelKeys 是全部面板设置。
var PanelKeys = []PanelKey{
	{"listen", "HOSTOWER_LISTEN"},
	{"public_url", "HOSTOWER_PUBLIC_URL"},
	{"tls_mode", "HOSTOWER_TLS"},
	{"tls_cert", "HOSTOWER_TLS_CERT"},
	{"tls_key", "HOSTOWER_TLS_KEY"},
	{"session_minutes", "HOSTOWER_SESSION_MINUTES"},
}

// LoadPanel 读出数据库里保存的面板设置。
func LoadPanel(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM settings WHERE key LIKE 'panel.%'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[strings.TrimPrefix(k, "panel.")] = v
	}
	return out, rows.Err()
}

// WithPanel 把面板设置并进配置：环境变量设过的项不动。
func (c Config) WithPanel(stored map[string]string) Config {
	pick := func(key string, cur string) string {
		if c.FromEnv[key] {
			return cur
		}
		return stored[key]
	}
	if v := pick("listen", c.Listen); v != "" {
		c.Listen = v
	}
	c.TLSMode = pick("tls_mode", c.TLSMode)
	c.TLSCert = pick("tls_cert", c.TLSCert)
	c.TLSKey = pick("tls_key", c.TLSKey)
	if !c.FromEnv["public_url"] {
		c.PublicURL = stored["public_url"]
	}
	if !c.FromEnv["session_minutes"] {
		if n, err := strconv.Atoi(stored["session_minutes"]); err == nil {
			c.SessionMinutes = n
		}
	}
	c.finalize()
	return c
}

func (c *Config) finalize() {
	if c.SessionMinutes <= 0 {
		c.SessionMinutes = 720
	}
	if c.PublicURL == "" {
		_, port, _ := net.SplitHostPort(c.Listen)
		scheme := "http"
		if c.TLSCert != "" {
			scheme = "https"
		}
		c.PublicURL = scheme + "://localhost:" + port
	}
}

// CertInfo 是证书的公开信息。
type CertInfo struct {
	Subject  string   `json:"subject"`
	Names    []string `json:"names"`
	Issuer   string   `json:"issuer"`
	NotAfter string   `json:"notAfter"`
	DaysLeft int      `json:"daysLeft"`
}

// InspectCert 读出证书文件的公开信息。
func InspectCert(path string) (*CertInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("读不到证书文件")
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("证书文件不是 PEM 格式")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.New("证书无法解析")
	}
	names := append([]string{}, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		names = append(names, ip.String())
	}
	return &CertInfo{Subject: cert.Subject.CommonName, Names: names, Issuer: cert.Issuer.CommonName,
		NotAfter: cert.NotAfter.UTC().Format(time.RFC3339), DaysLeft: int(time.Until(cert.NotAfter).Hours() / 24)}, nil
}

// PanelView 是面板设置页要展示的内容。
type PanelView struct {
	Fields []PanelField `json:"fields"`
	// RestartNeeded 表示保存的设置和正在运行的配置不一样，重启后才生效。
	RestartNeeded bool      `json:"restartNeeded"`
	Cert          *CertInfo `json:"cert,omitempty"`
	CertError     string    `json:"certError,omitempty"`
}

// PanelField 是一项设置的三个值：保存的、正在生效的、是否被环境变量接管。
type PanelField struct {
	Key       string `json:"key"`
	Env       string `json:"env"`
	Stored    string `json:"stored"`
	Effective string `json:"effective"`
	FromEnv   bool   `json:"fromEnv"`
}

func (c Config) panelValue(key string) string {
	switch key {
	case "listen":
		return c.Listen
	case "public_url":
		return c.PublicURL
	case "tls_mode":
		return c.TLSMode
	case "tls_cert":
		return c.TLSCert
	case "tls_key":
		return c.TLSKey
	case "session_minutes":
		return strconv.Itoa(c.SessionMinutes)
	}
	return ""
}

// Panel 返回面板设置的当前状态。
func (a *App) Panel() (PanelView, error) {
	stored, err := LoadPanel(a.DB)
	if err != nil {
		return PanelView{}, err
	}
	next := a.EnvConfig.WithPanel(stored)
	var v PanelView
	for _, k := range PanelKeys {
		v.Fields = append(v.Fields, PanelField{Key: k.Key, Env: k.Env, Stored: stored[k.Key], Effective: a.Config.panelValue(k.Key), FromEnv: a.EnvConfig.FromEnv[k.Key]})
		if next.panelValue(k.Key) != a.Config.panelValue(k.Key) && k.Key != "session_minutes" {
			v.RestartNeeded = true
		}
	}
	if a.Config.TLSCert != "" {
		if v.Cert, err = InspectCert(a.Config.TLSCert); err != nil {
			v.CertError = err.Error()
		}
	}
	return v, nil
}

// SavePanel 校验并保存面板设置。会让塔台下次起不来的设置直接拒绝。
func (a *App) SavePanel(values map[string]string) (Config, error) {
	clean := map[string]string{}
	for _, k := range PanelKeys {
		clean[k.Key] = strings.TrimSpace(values[k.Key])
	}
	if a.Config.Dev {
		return Config{}, errors.New("开发模式下不能改面板设置（开发模式只监听本机）")
	}
	if v := clean["listen"]; v != "" {
		host, port, err := net.SplitHostPort(v)
		n, perr := strconv.Atoi(port)
		if err != nil || perr != nil || n < 1 || n > 65535 || (host != "" && net.ParseIP(host) == nil && host != "localhost") {
			return Config{}, errors.New("监听地址应形如 127.0.0.1:18443 或 0.0.0.0:18443")
		}
	}
	if v := clean["session_minutes"]; v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 5 || n > 43200 {
			return Config{}, errors.New("会话时长应在 5 分钟到 30 天之间")
		}
	}
	next := a.EnvConfig.WithPanel(clean)
	if err := next.check(); err != nil {
		return Config{}, fmt.Errorf("这样设置塔台重启后会起不来：%w", err)
	}
	// 被环境变量接管的项也可以先在这里保存：去掉环境变量、重启之后才生效。
	// 所以还要检查“环境变量都去掉之后”的那份配置能不能起来，免得去掉环境变量后进不了面板。
	if bare := a.EnvConfig.withoutPanelEnv(); bare.anyPanelEnv(a.EnvConfig) {
		if err := bare.WithPanel(clean).check(); err != nil {
			return Config{}, fmt.Errorf("去掉环境变量之后塔台会起不来：%w", err)
		}
	}
	tx, err := a.DB.Begin()
	if err != nil {
		return Config{}, err
	}
	defer tx.Rollback()
	for k, v := range clean {
		if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, "panel."+k, v); err != nil {
			return Config{}, err
		}
	}
	return next, tx.Commit()
}

// check 检查一份配置能不能正常启动（含证书和私钥能不能读出来）。
func (c Config) check() error {
	if err := c.SelfCheck(); err != nil {
		return errors.New(strings.TrimPrefix(err.Error(), "拒绝启动："))
	}
	if c.TLSCert != "" {
		if _, err := tls.LoadX509KeyPair(c.TLSCert, c.TLSKey); err != nil {
			return errors.New("证书和私钥读不出来或不匹配：请确认两个文件的路径在塔台所在的机器（容器）里存在")
		}
	}
	return nil
}

// withoutPanelEnv 返回“面板设置相关的环境变量都没设”时的配置：用来检查去掉环境变量之后能不能起来。
func (c Config) withoutPanelEnv() Config {
	b := c
	b.FromEnv = map[string]bool{}
	if c.FromEnv["listen"] {
		b.Listen = "127.0.0.1:18443"
	}
	if c.FromEnv["public_url"] {
		b.PublicURL = ""
	}
	if c.FromEnv["tls_mode"] {
		b.TLSMode = ""
	}
	if c.FromEnv["tls_cert"] {
		b.TLSCert = ""
	}
	if c.FromEnv["tls_key"] {
		b.TLSKey = ""
	}
	if c.FromEnv["session_minutes"] {
		b.SessionMinutes = 0
	}
	return b
}

func (Config) anyPanelEnv(orig Config) bool {
	for _, v := range orig.FromEnv {
		if v {
			return true
		}
	}
	return false
}
