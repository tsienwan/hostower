package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/metrics"
)

// 月流量：看用量任何登录的人都可以；设置、修改、取消配额只有所有者能做，并且先写审计、写不进去就不改。
// 这里只读采样器已经记下的数据，不会为了统计流量去主机上执行任何东西。

const trafficPeriods = 12 // 按月流量图显示最近几个周期

func (s *Server) hostTraffic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.Kernel.Inventory().Host(name); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	q, has := s.Sampler.Quota(name)
	view := q
	if !has {
		view = metrics.Quota{ResetDay: 1, Mode: "sum"} // 没设配额：按自然月、全部物理网卡看
	}
	now := time.Now()
	start, end := metrics.Period(view.ResetDay, now)
	days, err := s.Sampler.DailyTraffic(r.Context(), name, view.Iface, start, end)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	periods, err := s.Sampler.PeriodsTraffic(r.Context(), name, view.Iface, view.ResetDay, trafficPeriods)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := map[string]any{"days": days, "periods": periods, "start": start.Format("2006-01-02"), "end": end.Format("2006-01-02"),
		"ifaces": s.Sampler.Ifaces(r.Context(), name), "modes": metrics.QuotaModes, "defaultThresholds": metrics.DefaultThresholds, "quota": nil}
	if has {
		out["quota"] = q
		if st, ok := s.Sampler.All(false)[name]; ok {
			out["status"] = st.Quota
		}
	}
	writeJSON(w, 200, out)
}

type quotaBody struct {
	Quota metrics.Quota `json:"quota"`
	// Used 是服务商面板上显示的本周期已用量（单位同额度）；填了就据此校准。
	Used *float64 `json:"used"`
	// ClearAdjust 清掉本周期的校准。
	ClearAdjust bool `json:"clearAdjust"`
}

func (s *Server) setHostTraffic(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	if _, ok := s.Kernel.Inventory().Host(name); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	var in quotaBody
	if !readJSON(w, r, &in) {
		return
	}
	q := in.Quota
	q.Adjust, q.AdjustStart = 0, ""
	if err := q.Normalize(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	old, had := s.Sampler.Quota(name)
	start, _ := metrics.Period(q.ResetDay, time.Now())
	switch {
	case in.Used != nil:
		if *in.Used < 0 || *in.Used > 1e7 {
			writeErr(w, 400, "已用量不合法")
			return
		}
		measured, err := s.Sampler.Measured(r.Context(), name, q)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		q.Adjust, q.AdjustStart = *in.Used*q.UnitBytes()-measured, start.Format("2006-01-02")
	case in.ClearAdjust:
	case had && old.Mode == q.Mode && old.Iface == q.Iface && old.ResetDay == q.ResetDay:
		// 计量口径没变：之前的校准照旧有效。口径变了，旧的差额就对不上了，丢掉。
		q.Adjust, q.AdjustStart = old.Adjust, old.AdjustStart
	}
	if err := q.Normalize(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if had && equalQuota(old, q) {
		writeJSON(w, 200, map[string]any{"quota": q})
		return
	}
	detail := map[string]any{"ip": s.clientIP(r), "after": q}
	if had {
		detail["before"] = old
	}
	if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "host.traffic_quota", Actor: sessionOf(r).User.Name, Entry: "web", Host: name,
		Message: "设置流量配额", Detail: detail}); err != nil {
		writeErr(w, 500, "写审计失败，没有修改")
		return
	}
	if err := s.Sampler.SetQuota(name, &q); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"quota": q})
}

func (s *Server) deleteHostTraffic(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	old, had := s.Sampler.Quota(name)
	if !had {
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "host.traffic_quota.removed", Actor: sessionOf(r).User.Name, Entry: "web", Host: name,
		Message: "取消流量配额", Detail: map[string]any{"ip": s.clientIP(r), "before": old}}); err != nil {
		writeErr(w, 500, "写审计失败，没有修改")
		return
	}
	if err := s.Sampler.SetQuota(name, nil); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// trafficRanking 返回全部主机本周期的流量（流量排行）。
func (s *Server) trafficRanking(w http.ResponseWriter, r *http.Request) {
	var names []string
	for _, h := range s.Kernel.Inventory().Hosts() {
		names = append(names, h.Name)
	}
	list, err := s.Sampler.TrafficRanking(r.Context(), names)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"hosts": list})
}

func equalQuota(a, b metrics.Quota) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
