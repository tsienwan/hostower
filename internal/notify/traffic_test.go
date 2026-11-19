package notify

import (
	"testing"

	"github.com/tsienwan/hostower/internal/metrics"
)

func drain(h *Hub) []Message {
	var out []Message
	for {
		select {
		case m := <-h.queue:
			out = append(out, m)
		default:
			return out
		}
	}
}

func TestTrafficAlerts(t *testing.T) {
	h := newHub(t)
	q := &metrics.QuotaStatus{Limit: 1000e9, Used: 500e9, Pct: 50, Start: "2026-11-01", End: "2026-12-01", DaysLeft: 12, Unit: "GB", Base: 1000,
		Thresholds: []int{80, 95, 100}}
	h.mu.Lock()
	h.checkTrafficLocked("web-1", q)
	h.mu.Unlock()
	if got := drain(h); len(got) != 0 {
		t.Fatalf("没到阈值不该通知：%v", got)
	}
	// 一次跨过 80 和 95：只发 95。
	q.Used, q.Pct = 960e9, 96
	h.mu.Lock()
	h.checkTrafficLocked("web-1", q)
	h.checkTrafficLocked("web-1", q) // 同一档不重发
	h.mu.Unlock()
	got := drain(h)
	if len(got) != 1 || got[0].Event != "traffic.quota" || got[0].Title.In("zh-CN") != "流量已用 96%：web-1" {
		t.Fatalf("应该只发一条 95%% 的通知：%+v", got)
	}
	// 重启（内存里的记录没了）之后也不重发；到 100% 再发一次。
	h.trafficSent = map[string]bool{}
	q.Used, q.Pct = 1001e9, 100.1
	h.mu.Lock()
	h.checkTrafficLocked("web-1", q)
	h.mu.Unlock()
	if got := drain(h); len(got) != 1 || got[0].Severity != Crit {
		t.Fatalf("到 100%% 应该发一条严重通知：%+v", got)
	}
	// 下一个周期重新开始。
	q2 := *q
	q2.Start, q2.End, q2.Used, q2.Pct = "2026-12-01", "2027-01-01", 100e9, 10
	q2.Projected, q2.Window = 1500e9, 2*86400
	h.mu.Lock()
	h.checkTrafficLocked("web-1", &q2)
	h.checkTrafficLocked("web-1", &q2)
	h.mu.Unlock()
	if got := drain(h); len(got) != 1 || got[0].Event != "traffic.forecast" {
		t.Fatalf("预计超额应该提前通知一次：%+v", got)
	}
}
