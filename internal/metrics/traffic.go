package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"time"
)

// 月流量：按网卡的累计计数器记账。
//
// 每一轮采样拿到每块物理网卡（不含 lo、docker、veth 这些虚拟网卡）的累计收发字节数，和上一次见到的值相减，
// 就是这段时间的流量，记到当天（按面板所在机器的时区）。上一次的值连同主机的开机时长一起落盘，
// 所以面板重启、采样断线期间的流量也能补上。
//
// 判断主机重启过：开机时长比上次小，或者比“距上次见到”的时间还短。重启过就把当前计数器（开机以来的流量）全部记上；
// 只有“上次见到 → 重启”之间那一段没法知道，会少算。
//
// 这里只用已有的采样数据，不会为了统计流量去主机上执行任何东西。

type ifaceCounter struct{ RX, TX float64 }

// hostCounters 是一台主机上一次见到的计数器。
type hostCounters struct {
	up     float64 // 主机的开机时长（秒）
	ts     int64   // 面板这边的时间（unix 秒）
	ifaces map[string]ifaceCounter
}

type dayKey struct{ host, day, iface string }

// periodCache 是一台设了配额的主机在当前周期里已经落盘的用量。
type periodCache struct {
	start   string
	in, out float64
	since   int64 // 这台主机最早有流量数据的那天零点（unix 秒）；0 表示还没有
}

type trafficState struct {
	counters map[string]*hostCounters
	dirty    map[string]bool
	pending  map[dayKey]*[2]float64 // 还没写盘的增量：入站、出站
	written  time.Time
	quotas   map[string]Quota
	cache    map[string]periodCache
}

const (
	trafficWriteEvery = time.Minute
	trafficKeepDays   = 800 // 按天的流量保留两年多：够画 12 个周期的图，也够对上一年的账单
	dayLayout         = "2006-01-02"
)

// Quota 是一台主机的月流量配额。
type Quota struct {
	V          int     `json:"v"`
	Limit      float64 `json:"limit"`      // 额度，单位见 Unit
	Unit       string  `json:"unit"`       // GB | TB
	Base       int     `json:"base"`       // 1000 | 1024：服务商按哪种换算
	Mode       string  `json:"mode"`       // out 仅出站 | in 仅入站 | sum 入加出 | max 两者取大
	ResetDay   int     `json:"resetDay"`   // 每月几号重置（1–31；没有这一天的月份按最后一天）
	Iface      string  `json:"iface"`      // 只统计这块网卡；空表示全部物理网卡
	Thresholds []int   `json:"thresholds"` // 到达这些百分比时通知
	// Adjust 是校准差额（字节，可以是负数）：服务商面板上的已用量减去这里测到的。只对 AdjustStart 开始的那个周期有效。
	Adjust      float64 `json:"adjust"`
	AdjustStart string  `json:"adjustStart,omitempty"`
}

const quotaVersion = 1

// QuotaModes 是可选的计费方式。
var QuotaModes = []string{"out", "in", "sum", "max"}

// DefaultThresholds 是新设配额时默认的告警百分比。
var DefaultThresholds = []int{80, 95, 100}

