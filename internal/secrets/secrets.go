// Package secrets 是内置的凭据提供方：信封加密，主密钥不进数据库。
// 资源里只出现 ref:<名称>；凭据本体只在执行时取出，用完即弃。
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// ErrNoMasterKey 表示没有配置主密钥，凭据功能不可用。
var ErrNoMasterKey = errors.New("没有配置主密钥（HOSTOWER_MASTER_KEY_FILE），无法读写凭据")

// ErrNotFound 表示凭据不存在。
var ErrNotFound = errors.New("凭据不存在")

// Store 是加密的凭据存储。
type Store struct {
	db     *sql.DB
	master []byte
	// autoPath 不为空表示主密钥是自动生成、放在数据目录里的（页面上据此提示分开保管）。
	autoPath string
}

// SetAutoPath 记下自动生成的主密钥文件的位置；显式配置的主密钥不调用它。只在启动时调用。
func (s *Store) SetAutoPath(p string) { s.autoPath = p }

// AutoKey 报告主密钥是不是自动生成、放在数据目录里的。
func (s *Store) AutoKey() bool { return s.autoPath != "" }

// LoadMasterKey 从环境变量指向的文件或环境变量本身读取主密钥（base64 编码的 32 字节）。
// 没有配置时返回 nil。
func LoadMasterKey() ([]byte, error) {
	var raw string
	if p := os.Getenv("HOSTOWER_MASTER_KEY_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("读取主密钥文件：%w", err)
		}
		raw = string(b)
	} else if v := os.Getenv("HOSTOWER_MASTER_KEY"); v != "" {
		raw = v
	} else {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(key) != 32 {
		return nil, errors.New("主密钥必须是 base64 编码的 32 字节（用 hostower keygen 生成）")
	}
	return key, nil
}

// GenerateMasterKey 生成一把新的主密钥。
func GenerateMasterKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// New 创建凭据存储。master 为 nil 时只能列出名称。
func New(db *sql.DB, master []byte) *Store {
	return &Store{db: db, master: master}
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9][a-z0-9._-]*)+$`)

// RefName 把 "ref:ssh/ops" 解析成存储里的名称 "ssh/ops"。
func RefName(ref string) (string, error) {
	name, ok := strings.CutPrefix(ref, "ref:")
	if !ok || !nameRe.MatchString(name) {
		return "", fmt.Errorf("凭据引用 %q 格式应为 ref:<提供方>/<名称>", ref)
	}
	if Reserved(name) {
		return "", fmt.Errorf("凭据 %s 是系统自己用的，不能当主机凭据", name)
	}
	return name, nil
}

// reservedPrefixes 是系统自己用的凭据名前缀：登录用的动态码密钥、单点登录的客户端密钥、通知渠道的密钥。
// 用户起名的凭据不能用它们（否则能覆盖掉），主机凭据也不能引用它们。
var reservedPrefixes = []string{"auth/", "oidc/", "notify/"}

// Reserved 报告这个凭据名是不是系统自己用的。
func Reserved(name string) bool {
	for _, p := range reservedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func seal(key, plaintext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, aad), nil
}

func open(key, sealed, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("密文损坏")
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], aad)
}

// Put 写入或覆盖一条凭据。
func (s *Store) Put(name string, plaintext []byte) error {
	if s.master == nil {
		return ErrNoMasterKey
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("凭据名称 %q 应形如 ssh/ops", name)
	}
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return err
	}
	ct, err := seal(dek, plaintext, []byte(name))
	if err != nil {
		return err
	}
	wrapped, err := seal(s.master, dek, []byte(name))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO secrets (name, wrapped_key, ciphertext, updated_at) VALUES (?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET wrapped_key=excluded.wrapped_key, ciphertext=excluded.ciphertext, updated_at=excluded.updated_at`,
		name, wrapped, ct, time.Now().UTC().Format(time.RFC3339))
	return err
}

// Get 取出凭据本体。调用方用完即弃，不要缓存、不要写日志。
func (s *Store) Get(name string) ([]byte, error) {
	if s.master == nil {
		return nil, ErrNoMasterKey
	}
	var wrapped, ct []byte
	err := s.db.QueryRow("SELECT wrapped_key, ciphertext FROM secrets WHERE name = ?", name).Scan(&wrapped, &ct)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w：%s", ErrNotFound, name)
	}
	if err != nil {
		return nil, err
	}
	dek, err := open(s.master, wrapped, []byte(name))
	if err != nil {
		return nil, errors.New("无法解开凭据：主密钥不匹配")
	}
	return open(dek, ct, []byte(name))
}

// Delete 删除一条凭据。
func (s *Store) Delete(name string) error {
	_, err := s.db.Exec("DELETE FROM secrets WHERE name = ?", name)
	return err
}

// Meta 是凭据的元数据；永远不包含凭据本体。
type Meta struct {
	Name      string `json:"name"`
	UpdatedAt string `json:"updatedAt"`
}

// List 列出凭据名称。
func (s *Store) List() ([]Meta, error) {
	rows, err := s.db.Query("SELECT name, updated_at FROM secrets ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Meta{}
	for rows.Next() {
		var m Meta
		if err := rows.Scan(&m.Name, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Available 报告是否配置了主密钥。
func (s *Store) Available() bool { return s.master != nil }
