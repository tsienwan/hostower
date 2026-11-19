package metrics

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestPeriod(t *testing.T) {
	loc := time.UTC
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, loc) }
	cases := []struct {
		reset      int
		now        time.Time
		start, end string
	}{
		{1, day(2026, 3, 15), "2026-03-01", "2026-04-01"},
		{1, time.Date(2026, 3, 1, 0, 0, 0, 0, loc), "2026-03-01", "2026-04-01"},
		{15, day(2026, 3, 10), "2026-02-15", "2026-03-15"},
		{15, day(2026, 3, 15), "2026-03-15", "2026-04-15"},
		{31, day(2026, 2, 20), "2026-01-31", "2026-02-28"},
		{31, day(2026, 2, 28), "2026-02-28", "2026-03-31"},
		{31, day(2026, 4, 30), "2026-04-30", "2026-05-31"},
		{5, day(2026, 1, 2), "2025-12-05", "2026-01-05"},
		{29, day(2028, 2, 29), "2028-02-29", "2028-03-29"},
	}
	for _, c := range cases {
		s, e := Period(c.reset, c.now)
		if s.Format(dayLayout) != c.start || e.Format(dayLayout) != c.end {
			t.Errorf("Period(%d, %s) = %s–%s，想要 %s–%s", c.reset, c.now.Format(dayLayout), s.Format(dayLayout), e.Format(dayLayout), c.start, c.end)
		}
	}
}

func testSampler() *Sampler {
	return &Sampler{hosts: map[string]*hostState{}, tr: newTraffic()}
}

func pendingSum(s *Sampler, host, iface string) (rx, tx float64) {
	for k, v := range s.tr.pending {
		if k.host == host && (iface == "" || k.iface == iface) {
			rx += v[0]
			tx += v[1]
		}
	}
	return
}

func TestMeter(t *testing.T) {
	s := testSampler()
	now := time.Now()
	c := func(rx, tx float64) map[string]ifaceCounter {
		return map[string]ifaceCounter{"eth0": {rx, tx}, "eth1": {rx / 10, tx / 10}}
	}
	// 第一次见到：只记起点，不把开机以来的流量算进来。
	s.meterLocked("web-1", c(1000, 2000), 1000, now)
	if rx, tx := pendingSum(s, "web-1", ""); rx != 0 || tx != 0 {
		t.Fatalf("第一次不该记流量：%v %v", rx, tx)
	}
	// 正常增长。
	now = now.Add(5 * time.Second)
	s.meterLocked("web-1", c(1500, 2600), 1005, now)
	if rx, tx := pendingSum(s, "web-1", "eth0"); rx != 500 || tx != 600 {
		t.Fatalf("增量不对：%v %v", rx, tx)
	}
	// 面板停了两小时，主机没重启：断开期间的流量照样补上。
	now = now.Add(2 * time.Hour)
	s.meterLocked("web-1", c(11500, 2600), 1005+7200, now)
	if rx, _ := pendingSum(s, "web-1", "eth0"); rx != 10500 {
		t.Fatalf("断开期间的流量没补上：%v", rx)
	}
	// 主机重启（开机时长变小）：记上开机以来的计数器。
	now = now.Add(time.Minute)
	s.meterLocked("web-1", c(300, 400), 30, now)
	if rx, tx := pendingSum(s, "web-1", "eth0"); rx != 10800 || tx != 1000 {
		t.Fatalf("重启后不对：%v %v", rx, tx)
	}
	// 面板停的时间比主机的开机时长还长：中间重启过。
	now = now.Add(3 * time.Hour)
	s.meterLocked("web-1", c(100, 100), 600, now)
	if rx, _ := pendingSum(s, "web-1", "eth0"); rx != 10900 {
		t.Fatalf("没有发现中间的重启：%v", rx)
	}
	// 网卡被重建（计数器变小，主机没重启）：当前值就是重建以来的。
	now = now.Add(5 * time.Second)
	s.meterLocked("web-1", map[string]ifaceCounter{"eth0": {50, 50}, "eth1": {10, 10}}, 605, now)
	if rx, _ := pendingSum(s, "web-1", "eth0"); rx != 10950 {
		t.Fatalf("计数器归零后不对：%v", rx)
	}
	// 新出现的网卡：从下一轮开始算。
	now = now.Add(5 * time.Second)
	s.meterLocked("web-1", map[string]ifaceCounter{"eth0": {50, 50}, "eth1": {10, 10}, "ens5": {9e9, 9e9}}, 610, now)
	if rx, _ := pendingSum(s, "web-1", "ens5"); rx != 0 {
		t.Fatalf("新网卡不该把旧计数算进来：%v", rx)
	}
}

