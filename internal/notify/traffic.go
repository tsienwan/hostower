package notify

import (
	"fmt"
	"strconv"
	"time"

	"github.com/tsienwan/hostower/internal/i18n"
	"github.com/tsienwan/hostower/internal/metrics"
)

// 流量告警：本周期的用量到达设定的百分比时发一次；按当前速度预计会超出额度时也提前发一次。
// 每台主机每个周期每一档只发一次（记在 traffic_alerts 里，面板重启也不重发）。
// 同一次跨过好几档（比如刚校准过）只发最高的那一档。

// forecastAfter 是周期里至少要有这么多的数据，才按平均速度预计超额（刚开始时速度不稳）。
const forecastAfter = 24 * time.Hour

// checkTrafficLocked 调用方持有 h.mu。
func (h *Hub) checkTrafficLocked(host string, q *metrics.QuotaStatus) {
	if q.Limit <= 0 {
		return
	}
	top := 0
	for _, t := range q.Thresholds {
		if q.Pct >= float64(t) && !h.trafficSentLocked(host, q.Start, strconv.Itoa(t)) {
			top = t
		}
	}
	qq := metrics.Quota{Unit: q.Unit, Base: q.Base}
	period := q.Start + " ~ " + lastDay(q.End)
	if top > 0 {
		// 低于这一档的也一并记成已发：不会在之后补发更低的档位。
		for _, t := range q.Thresholds {
			if t <= top {
				h.markTrafficLocked(host, q.Start, strconv.Itoa(t))
			}
		}
		sev := Warn
		if top >= 100 {
			sev = Crit
		}
		h.queueLocked(Message{Event: "traffic.quota", Severity: sev, Link: "/hosts/" + host,
			Title: i18n.Tx("流量已用 {pct}%：{host}", "pct", fmt.Sprintf("%.0f", q.Pct), "host", host),
			Fields: []Field{f(i18n.Tx("主机"), host),
				f(i18n.Tx("已用"), qq.FormatBytes(q.Used)+" / "+qq.FormatBytes(q.Limit)),
				f(i18n.Tx("计费周期"), period),
				f(i18n.Tx("剩余天数"), fmt.Sprintf("%.1f", q.DaysLeft))}})
	}
	if q.Projected > q.Limit && q.Pct < 100 && q.Window >= forecastAfter.Seconds() && q.DaysLeft >= 1 &&
		!h.trafficSentLocked(host, q.Start, "forecast") {
		h.markTrafficLocked(host, q.Start, "forecast")
		h.queueLocked(Message{Event: "traffic.forecast", Severity: Warn, Link: "/hosts/" + host,
			Title: i18n.Tx("按当前速度将超出流量额度：{host}", "host", host),
			Fields: []Field{f(i18n.Tx("主机"), host),
				f(i18n.Tx("已用"), qq.FormatBytes(q.Used)+" / "+qq.FormatBytes(q.Limit)),
				f(i18n.Tx("预计周期末"), qq.FormatBytes(q.Projected)),
				f(i18n.Tx("计费周期"), period)}})
	}
}

func lastDay(end string) string {
	t, err := time.Parse("2006-01-02", end)
	if err != nil {
		return end
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

func (h *Hub) trafficSentLocked(host, period, level string) bool {
	key := host + "|" + period + "|" + level
	if sent, ok := h.trafficSent[key]; ok {
		return sent
	}
	var n int
	if h.db != nil {
		h.db.QueryRow("SELECT COUNT(*) FROM traffic_alerts WHERE host = ? AND period = ? AND level = ?", host, period, level).Scan(&n)
	}
	h.trafficSent[key] = n > 0
	return n > 0
}

func (h *Hub) markTrafficLocked(host, period, level string) {
	h.trafficSent[host+"|"+period+"|"+level] = true
	if h.db != nil {
		if _, err := h.db.Exec("INSERT OR IGNORE INTO traffic_alerts (host, period, level, ts) VALUES (?, ?, ?, ?)", host, period, level, time.Now().Unix()); err != nil {
			h.log.Warn("记录流量告警失败", "host", host, "err", err)
		}
	}
}
