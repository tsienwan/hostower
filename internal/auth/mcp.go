package auth

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// MCP 令牌：所有者在网页上签发，给外部 AI 助手（Claude Code、Codex 这类 MCP 客户端）连 /mcp 用。
// 库里只存令牌的 SHA-256 摘要；令牌本体只在签发时返回一次，之后谁也拿不到。
// 令牌只代表“哪个 AI 助手、替哪个所有者发起”，网关据此构造 Kind 为 AI、不能批准的发起人。

// MCPTokenPrefix 是 MCP 令牌的前缀，方便在配置文件和密钥扫描里认出来。
const MCPTokenPrefix = "hto_"

// MCP 令牌的错误。
var (
	ErrMCPTokenName   = errors.New("令牌名称要 1 到 40 个字符，只能用字母、数字、点、横线和下划线")
	ErrMCPTokenExists = errors.New("已经有同名的令牌")
	ErrMCPTokenTTL    = errors.New("有效期要在 1 到 365 天之间，或者选永久")
	ErrMCPTokenLimit  = errors.New("令牌太多了，先吊销不用的")
)

const maxMCPTokens = 20

// MCPToken 是一个 MCP 令牌的元数据（不含令牌本体）。
type MCPToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"userId"`
	UserName  string     `json:"userName"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	LastUsed  *time.Time `json:"lastUsedAt,omitempty"`
	// Role 是签发人现在的角色；网关只接受仍然是所有者的。
	Role string `json:"-"`
}

func validMCPName(name string) bool {
	if n := utf8.RuneCountInString(name); n < 1 || n > 40 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func parseOptTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

// CreateMCPToken 签发一个 MCP 令牌。days 为 0 表示永久。返回令牌本体（只此一次）和元数据。
func (s *Service) CreateMCPToken(userID, name string, days int) (string, *MCPToken, error) {
	name = strings.TrimSpace(name)
	if !validMCPName(name) {
		return "", nil, ErrMCPTokenName
	}
	if days < 0 || days > 365 {
		return "", nil, ErrMCPTokenTTL
	}
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM mcp_tokens").Scan(&n); err != nil {
		return "", nil, err
	}
	if n >= maxMCPTokens {
		return "", nil, ErrMCPTokenLimit
	}
	var dup int
	s.db.QueryRow("SELECT COUNT(*) FROM mcp_tokens WHERE name = ?", name).Scan(&dup)
	if dup > 0 {
		return "", nil, ErrMCPTokenExists
	}
	u, err := s.User(userID)
	if err != nil {
		return "", nil, err
	}
	token := MCPTokenPrefix + randomToken(32)
	now := time.Now().UTC().Truncate(time.Second)
	t := &MCPToken{ID: randomToken(9), UserID: u.ID, UserName: u.Name, Name: name, CreatedAt: now, Role: u.Role}
	exp := ""
	if days > 0 {
		e := now.Add(time.Duration(days) * 24 * time.Hour)
		t.ExpiresAt, exp = &e, e.Format(time.RFC3339)
	}
	if _, err := s.db.Exec("INSERT INTO mcp_tokens (id, user_id, name, token_hash, created_at, expires_at) VALUES (?,?,?,?,?,?)",
		t.ID, t.UserID, t.Name, hashToken(token), now.Format(time.RFC3339), exp); err != nil {
		return "", nil, err
	}
	return token, t, nil
}

// MCPTokens 列出所有 MCP 令牌（新的在前）。已过期的照样列出，方便所有者看见并吊销。
func (s *Service) MCPTokens() ([]MCPToken, error) {
	rows, err := s.db.Query(`SELECT t.id, t.user_id, u.name, u.role, t.name, t.created_at, t.expires_at, t.last_used_at
		FROM mcp_tokens t JOIN users u ON u.id = t.user_id ORDER BY t.created_at DESC, t.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MCPToken{}
	for rows.Next() {
		var t MCPToken
		var created, exp, used string
		if err := rows.Scan(&t.ID, &t.UserID, &t.UserName, &t.Role, &t.Name, &created, &exp, &used); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339, created)
		t.ExpiresAt, t.LastUsed = parseOptTime(exp), parseOptTime(used)
		out = append(out, t)
	}
	return out, rows.Err()
}

// LookupMCPToken 按令牌本体找令牌。不存在、已过期都返回 ErrUnauthorized；顺手记下最后使用时间。
func (s *Service) LookupMCPToken(token string) (*MCPToken, error) {
	if !strings.HasPrefix(token, MCPTokenPrefix) || len(token) > 128 {
		return nil, ErrUnauthorized
	}
	var t MCPToken
	var created, exp, used string
	err := s.db.QueryRow(`SELECT t.id, t.user_id, u.name, u.role, t.name, t.created_at, t.expires_at, t.last_used_at
		FROM mcp_tokens t JOIN users u ON u.id = t.user_id WHERE t.token_hash = ?`, hashToken(token)).
		Scan(&t.ID, &t.UserID, &t.UserName, &t.Role, &t.Name, &created, &exp, &used)
	if err != nil {
		return nil, ErrUnauthorized
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339, created)
	t.ExpiresAt, t.LastUsed = parseOptTime(exp), parseOptTime(used)
	now := time.Now().UTC()
	if t.ExpiresAt != nil && now.After(*t.ExpiresAt) {
		return nil, ErrUnauthorized
	}
	// 最后使用时间只精确到分钟，免得每次调用都写库。
	if t.LastUsed == nil || now.Sub(*t.LastUsed) >= time.Minute {
		s.db.Exec("UPDATE mcp_tokens SET last_used_at = ? WHERE id = ?", now.Format(time.RFC3339), t.ID)
	}
	return &t, nil
}

// RevokeMCPToken 吊销一个 MCP 令牌，返回被吊销的那个。
func (s *Service) RevokeMCPToken(id string) (*MCPToken, error) {
	all, err := s.MCPTokens()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			if _, err := s.db.Exec("DELETE FROM mcp_tokens WHERE id = ?", id); err != nil {
				return nil, err
			}
			return &all[i], nil
		}
	}
	return nil, sql.ErrNoRows
}