var ifaceRe = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,32}$`)

// Normalize 检查配额是否合法，并整理告警百分比（去重、排序）。
func (q *Quota) Normalize() error {
	q.V = quotaVersion
	if q.Unit != "GB" && q.Unit != "TB" {
		return errors.New("单位只能是 GB 或 TB")
	}
	if q.Base != 1000 && q.Base != 1024 {
		return errors.New("换算只能按 1000 或 1024")
	}
	if !(q.Limit > 0 && q.Limit <= 1e6) {
		return errors.New("额度要大于 0")
	}
	ok := false
	for _, m := range QuotaModes {
		ok = ok || m == q.Mode
	}
	if !ok {
		return errors.New("不认识的计费方式")
	}
	if q.ResetDay < 1 || q.ResetDay > 31 {
		return errors.New("重置日要在 1 到 31 之间")
	}
	if q.Iface != "" && !ifaceRe.MatchString(q.Iface) {
		return errors.New("网卡名不合法")
	}
	seen := map[int]bool{}
	var th []int
	for _, v := range q.Thresholds {
		if v < 1 || v > 200 {
			return errors.New("告警百分比要在 1 到 200 之间")
		}
		if !seen[v] {
			seen[v] = true
			th = append(th, v)
		}
	}
	if len(th) > 5 {
		return errors.New("告警百分比最多 5 个")
	}
	sort.Ints(th)
	q.Thresholds = th
	if math.IsNaN(q.Adjust) || math.IsInf(q.Adjust, 0) {
		q.Adjust, q.AdjustStart = 0, ""
	}
	if q.Adjust == 0 {
		q.AdjustStart = ""
	}
	return nil
}

// UnitBytes 是一个“单位”（GB 或 TB）有多少字节。
func (q Quota) UnitBytes() float64 {
	b := float64(q.Base)
	if q.Unit == "TB" {
		return b * b * b * b
	}
	return b * b * b
}

// LimitBytes 是额度的字节数。
func (q Quota) LimitBytes() float64 { return q.Limit * q.UnitBytes() }

// Billable 按计费方式把入站、出站折成计费用量。
func (q Quota) Billable(in, out float64) float64 {
	switch q.Mode {
	case "in":
		return in
	case "out":
		return out
	case "max":
		return math.Max(in, out)
	}
	return in + out
}

// Period 返回 now 所在的计费周期 [start, end)：从每月的重置日零点开始（没有这一天的月份按最后一天）。
func Period(resetDay int, now time.Time) (start, end time.Time) {
	loc := now.Location()
	at := func(y int, m time.Month) time.Time {
		first := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		dim := first.AddDate(0, 1, -1).Day()
		d := resetDay
		if d > dim {
			d = dim
		}
		return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, loc)
	}
	y, m, _ := now.Date()
	start = at(y, m)
	if now.Before(start) {
		start = at(y, m-1)
	}
	end = at(start.Year(), start.Month()+1)
	return start, end
}

// QuotaStatus 是一台主机本周期的用量（随主机状态一起推给页面）。
type QuotaStatus struct {
	Limit     float64 `json:"limit"`     // 额度（字节）
	Used      float64 `json:"used"`      // 按计费方式算的已用量（含校准）
	In        float64 `json:"in"`        // 本周期测到的入站字节
	Out       float64 `json:"out"`       // 本周期测到的出站字节
	Pct       float64 `json:"pct"`       // 已用百分比
	Projected float64 `json:"projected"` // 按本周期的平均速度，到周期末的预计用量；0 表示数据还不够
	Window    float64 `json:"window"`    // 算平均速度用了多长时间的数据（秒）
	Start     string  `json:"start"`     // 周期开始（含）
	End       string  `json:"end"`       // 周期结束（不含）
	DaysLeft  float64 `json:"daysLeft"`  // 离周期结束还有几天
	Mode      string  `json:"mode"`
	Unit      string  `json:"unit"`
	Base      int     `json:"base"`
	Adjusted  bool    `json:"adjusted"` // 本周期做过校准
	// Thresholds 是告警的百分比（通知中心按它判断该不该发）。
	Thresholds []int `json:"thresholds"`
}

func newTraffic() trafficState {
	return trafficState{counters: map[string]*hostCounters{}, dirty: map[string]bool{}, pending: map[dayKey]*[2]float64{},
		quotas: map[string]Quota{}, cache: map[string]periodCache{}, written: time.Now()}
}

// loadTraffic 读出上次见到的计数器和各主机的配额。
func (s *Sampler) loadTraffic() {
	if s.db == nil {
		return
	}
	if rows, err := s.db.Query("SELECT host, iface, up, ts, rx, tx FROM traffic_counters"); err == nil {
		for rows.Next() {
			var host, iface string
			var c hostCounters
			var v ifaceCounter
			if rows.Scan(&host, &iface, &c.up, &c.ts, &v.RX, &v.TX) != nil {
				continue
			}
			hc := s.tr.counters[host]
			if hc == nil {
				hc = &hostCounters{up: c.up, ts: c.ts, ifaces: map[string]ifaceCounter{}}
				s.tr.counters[host] = hc
			}
			hc.ifaces[iface] = v
		}
		rows.Close()
	}
	if rows, err := s.db.Query("SELECT host, body FROM traffic_quotas"); err == nil {
		for rows.Next() {
			var host, body string
			var q Quota
			if rows.Scan(&host, &body) == nil && json.Unmarshal([]byte(body), &q) == nil && q.Normalize() == nil {
				s.tr.quotas[host] = q
			}
		}
		rows.Close()
	}
	s.refreshPeriods(time.Now())
}

// meterLocked 记下一轮采样里各网卡的计数器，把和上一次的差记进当天的流量。调用方持有 s.mu。
func (s *Sampler) meterLocked(host string, cur map[string]ifaceCounter, up float64, now time.Time) {
	prev := s.tr.counters[host]
	s.tr.counters[host] = &hostCounters{up: up, ts: now.Unix(), ifaces: cur}
	s.tr.dirty[host] = true
	if prev == nil {
		return // 第一次见到这台主机：只记下起点
	}
	rebooted := up < prev.up || up+60 < float64(now.Unix()-prev.ts)
	day := now.Format(dayLayout)
	for iface, c := range cur {
		p, seen := prev.ifaces[iface]
		var drx, dtx float64
		switch {
		case rebooted:
			drx, dtx = c.RX, c.TX
		case !seen:
			continue // 新出现的网卡：从现在开始算
		default:
			// 计数器变小了但主机没重启（网卡被重建）：当前值就是重建以来的流量。
			if drx = c.RX - p.RX; drx < 0 {
				drx = c.RX
			}
			if dtx = c.TX - p.TX; dtx < 0 {
				dtx = c.TX
			}
		}
		if drx == 0 && dtx == 0 {
			continue
		}
		k := dayKey{host, day, iface}
		v := s.tr.pending[k]
		if v == nil {
			v = &[2]float64{}
			s.tr.pending[k] = v
		}
		v[0] += drx
		v[1] += dtx
	}
}

// forgetCountersLocked 在主机从清单里移除时丢掉它的计数器：同名主机再加回来时重新从起点算。调用方持有 s.mu。
func (s *Sampler) forgetCountersLocked(host string) {
	delete(s.tr.counters, host)
	s.tr.dirty[host] = true
}

// flushTraffic 每分钟把流量增量和计数器写一次盘，然后刷新各主机本周期的用量；跨进新周期时也刷新。
func (s *Sampler) flushTraffic(force bool) {
	if s.db == nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	due := force || now.Sub(s.tr.written) >= trafficWriteEvery
	stale := false
	for host, q := range s.tr.quotas {
		start, _ := Period(q.ResetDay, now)
		if s.tr.cache[host].start != start.Format(dayLayout) {
			stale = true
		}
	}
	if !due {
		s.mu.Unlock()
		if stale {
			s.refreshPeriods(now)
		}
		return
	}
	pending := s.tr.pending
	s.tr.pending = map[dayKey]*[2]float64{}
	counters := map[string]*hostCounters{}
	for host := range s.tr.dirty {
		counters[host] = s.tr.counters[host] // nil 表示删掉
	}
	s.tr.dirty = map[string]bool{}
	s.tr.written = now
	s.mu.Unlock()

	if err := s.writeTraffic(pending, counters); err != nil {
		slog.Warn("流量写入失败，下一次再试", "err", err)
		s.mu.Lock()
		for k, v := range pending {
			if cur := s.tr.pending[k]; cur != nil {
				cur[0] += v[0]
				cur[1] += v[1]
			} else {
				s.tr.pending[k] = v
			}
		}
		for host := range counters {
			s.tr.dirty[host] = true
		}
		s.mu.Unlock()
		return
	}
	s.refreshPeriods(now)
}

func (s *Sampler) writeTraffic(pending map[dayKey]*[2]float64, counters map[string]*hostCounters) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range pending {
		if _, err := tx.Exec(`INSERT INTO traffic_daily (host, day, iface, rx, tx) VALUES (?,?,?,?,?)