func TestQuotaStatus(t *testing.T) {
	s := testSampler()
	q := Quota{Limit: 1, Unit: "TB", Base: 1000, Mode: "max", ResetDay: 1, Thresholds: []int{100, 80, 80}}
	if err := q.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(q.Thresholds) != 2 || q.Thresholds[0] != 80 {
		t.Fatalf("告警百分比没整理：%v", q.Thresholds)
	}
	now := time.Now()
	start, _ := Period(1, now)
	s.tr.quotas["web-1"] = q
	s.tr.cache["web-1"] = periodCache{start: start.Format(dayLayout), in: 100e9, out: 300e9, since: start.Unix()}
	s.tr.pending[dayKey{"web-1", now.Format(dayLayout), "eth0"}] = &[2]float64{0, 100e9}
	st := s.quotaStatusLocked("web-1", now)
	if st.Used != 400e9 || math.Abs(st.Pct-40) > 1e-9 {
		t.Fatalf("用量不对：%+v", st)
	}
	// 校准：服务商那边多算了 50 GB。
	q.Adjust, q.AdjustStart = 50e9, start.Format(dayLayout)
	s.tr.quotas["web-1"] = q
	st = s.quotaStatusLocked("web-1", now)
	if st.Used != 450e9 || !st.Adjusted {
		t.Fatalf("校准没生效：%+v", st)
	}
	// 上个周期的校准不再生效。
	q.AdjustStart = "2000-01-01"
	s.tr.quotas["web-1"] = q
	if st = s.quotaStatusLocked("web-1", now); st.Used != 400e9 || st.Adjusted {
		t.Fatalf("过期的校准还在生效：%+v", st)
	}
	for _, mode := range []struct {
		m    string
		want float64
	}{{"in", 100e9}, {"out", 400e9}, {"sum", 500e9}} {
		q.Mode = mode.m
		s.tr.quotas["web-1"] = q
		if st = s.quotaStatusLocked("web-1", now); st.Used != mode.want {
			t.Errorf("%s：%v，想要 %v", mode.m, st.Used, mode.want)
		}
	}
	if s.quotaStatusLocked("web-2", now) != nil {
		t.Fatal("没设配额的主机不该有用量")
	}
}

func TestQuotaNormalize(t *testing.T) {
	bad := []Quota{
		{Limit: 0, Unit: "GB", Base: 1000, Mode: "sum", ResetDay: 1},
		{Limit: 1, Unit: "PB", Base: 1000, Mode: "sum", ResetDay: 1},
		{Limit: 1, Unit: "GB", Base: 1001, Mode: "sum", ResetDay: 1},
		{Limit: 1, Unit: "GB", Base: 1000, Mode: "both", ResetDay: 1},
		{Limit: 1, Unit: "GB", Base: 1000, Mode: "sum", ResetDay: 32},
		{Limit: 1, Unit: "GB", Base: 1000, Mode: "sum", ResetDay: 1, Iface: "eth0; rm"},
		{Limit: 1, Unit: "GB", Base: 1000, Mode: "sum", ResetDay: 1, Thresholds: []int{0}},
	}
	for _, q := range bad {
		if q.Normalize() == nil {
			t.Errorf("应该不合法：%+v", q)
		}
	}
	q := Quota{Limit: 2, Unit: "TB", Base: 1024, Mode: "out", ResetDay: 1}
	if q.Normalize() != nil || q.LimitBytes() != 2*math.Pow(1024, 4) || q.FormatBytes(q.LimitBytes()) != "2.00 TiB" {
		t.Fatalf("换算不对：%v %s", q.LimitBytes(), q.FormatBytes(q.LimitBytes()))
	}
}

func TestUplinkIfaces(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"eth0", "wg0", "lo", "docker0", "veth1a2b"}, "eth0"},
		{[]string{"eno1", "vmbr0", "tap100i0"}, "eno1"},
		{[]string{"bond0", "eth0", "eth1"}, "bond0"},
		{[]string{"eth0", "eth0.100", "eth0@if3"}, "eth0"},
		{[]string{"ens5", "tailscale0", "tun0", "ifb0", "br0"}, "ens5"},
		{[]string{"eth0", "eth1"}, "eth0,eth1"},
		{[]string{"ppp0"}, "ppp0"}, // 全被排除时退回全部
		{[]string{"venet0"}, "venet0"},
	}
	for _, c := range cases {
		got := UplinkIfaces(c.in)
		var names []string
		for _, n := range c.in {
			if got[n] {
				names = append(names, n)
			}
		}
		if s := strings.Join(names, ","); s != c.want {
			t.Errorf("UplinkIfaces(%v) = %s，想要 %s", c.in, s, c.want)
		}
	}
}

// 隧道上的流量已经算在物理网卡里了：配额不能再算一遍；只有明确指定这块网卡时才算它。
func TestQuotaNoDoubleCount(t *testing.T) {
	s := testSampler()
	now := time.Now()
	day := now.Format(dayLayout)
	q := Quota{Limit: 1, Unit: "TB", Base: 1000, Mode: "sum", ResetDay: 1}
	q.Normalize()
	s.tr.quotas["web-1"] = q
	s.tr.counters["web-1"] = &hostCounters{up: 1000, ts: now.Unix(), ifaces: map[string]ifaceCounter{"eth0": {}, "wg0": {}}}
	s.tr.pending[dayKey{"web-1", day, "eth0"}] = &[2]float64{10e9, 10e9}
	s.tr.pending[dayKey{"web-1", day, "wg0"}] = &[2]float64{8e9, 8e9}
	if st := s.quotaStatusLocked("web-1", now); st.Used != 20e9 {
		t.Fatalf("隧道流量被重复算了：%v", st.Used)
	}
	q.Iface = "wg0"
	s.tr.quotas["web-1"] = q
	if st := s.quotaStatusLocked("web-1", now); st.Used != 16e9 {
		t.Fatalf("指定网卡时应该只算它：%v", st.Used)
	}
}
