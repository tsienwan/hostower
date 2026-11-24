package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/metrics"
)

func resStatus(cpu, mem, disk float64) metrics.Status {
	return metrics.Status{Host: "web-1", Online: true, Known: true, MemKB: 1 << 20, DiskKB: 1 << 30,
		Disks:  []metrics.Disk{{Mount: "/", Used: disk}, {Mount: "/data", Used: 10}},
		Latest: &metrics.Sample{CPU: cpu, Mem: mem, Disk: disk}}
}

// feed 从 start 开始每 5 秒喂一次同样的状态，持续 d，返回发出的通知。
func feed(h *Hub, st metrics.Status, start time.Time, d time.Duration) ([]Message, time.Time) {
	now := start
	for ; now.Sub(start) <= d; now = now.Add(5 * time.Second) {
		h.mu.Lock()
		h.checkResourcesLocked("web-1", st, now)
		h.mu.Unlock()
	}
	return drain(h), now
}

func events(ms []Message) string {
	var s []string
	for _, m := range ms {
		s = append(s, m.Event+":"+string(m.Severity))
	}
	return strings.Join(s, ",")
}

func TestResourceThresholdAndDuration(t *testing.T) {
	h := newHub(t)
	t0 := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

	// 低于阈值多久都不报。
	if got, _ := feed(h, resStatus(85, 70, 89), t0, time.Hour); len(got) != 0 {
		t.Fatalf("没到阈值不该通知：%s", events(got))
	}
	// CPU 超过阈值但不到 10 分钟，不报。
	got, now := feed(h, resStatus(95, 70, 50), t0, 9*time.Minute)
	if len(got) != 0 {
		t.Fatalf("CPU 不到 10 分钟不该通知：%s", events(got))
	}
	// 中途回落一次，计时重来。
	h.mu.Lock()
	h.checkResourcesLocked("web-1", resStatus(50, 70, 50), now)
	h.mu.Unlock()
	if got, _ = feed(h, resStatus(95, 70, 50), now.Add(5*time.Second), 9*time.Minute); len(got) != 0 {
		t.Fatalf("回落过以后应重新计时：%s", events(got))
	}
	// 连续 10 分钟才报，而且只报一次。
	got, _ = feed(h, resStatus(95, 70, 50), t0.Add(2*time.Hour), time.Hour)
	if events(got) != "host.cpu:warn" {
		t.Fatalf("CPU 持续超过阈值应只通知一次：%s", events(got))
	}
	_, text := got[0].Text("zh-CN", "")
	if !strings.Contains(got[0].Title.In("zh-CN"), "90%") || !strings.Contains(text, "95%") {
		t.Fatalf("通知内容不对：%s / %s", got[0].Title.In("zh-CN"), text)
	}

	// 磁盘：取使用率最高的挂载点，5 分钟后报。
	h2 := newHub(t)
	got, _ = feed(h2, resStatus(10, 10, 93), t0, 6*time.Minute)
	if events(got) != "host.disk:warn" {
		t.Fatalf("磁盘应告警一次：%s", events(got))
	}
	if _, text := got[0].Text("zh-CN", ""); !strings.Contains(text, "挂载点：/") {
		t.Fatalf("磁盘告警应带挂载点：%s", text)
	}
}

func TestResourceRecovery(t *testing.T) {
	h := newHub(t)
	t0 := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	got, now := feed(h, resStatus(10, 95, 50), t0, 6*time.Minute)
	if events(got) != "host.memory:warn" {
		t.Fatalf("内存应告警：%s", events(got))
	}
	// 降到告警线以下、但还在恢复线以上：既不恢复也不重报。
	if got, now = feed(h, resStatus(10, 85, 50), now, time.Hour); len(got) != 0 {
		t.Fatalf("在告警线和恢复线之间不该通知：%s", events(got))
	}
	// 再冲上去也不重报。
	if got, now = feed(h, resStatus(10, 96, 50), now, time.Hour); len(got) != 0 {
		t.Fatalf("同一次告警不该重报：%s", events(got))
	}
	// 低于恢复线不到 5 分钟，不恢复。
	if got, now = feed(h, resStatus(10, 60, 50), now, 4*time.Minute); len(got) != 0 {
		t.Fatalf("恢复不到 5 分钟不该通知：%s", events(got))
	}
	// 离线期间不判断，也不恢复。
	off := resStatus(10, 60, 50)
	off.Online = false
	if got, now = feed(h, off, now, time.Hour); len(got) != 0 {
		t.Fatalf("离线时不该通知：%s", events(got))
	}
	// 回来后连续 5 分钟低于恢复线，恢复一次。
	got, now = feed(h, resStatus(10, 60, 50), now, time.Hour)
	if events(got) != "host.memory:ok" {
		t.Fatalf("应恢复一次：%s", events(got))
	}
	// 恢复后再超过阈值，是新的一次，可以再报。
	if got, _ = feed(h, resStatus(10, 95, 50), now, 6*time.Minute); events(got) != "host.memory:warn" {
		t.Fatalf("恢复后再次超过应再告警：%s", events(got))
	}
}

func TestResourceAlertSurvivesRestart(t *testing.T) {
	h := newHub(t)
	t0 := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	if got, _ := feed(h, resStatus(10, 10, 95), t0, 6*time.Minute); events(got) != "host.disk:warn" {
		t.Fatalf("磁盘应告警：%s", events(got))
	}
	// 模拟面板重启：内存里的状态清空，数据库还在。
	h.mu.Lock()
	h.resState = map[string]*resTrack{}
	h.mu.Unlock()
	got, now := feed(h, resStatus(10, 10, 95), t0.Add(time.Hour), time.Hour)
	if len(got) != 0 {
		t.Fatalf("重启后不该重报：%s", events(got))
	}
	if got, now = feed(h, resStatus(10, 10, 40), now, 6*time.Minute); events(got) != "host.disk:ok" {
		t.Fatalf("重启后仍应能恢复：%s", events(got))
	}
	if got, _ = feed(h, resStatus(10, 10, 95), now, 6*time.Minute); events(got) != "host.disk:warn" {
		t.Fatalf("磁盘应再次告警：%s", events(got))
	}
	// 主机从清单里移除，记录一并清掉。
	h.mu.Lock()
	h.forgetResourcesLocked("web-1")
	h.mu.Unlock()
	var n int
	h.db.QueryRow("SELECT COUNT(*) FROM resource_alerts").Scan(&n)
	if n != 0 {
		t.Fatalf("移除主机后应清掉记录，还剩 %d 条", n)
	}
}