ON CONFLICT(host, day, iface) DO UPDATE SET rx = rx + excluded.rx, tx = tx + excluded.tx`, k.host, k.day, k.iface, v[0], v[1]); err != nil {
			return err
		}
	}
	for host, c := range counters {
		if _, err := tx.Exec("DELETE FROM traffic_counters WHERE host = ?", host); err != nil {
			return err
		}
		if c == nil {
			continue
		}
		for iface, v := range c.ifaces {
			if _, err := tx.Exec("INSERT INTO traffic_counters (host, iface, up, ts, rx, tx) VALUES (?,?,?,?,?,?)", host, iface, c.up, c.ts, v.RX, v.TX); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// refreshPeriods 从数据库重新算各个设了配额的主机本周期已经落盘的用量。
func (s *Sampler) refreshPeriods(now time.Time) {
	if s.db == nil {
		return
	}
	s.mu.RLock()
	quotas := make(map[string]Quota, len(s.tr.quotas))
	for h, q := range s.tr.quotas {
		quotas[h] = q
	}
	s.mu.RUnlock()
	cache := map[string]periodCache{}
	for host, q := range quotas {
		start, _ := Period(q.ResetDay, now)
		c := periodCache{start: start.Format(dayLayout)}
		_, end := Period(q.ResetDay, now)
		days, err := s.dailyTraffic(context.Background(), host, q.Iface, start, end, false)
		if err != nil {
			continue
		}
		for _, d := range days {
			c.in += d.In
			c.out += d.Out
		}
		var first sql.NullString
		if s.db.QueryRow("SELECT MIN(day) FROM traffic_daily WHERE host = ?", host).Scan(&first) == nil && first.Valid {
			if t, err := time.ParseInLocation(dayLayout, first.String, now.Location()); err == nil {
				c.since = t.Unix()
			}
		}
		cache[host] = c
	}
	s.mu.Lock()
	s.tr.cache = cache
	s.mu.Unlock()
}

// quotaStatusLocked 算一台主机本周期的用量；没有设配额时返回 nil。调用方至少持有 s.mu 的读锁。
func (s *Sampler) quotaStatusLocked(host string, now time.Time) *QuotaStatus {
	q, ok := s.tr.quotas[host]
	if !ok {
		return nil
	}
	start, end := Period(q.ResetDay, now)
	st := &QuotaStatus{Limit: q.LimitBytes(), Start: start.Format(dayLayout), End: end.Format(dayLayout), Mode: q.Mode, Unit: q.Unit, Base: q.Base,
		Thresholds: append([]int(nil), q.Thresholds...), DaysLeft: end.Sub(now).Hours() / 24}
	c := s.tr.cache[host]
	since := c.since
	if c.start == st.Start {
		st.In, st.Out = c.in, c.out
	}
	var live []string
	if c := s.tr.counters[host]; c != nil {
		for n := range c.ifaces {
			live = append(live, n)
		}
	}
	for k := range s.tr.pending {
		if k.host == host {
			live = append(live, k.iface)
		}
	}
	keep := UplinkIfaces(live)
	for k, v := range s.tr.pending {
		if k.host == host && k.day >= st.Start && (q.Iface == k.iface || (q.Iface == "" && keep[k.iface])) {
			st.In += v[0]
			st.Out += v[1]
			if since == 0 {
				since = now.Unix()
			}
		}
	}
	measured := q.Billable(st.In, st.Out)
	st.Used = measured
	if q.Adjust != 0 && q.AdjustStart == st.Start {
		st.Used, st.Adjusted = math.Max(0, measured+q.Adjust), true
	}
	if st.Limit > 0 {
		st.Pct = 100 * st.Used / st.Limit
	}
	// 预计用量：只按这里测到的部分算平均速度（校准差额是接入前用掉的，不代表速度）。
	from := start.Unix()
	if since > from {
		from = since
	}
	if window := float64(now.Unix() - from); window >= 6*3600 && since > 0 {
		st.Window = window
		st.Projected = st.Used + measured/window*end.Sub(now).Seconds()
	}
	return st
}

// Quota 返回一台主机的配额。
func (s *Sampler) Quota(host string) (Quota, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	q, ok := s.tr.quotas[host]
	return q, ok
}

// SetQuota 保存（q 为 nil 时取消）一台主机的配额。调用方负责先写审计。
func (s *Sampler) SetQuota(host string, q *Quota) error {
	if q != nil {
		if err := q.Normalize(); err != nil {
			return err
		}
	}
	if s.db != nil {
		var err error
		if q == nil {
			_, err = s.db.Exec("DELETE FROM traffic_quotas WHERE host = ?", host)
		} else {
			body, _ := json.Marshal(q)
			_, err = s.db.Exec(`INSERT INTO traffic_quotas (host, body, updated_at) VALUES (?, ?, ?)
