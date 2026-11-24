package notify

import (
	"fmt"
	"time"

	"github.com/tsienwan/hostower/internal/i18n"
	"github.com/tsienwan/hostower/internal/metrics"
)

// 资源告警：磁盘、内存、CPU 的使用率持续超过阈值时发一次告警，回落到恢复线以下并保持一段时间后发一次恢复。
//
// 数据只来自已有的采样（metrics.Status），不往主机上执行任何东西。
// 告警线和恢复线分开（滞回），持续时长也分开计，免得在阈值附近来回抖时反复报。
// 已经告警、还没恢复的记在 resource_alerts 里，面板重启后不重发。阈值写在代码里，不提供开关。

type resourceRule struct {
	kind     string  // disk | memory | cpu
	over     float64 // 使用率到这个百分比开始计时
	clear    float64 // 告警后降到这个百分比以下才开始算恢复
	overFor  time.Duration
	clearFor time.Duration
}

var resourceRules = []resourceRule{
	{kind: "disk", over: 90, clear: 85, overFor: 5 * time.Minute, clearFor: 5 * time.Minute},
	{kind: "memory", over: 90, clear: 80, overFor: 5 * time.Minute, clearFor: 5 * time.Minute},
	{kind: "cpu", over: 90, clear: 80, overFor: 10 * time.Minute, clearFor: 5 * time.Minute},
}

type resTrack struct {
	loaded     bool
	alerted    bool      // 已经发过告警、还没恢复
	overSince  time.Time // 连续超过告警线的起点；零值表示当前没超
	clearSince time.Time // 告警后连续低于恢复线的起点
	alertAt    time.Time // 发告警的时间
}

func resKey(host, kind string) string { return host + "|" + kind }

// resourceValue 取一台主机某项资源的使用率。磁盘取使用率最高的挂载点。ok 为 false 表示没有可用的数据。
func resourceValue(st metrics.Status, kind string) (v float64, mount string, ok bool) {
	if !st.Online || st.Latest == nil {
		return 0, "", false
	}
	switch kind {
	case "disk":
		for _, d := range st.Disks {
			if !ok || d.Used > v {
				v, mount, ok = d.Used, d.Mount, true
			}
		}
		if !ok && st.DiskKB > 0 {
			return st.Latest.Disk, "/", true
		}
		return v, mount, ok
	case "memory":
		return st.Latest.Mem, "", st.MemKB > 0
	case "cpu":
		return st.Latest.CPU, "", true
	}
	return 0, "", false
}

// checkResourcesLocked 调用方持有 h.mu。
func (h *Hub) checkResourcesLocked(host string, st metrics.Status, now time.Time) {
	for _, r := range resourceRules {
		key := resKey(host, r.kind)
		t := h.resState[key]
		if t == nil {
			t = &resTrack{}
			h.resState[key] = t
		}
		if !t.loaded {
			t.loaded = true
			t.alerted, t.alertAt = h.resourceAlertedLocked(host, r.kind)
		}
		v, mount, ok := resourceValue(st, r.kind)
		if !ok {
			// 离线或者没有数据：计时作废，已经发出的告警保持，等有数据再判断恢复。
			t.overSince, t.clearSince = time.Time{}, time.Time{}
			continue
		}
		if !t.alerted {
			if v < r.over {
				t.overSince = time.Time{}
				continue
			}
			if t.overSince.IsZero() {
				t.overSince = now
			}
			if now.Sub(t.overSince) < r.overFor {
				continue
			}
			t.alerted, t.alertAt, t.clearSince = true, now, time.Time{}
			h.markResourceLocked(host, r.kind, now)
			h.log.Info("资源告警", "host", host, "kind", r.kind, "value", fmt.Sprintf("%.0f%%", v), "mount", mount)
			h.queueLocked(resourceMessage(r, host, v, mount, t.overSince, false))
			continue
		}
		if v >= r.clear {
			t.clearSince = time.Time{}
			continue
		}
		if t.clearSince.IsZero() {
			t.clearSince = now
		}
		if now.Sub(t.clearSince) < r.clearFor {
			continue
		}
		since := t.alertAt
		t.alerted, t.overSince, t.clearSince = false, time.Time{}, time.Time{}
		h.unmarkResourceLocked(host, r.kind)
		h.log.Info("资源告警恢复", "host", host, "kind", r.kind, "value", fmt.Sprintf("%.0f%%", v), "mount", mount)
		h.queueLocked(resourceMessage(r, host, v, mount, since, true))
	}
}

