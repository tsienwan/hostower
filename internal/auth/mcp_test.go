package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMCPTokens(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.db.Exec("INSERT INTO users (id, name, role, created_at) VALUES ('u1', 'owner', 'owner', '2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "有中文", "a b", strings.Repeat("x", 41)} {
		if _, _, err := s.CreateMCPToken("u1", bad, 30); !errors.Is(err, ErrMCPTokenName) {
			t.Fatalf("名称 %q 应该不合格：%v", bad, err)
		}
	}
	if _, _, err := s.CreateMCPToken("u1", "x", 366); !errors.Is(err, ErrMCPTokenTTL) {
		t.Fatalf("有效期超过 365 天应该不行：%v", err)
	}
	tok, info, err := s.CreateMCPToken("u1", "claude-code", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, MCPTokenPrefix) || info.ExpiresAt == nil {
		t.Fatalf("令牌不对：%s %+v", tok, info)
	}
	if _, _, err := s.CreateMCPToken("u1", "claude-code", 0); !errors.Is(err, ErrMCPTokenExists) {
		t.Fatalf("重名应该不行：%v", err)
	}
	// 库里只有摘要，没有令牌本体。
	var n int
	s.db.QueryRow("SELECT COUNT(*) FROM mcp_tokens WHERE token_hash = ? OR token_hash = ?", tok, hashToken(tok)).Scan(&n)
	var raw int
	s.db.QueryRow("SELECT COUNT(*) FROM mcp_tokens WHERE token_hash = ?", tok).Scan(&raw)
	if n != 1 || raw != 0 {
		t.Fatalf("应该只存摘要：%d %d", n, raw)
	}

	got, err := s.LookupMCPToken(tok)
	if err != nil || got.UserID != "u1" || got.Role != "owner" || got.Name != "claude-code" {
		t.Fatalf("查令牌：%+v %v", got, err)
	}
	if _, err := s.LookupMCPToken(tok + "x"); err == nil {
		t.Fatal("错的令牌不该查到")
	}
	if _, err := s.LookupMCPToken("hostower_session_like"); err == nil {
		t.Fatal("没有前缀的不该查到")
	}

	// 过期了就不能用，但仍然列出来，方便吊销。
	s.db.Exec("UPDATE mcp_tokens SET expires_at = ? WHERE id = ?", time.Now().Add(-time.Minute).UTC().Format(time.RFC3339), info.ID)
	if _, err := s.LookupMCPToken(tok); err == nil {
		t.Fatal("过期的令牌不该能用")
	}
	if list, _ := s.MCPTokens(); len(list) != 1 {
		t.Fatalf("过期的也要列出：%d", len(list))
	}

	// 永久的令牌没有到期时间；吊销之后就没了。
	tok2, info2, err := s.CreateMCPToken("u1", "codex", 0)
	if err != nil || info2.ExpiresAt != nil {
		t.Fatalf("永久令牌：%+v %v", info2, err)
	}
	if _, err := s.RevokeMCPToken(info2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LookupMCPToken(tok2); err == nil {
		t.Fatal("吊销后不该能用")
	}
}
