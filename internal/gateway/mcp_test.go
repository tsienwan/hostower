package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
)

// mcpPost 走完整的 Handler（包括 apiMCP 的认证和来源校验）发一条 JSON-RPC。
func mcpPost(t *testing.T, s *Server, token, origin, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

// call 调一个工具，返回结构化结果；isError 时返回错误文本。
func call(t *testing.T, s *Server, token, tool string, args any) (map[string]any, string) {
	t.Helper()
	a, _ := json.Marshal(args)
	w, out := mcpPost(t, s, token, "", `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"`+tool+`","arguments":`+string(a)+`}}`)
	if w.Code != 200 {
		t.Fatalf("%s：HTTP %d %s", tool, w.Code, w.Body.String())
	}
	res, _ := out["result"].(map[string]any)
	if res == nil {
		t.Fatalf("%s：没有结果 %v", tool, out)
	}
	if res["isError"] == true {
		c := res["content"].([]any)[0].(map[string]any)
		return nil, c["text"].(string)
	}
	sc, _ := res["structuredContent"].(map[string]any)
	return sc, ""
}

func mcpSetup(t *testing.T) (*Server, string) {
	t.Helper()
	s, _ := chatSetup(t)
	tok, _, err := s.Auth.CreateMCPToken("u1", "claude-code", 30)
	if err != nil {
		t.Fatal(err)
	}
	return s, tok
}

func TestMCPAuth(t *testing.T) {
	s, tok := mcpSetup(t)
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`

	if w, _ := mcpPost(t, s, "", "", init); w.Code != 401 {
		t.Fatalf("没有令牌应该 401：%d", w.Code)
	}
	if w, _ := mcpPost(t, s, "hto_nope", "", init); w.Code != 401 {
		t.Fatalf("错的令牌应该 401：%d", w.Code)
	}
	if w, _ := mcpPost(t, s, tok, "https://evil.example.net", init); w.Code != 403 {
		t.Fatalf("别的站点发来的应该 403：%d", w.Code)
	}
	// 会话 Cookie 不能代替令牌。
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(init))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: "whatever"})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("只带 Cookie 应该 401：%d", w.Code)
	}

	w2, out := mcpPost(t, s, tok, "https://hostower.example.com", init)
	if w2.Code != 200 {
		t.Fatalf("initialize：%d %s", w2.Code, w2.Body.String())
	}
	res := out["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" || res["serverInfo"].(map[string]any)["name"] != "hostower" {
		t.Fatalf("initialize 结果不对：%v", res)
	}
	if w, _ := mcpPost(t, s, tok, "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); w.Code != 202 {
		t.Fatalf("通知应该 202：%d", w.Code)
	}

	// 工具里没有批准、终端、文件、设置。
	_, out = mcpPost(t, s, tok, "", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var names []string
	for _, tool := range out["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	joined := strings.Join(names, ",")
	for _, bad := range []string{"approve", "deny", "terminal", "file", "secret", "pause", "resume", "setting"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("不该有 %s 这类工具：%s", bad, joined)
		}
	}

	// 签发人不再是所有者，令牌就不能用。
	s.DB.Exec("UPDATE users SET role = 'ops' WHERE id = 'u1'")
	if w, _ := mcpPost(t, s, tok, "", init); w.Code != 401 {
		t.Fatalf("签发人降级后应该 401：%d", w.Code)
	}
	s.DB.Exec("UPDATE users SET role = 'owner' WHERE id = 'u1'")

	// 吊销所有会话会连 MCP 令牌一起吊销。
	if err := s.Auth.RevokeAll(); err != nil {
		t.Fatal(err)
	}
	if w, _ := mcpPost(t, s, tok, "", init); w.Code != 401 {
		t.Fatalf("吊销所有会话后应该 401：%d", w.Code)
	}
}

// 完整的一条路：MCP 提交 L1 → 等待批准 → MCP 自己批不了 → 人在网页上批准 → MCP 拿到结果；审计里是 entry=mcp。
func TestMCPSubmitApprove(t *testing.T) {
	s, tok := mcpSetup(t)
	ctx := context.Background()

	hosts, errText := call(t, s, tok, "list_hosts", map[string]any{})
	if errText != "" || len(hosts["hosts"].([]any)) != 1 {
		t.Fatalf("list_hosts：%v %s", hosts, errText)
	}

	cls, _ := call(t, s, tok, "classify_command", map[string]any{"hosts": []string{"web-1"}, "command": "systemctl restart nginx"})
	if cls["level"] != "L1" {
		t.Fatalf("预估应该是 L1：%v", cls)
	}

	if _, e := call(t, s, tok, "submit_command", map[string]any{"hosts": []string{"web-1"}, "command": "systemctl restart nginx"}); e == "" {
		t.Fatal("没有 intent 应该报错")
	}
	sub, e := call(t, s, tok, "submit_command", map[string]any{"hosts": []string{"web-1"}, "command": "systemctl restart nginx", "intent": "重启 nginx 让新配置生效"})
	if e != "" || sub["state"] != "pending" || sub["level"] != "L1" {
		t.Fatalf("提交 L1 应该等待批准：%v %s", sub, e)
	}
	id := sub["id"].(string)
	if !strings.HasSuffix(sub["approveUrl"].(string), "/requests/"+id) {
		t.Fatalf("批准链接不对：%v", sub["approveUrl"])
	}

	req, _ := s.Kernel.Get(id)
	if req.Actor.Kind != kernel.AI || req.Actor.Entry != "mcp" || req.Actor.CanApprove {
		t.Fatalf("发起人不对：%+v", req.Actor)
	}
	// MCP 的发起人不能批准（哪怕是自己的请求）。
	if _, err := s.Kernel.Approve(ctx, id, mcpActor(&auth.MCPToken{UserID: "u1", UserName: "owner", Name: "claude-code"}), kernel.Proof{Digest: req.Digest}); err == nil {
		t.Fatal("AI 不该能批准")
	}

	// 等一下还没批准：返回的仍是 pending。
	got, _ := call(t, s, tok, "get_request", map[string]any{"id": id, "wait_seconds": 1})
	if got["state"] != "pending" {
		t.Fatalf("没批准之前应该还在等：%v", got)
	}

	// 人在网页上批准。
	if _, err := s.Kernel.Approve(ctx, id, testOwner, kernel.Proof{Digest: req.Digest}); err != nil {
		t.Fatal(err)
	}
	got, _ = call(t, s, tok, "get_request", map[string]any{"id": id, "wait_seconds": 10})
	if got["state"] != "done" {
		t.Fatalf("批准后应该执行完：%v", got)
	}
	tg := got["targets"].([]any)[0].(map[string]any)
	if tg["exitCode"] != float64(0) {
		t.Fatalf("退出码不对：%v", tg)
	}

	list, _ := call(t, s, tok, "list_requests", map[string]any{})
	if n := len(list["requests"].([]any)); n != 1 {
		t.Fatalf("应该只列出自己的 1 个请求：%d", n)
	}

	// 别人（网页上的人）提交的请求，MCP 看不到也取消不了。
	other, err := s.Kernel.SubmitCommand(ctx, testOwner, kernel.CommandSpec{Hosts: []string{"web-1"}, Commands: map[string]string{"web-1": "systemctl restart nginx"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, e := call(t, s, tok, "get_request", map[string]any{"id": other.ID}); e == "" {
		t.Fatal("别人的请求应该当作不存在")
	}
	if _, e := call(t, s, tok, "cancel_request", map[string]any{"id": other.ID}); e == "" {
		t.Fatal("不该能取消别人的请求")
	}

	// L3 被内核拒绝。
	rej, e := call(t, s, tok, "submit_command", map[string]any{"hosts": []string{"web-1"}, "command": "rm -rf /", "intent": "测试"})
	if e != "" || rej["state"] != "rejected" {
		t.Fatalf("L3 应该被拒绝：%v %s", rej, e)
	}

	// 审计：请求记的是 entry=mcp、actorKind=ai；每次工具调用都有一条 mcp.call。
	recs, err := s.Kernel.Audit().List(ctx, 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	var submitted, calls int
	for _, rec := range recs {
		var e audit.Entry
		json.Unmarshal(rec.Body, &e)
		if e.Type == "request.submitted" && e.RequestID == id && e.Entry == "mcp" && e.ActorKind == "ai" {
			submitted++
		}
		if e.Type == "mcp.call" && e.Entry == "mcp" {
			calls++
		}
	}
	if submitted != 1 || calls < 8 {
		t.Fatalf("审计不对：submitted=%d calls=%d", submitted, calls)
	}
	if res, err := audit.Verify(ctx, s.DB); err != nil || !res.OK {
		t.Fatalf("哈希链应该完整：%+v %v", res, err)
	}
}