// forgetResourcesLocked 清掉已经不在清单里的主机的资源告警状态。
func (h *Hub) forgetResourcesLocked(host string) {
	for _, r := range resourceRules {
		delete(h.resState, resKey(host, r.kind))
	}
	if h.db != nil {
		h.db.Exec("DELETE FROM resource_alerts WHERE host = ?", host)
	}
}

func resourceMessage(r resourceRule, host string, v float64, mount string, since time.Time, recovered bool) Message {
	limit := fmt.Sprintf("%.0f", r.over)
	var title i18n.Text
	switch {
	case r.kind == "disk" && !recovered:
		title = i18n.Tx("磁盘使用率超过 {limit}%：{host}", "limit", limit, "host", host)
	case r.kind == "disk":
		title = i18n.Tx("磁盘使用率已回落：{host}", "host", host)
	case r.kind == "memory" && !recovered:
		title = i18n.Tx("内存使用率超过 {limit}%：{host}", "limit", limit, "host", host)
	case r.kind == "memory":
		title = i18n.Tx("内存使用率已回落：{host}", "host", host)
	case !recovered:
		title = i18n.Tx("CPU 使用率持续超过 {limit}%：{host}", "limit", limit, "host", host)
	default:
		title = i18n.Tx("CPU 使用率已回落：{host}", "host", host)
	}
	sev := Warn
	if recovered {
		sev = OK
	}
	fields := []Field{f(i18n.Tx("主机"), host)}
	if mount != "" {
		fields = append(fields, f(i18n.Tx("挂载点"), mount))
	}
	fields = append(fields, f(i18n.Tx("使用率"), fmt.Sprintf("%.0f%%", v)), f(i18n.Tx("告警线"), limit+"%"))
	if !since.IsZero() {
		name := i18n.Tx("开始时间")
		if recovered {
			name = i18n.Tx("告警时间")
		}
		fields = append(fields, f(name, since.Format("2006-01-02 15:04:05")))
	}
	return Message{Event: "host." + r.kind, Severity: sev, Link: "/hosts/" + host, Title: title, Fields: fields}
}

func (h *Hub) resourceAlertedLocked(host, kind string) (bool, time.Time) {
	if h.db == nil {
		return false, time.Time{}
	}
	var ts int64
	if err := h.db.QueryRow("SELECT ts FROM resource_alerts WHERE host = ? AND kind = ?", host, kind).Scan(&ts); err != nil {
		return false, time.Time{}
	}
	return true, time.Unix(ts, 0)
}

func (h *Hub) markResourceLocked(host, kind string, now time.Time) {
	if h.db == nil {
		return
	}
	if _, err := h.db.Exec("INSERT OR REPLACE INTO resource_alerts (host, kind, ts) VALUES (?, ?, ?)", host, kind, now.Unix()); err != nil {
		h.log.Warn("记录资源告警失败", "host", host, "kind", kind, "err", err)
	}
}

func (h *Hub) unmarkResourceLocked(host, kind string) {
	if h.db == nil {
		return
	}
	if _, err := h.db.Exec("DELETE FROM resource_alerts WHERE host = ? AND kind = ?", host, kind); err != nil {
		h.log.Warn("清除资源告警失败", "host", host, "kind", kind, "err", err)
	}
}