ON CONFLICT(host) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at`, host, string(body), time.Now().UTC().Format(time.RFC3339))
		}
		if err != nil {
			return err
		}
	}
	s.mu.Lock()
	if q == nil {
		delete(s.tr.quotas, host)
	} else {
		s.tr.quotas[host] = *q
	}
	s.mu.Unlock()
	s.refreshPeriods(time.Now())
	return nil
}

// Measured 返回按 q 的周期、网卡和计费方式，本周期这里测到的用量（不含校准）。用来把“服务商显示的已用量”换算成校准差额。
func (s *Sampler) Measured(ctx context.Context, host string, q Quota) (float64, error) {
	start, end := Period(q.ResetDay, time.Now())
	days, err := s.DailyTraffic(ctx, host, q.Iface, start, end)
	if err != nil {
		return 0, err
	}
	var in, out float64
	for _, d := range days {
		in += d.In
		out += d.Out
	}
	return q.Billable(in, out), nil
}

// DayTraffic 是一天的流量。
type DayTraffic struct {
	Day string  `json:"day"`
	In  float64 `json:"in"`
	Out float64 `json:"out"`
}

// DailyTraffic 返回 [from, to) 里每天的流量（含还没写盘的部分）。iface 为空时按统计口径只算出口网卡（见 uplink.go）。没有数据的日子不返回。
func (s *Sampler) DailyTraffic(ctx context.Context, host, iface string, from, to time.Time) ([]DayTraffic, error) {
	return s.dailyTraffic(ctx, host, iface, from, to, true)
}

// dailyTraffic 是 DailyTraffic；withPending 为 false 时只算已经落盘的（本周期用量的缓存只存落盘的部分，没写盘的另外加，不能两边都算）。
func (s *Sampler) dailyTraffic(ctx context.Context, host, iface string, from, to time.Time, withPending bool) ([]DayTraffic, error) {
	a, b := from.Format(dayLayout), to.Format(dayLayout)
	type cell struct{ rx, tx float64 }
	byDay := map[string]map[string]*cell{} // 日期 → 网卡 → 流量
	add := func(day, name string, rx, tx float64) {
		m := byDay[day]
		if m == nil {
			m = map[string]*cell{}
			byDay[day] = m
		}
		c := m[name]
		if c == nil {
			c = &cell{}
			m[name] = c
		}
		c.rx += rx
		c.tx += tx
	}
	if s.db != nil {
		rows, err := s.db.QueryContext(ctx, `SELECT day, iface, rx, tx FROM traffic_daily WHERE host = ? AND day >= ? AND day < ? AND (? = '' OR iface = ?)`,
			host, a, b, iface, iface)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var day, name string
			var rx, tx float64
			if err := rows.Scan(&day, &name, &rx, &tx); err != nil {
				rows.Close()
				return nil, err
			}
			add(day, name, rx, tx)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	if withPending {
		s.mu.RLock()
		for k, v := range s.tr.pending {
			if k.host == host && k.day >= a && k.day < b && (iface == "" || k.iface == iface) {
				add(k.day, k.iface, v[0], v[1])
			}
		}
		s.mu.RUnlock()
	}
	out := make([]DayTraffic, 0, len(byDay))
	for day, m := range byDay {
		names := make([]string, 0, len(m))
		for n := range m {
			names = append(names, n)
		}
		keep := UplinkIfaces(names)
		d := DayTraffic{Day: day}
		for n, c := range m {
			if iface != "" || keep[n] {
				d.In += c.rx
				d.Out += c.tx
			}
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
}

// PeriodTraffic 是一个计费周期的流量。
type PeriodTraffic struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	In    float64 `json:"in"`
	Out   float64 `json:"out"`
}

// PeriodsTraffic 返回最近 n 个计费周期（按 resetDay 划分，最后一个是当前周期）的流量。
func (s *Sampler) PeriodsTraffic(ctx context.Context, host, iface string, resetDay, n int) ([]PeriodTraffic, error) {
	now := time.Now()
	periods := make([]PeriodTraffic, n)
	at := now
	var first time.Time
	for i := n - 1; i >= 0; i-- {
		st, en := Period(resetDay, at)
		periods[i] = PeriodTraffic{Start: st.Format(dayLayout), End: en.Format(dayLayout)}
		first = st
		at = st.Add(-time.Second)
	}
	_, end := Period(resetDay, now)
	days, err := s.DailyTraffic(ctx, host, iface, first, end)
	if err != nil {
		return nil, err
	}
	for _, d := range days {
		for i := range periods {
			if d.Day >= periods[i].Start && d.Day < periods[i].End {
				periods[i].In += d.In
				periods[i].Out += d.Out
				break
			}
		}
	}
	return periods, nil
}

// HostTraffic 是流量排行里的一台主机：设了配额的按配额的周期和网卡算，没设的按自然月、全部物理网卡算。
type HostTraffic struct {
	Host  string       `json:"host"`
	Start string       `json:"start"`
	End   string       `json:"end"`
	In    float64      `json:"in"`
	Out   float64      `json:"out"`
	Quota *QuotaStatus `json:"quota,omitempty"`
}

// TrafficRanking 返回这些主机本周期的流量。
func (s *Sampler) TrafficRanking(ctx context.Context, hosts []string) ([]HostTraffic, error) {
	now := time.Now()
	out := make([]HostTraffic, 0, len(hosts))
	for _, h := range hosts {
		q, ok := s.Quota(h)
		if !ok {
			q = Quota{ResetDay: 1, Mode: "sum"}
		}
		start, end := Period(q.ResetDay, now)
		days, err := s.DailyTraffic(ctx, h, q.Iface, start, end)
		if err != nil {
			return nil, err
		}
		t := HostTraffic{Host: h, Start: start.Format(dayLayout), End: end.Format(dayLayout)}
		for _, d := range days {
			t.In += d.In
			t.Out += d.Out
		}
		if ok {
			s.mu.RLock()
			t.Quota = s.quotaStatusLocked(h, now)
			s.mu.RUnlock()
		}
		out = append(out, t)
	}
	return out, nil
}

// Ifaces 返回一台主机统计到的物理网卡（当前在用的，加上历史上有过流量的）。
func (s *Sampler) Ifaces(ctx context.Context, host string) []string {
	seen := map[string]bool{}
	s.mu.RLock()
	if c := s.tr.counters[host]; c != nil {
		for name := range c.ifaces {
			seen[name] = true
		}
	}
	s.mu.RUnlock()
	if s.db != nil {
		if rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT iface FROM traffic_daily WHERE host = ?", host); err == nil {
			for rows.Next() {
				var n string
				if rows.Scan(&n) == nil {
					seen[n] = true
				}
			}
			rows.Close()
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// FormatBytes 按配额的单位把字节数写成 “12.3 GB”（按 1024 换算时写 GiB）。通知和聊天机器人用。
func (q Quota) FormatBytes(v float64) string {
	unit := q.Unit
	if q.Base == 1024 {
		unit = map[string]string{"GB": "GiB", "TB": "TiB"}[unit]
	}
	x := v / q.UnitBytes()
	switch {
	case math.Abs(x) >= 100:
		return fmt.Sprintf("%.0f %s", x, unit)
	case math.Abs(x) >= 10:
		return fmt.Sprintf("%.1f %s", x, unit)
	}
	return fmt.Sprintf("%.2f %s", x, unit)
}
