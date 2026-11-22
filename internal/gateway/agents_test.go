package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 派一个只读任务给两台主机上的 Claude Code，批准执行完后让 web-1 上的 Claude 汇总。
func TestAgentTaskAndSummary(t *testing.T) {
	old := executor.SimAgentDelay
	executor.SimAgentDelay = 0
	t.Cleanup(func() { executor.SimAgentDelay = old })
	s := appSetup(t)
	ctx := context.Background()
	call := func(handler func(w *httptest.ResponseRecorder, body []byte), body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		handler(w, b)
		return w
	}
	task := func(w *httptest.ResponseRecorder, b []byte) {
		s.agentTask(w, asOwner(httptest.NewRequest("POST", "/api/v1/agents/task", bytes.NewReader(b))))
	}
	summary := func(w *httptest.ResponseRecorder, b []byte) {
		s.agentSummary(w, asOwner(httptest.NewRequest("POST", "/api/v1/agents/summary", bytes.NewReader(b))))
	}

	// 预览：命令里带着任务原文（引号转义过），等级是变更。
	w := call(task, map[string]any{"agent": "claude", "hosts": []string{"web-1", "web-2"}, "dir": "/srv/app", "prompt": "看看 nginx 配置有没有问题 'x'; reboot", "preview": true})
	var pv struct {
		Level   risk.Level
		Targets []struct{ Host, Command string }
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &pv) != nil || pv.Level != risk.L1 || len(pv.Targets) != 2 {
		t.Fatalf("预览：%d %s", w.Code, w.Body.String())
	}
	if c := pv.Targets[0].Command; !strings.HasPrefix(c, "cd /srv/app && ") || !strings.Contains(c, "claude -p ") || strings.Contains(c, "--permission-mode") {
		t.Fatalf("命令不对：%s", c)
	}

	// 允许改文件：Claude 只放开工作目录里的编辑。
	w = call(task, map[string]any{"agent": "claude", "hosts": []string{"web-1"}, "dir": "/srv/app", "prompt": "修一下", "write": true, "preview": true})
	if !strings.Contains(w.Body.String(), "--permission-mode acceptEdits") {
		t.Fatalf("允许改文件时应该带 acceptEdits：%s", w.Body.String())
	}

	// 提交、批准、执行完。
	w = call(task, map[string]any{"agent": "claude", "hosts": []string{"web-1", "web-2"}, "dir": "/srv/app", "prompt": "看看这台主机的负载"})
	var res struct{ Requests []*kernel.Request }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &res) != nil || len(res.Requests) != 1 {
		t.Fatalf("提交：%d %s", w.Code, w.Body.String())
	}
	req := res.Requests[0]
	if req.Snippet != "agent:claude" || len(req.Targets) != 2 || !strings.Contains(req.Intent, "Claude Code") {
		t.Fatalf("请求不对：%+v", req)
	}
	// 还没执行完不能汇总。
	if w := call(summary, map[string]any{"from": req.ID, "host": "web-1", "agent": "claude", "prompt": "汇总"}); w.Code == 200 {
		t.Fatal("没执行完不应该能汇总")
	}
	if _, err := s.Kernel.Approve(ctx, req.ID, testOwner, kernel.Proof{Digest: req.Digest, Reauthed: true, Typed: req.Confirm}); err != nil {
		t.Fatal(err)
	}
	done := waitState(t, s, req.ID, kernel.StateDone)
	if !strings.Contains(done.Targets[0].Output, "结论") {
		t.Fatalf("模拟助手的输出不对：%q", done.Targets[0].Output)
	}

	// Codex 不能拿来汇总（它不从标准输入读材料）。
	if w := call(summary, map[string]any{"from": req.ID, "host": "web-1", "agent": "codex", "prompt": "汇总"}); w.Code == 200 {
		t.Fatal("codex 不应该能汇总")
	}
	w = call(summary, map[string]any{"from": req.ID, "host": "web-1", "agent": "claude", "prompt": "对比各台主机的结论"})
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &res) != nil || len(res.Requests) != 1 {
		t.Fatalf("汇总：%d %s", w.Code, w.Body.String())
	}
	sum := res.Requests[0]
	if sum.Snippet != "agent-summary:claude" || sum.Input == nil || sum.Input.Kind != "upload" || sum.Input.Size == 0 {
		t.Fatalf("汇总请求不对：%+v", sum)
	}
	if _, err := s.Kernel.Approve(ctx, sum.ID, testOwner, kernel.Proof{Digest: sum.Digest, Reauthed: true, Typed: sum.Confirm}); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, sum.ID, kernel.StateDone)

	// 不合法的参数。
	for _, body := range []map[string]any{
		{"agent": "rm", "hosts": []string{"web-1"}, "dir": "/srv", "prompt": "x"},
		{"agent": "claude", "hosts": []string{"web-1"}, "dir": "srv", "prompt": "x"},
		{"agent": "claude", "hosts": []string{"web-1"}, "dir": "/srv", "prompt": "  "},
		{"agent": "claude", "hosts": []string{"nope"}, "dir": "/srv", "prompt": "x"},
	} {
		if w := call(task, body); w.Code == 200 {
			t.Errorf("%v 应该被拒绝", body)
		}
	}
}
