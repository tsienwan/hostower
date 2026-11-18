package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// 指标历史：分两级落盘，旧数据自动降采样。
//   - 每分钟一个点，默认保留 1 天（可设 1～14 天）；
//   - 每 15 分钟一个点，默认保留 35 天（可设 7 天～2 年）。
// 每个点是那段时间里所有采样的平均值。只存总览和健康判断需要的这几项；
// 要更细的数据请对接已有的监控系统。

type tier struct {
	res  int64         // 桶宽（秒）
	keep time.Duration // 保留多久
}

// 两级的桶宽；保留多久由 Retention 决定。
const (
	fineRes   = 60
	coarseRes = 900
)

var resolutions = []int64{fineRes, coarseRes}

// Retention 是指标历史的保留时长（天）。
type Retention struct {
	FineDays   int `json:"fineDays"`   // 分钟级
	CoarseDays int `json:"coarseDays"` // 15 分钟级
}

// DefaultRetention 是没有设置过时的保留时长。
var DefaultRetention = Retention{FineDays: 1, CoarseDays: 35}

// 页面上可选的值。
var (
	FineChoices   = []int{1, 2, 3, 7, 14}
	CoarseChoices = []int{7, 14, 35, 90, 180, 365, 730}
)

// Valid 报告保留时长是不是可选的值之一。
func (r Retention) Valid() bool {
	in := func(v int, list []int) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	return in(r.FineDays, FineChoices) && in(r.CoarseDays, CoarseChoices)
}

// tiers 按当前的保留时长给出两级。分钟级多留两小时：“24 小时”的图要能从头画到尾。
func (r Retention) tiers() []tier {
	return []tier{{fineRes, time.Duration(r.FineDays)*24*time.Hour + 2*time.Hour}, {coarseRes, time.Duration(r.CoarseDays) * 24 * time.Hour}}
}

const retentionKey = "metrics.retention"

// loadRetention 从设置里读保留时长；没有或不合法时用默认值。
func loadRetention(db *sql.DB) Retention {
	r := DefaultRetention
	if db == nil {
		return r
	}
	var v string
	if db.QueryRow("SELECT value FROM settings WHERE key = ?", retentionKey).Scan(&v) == nil {
		var x Retention
		if json.Unmarshal([]byte(v), &x) == nil && x.Valid() {
			r = x
		}
	}
	return r
}

// Retention 返回当前的保留时长。
func (s *Sampler) Retention() Retention {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.retention
}

// SetRetention 保存新的保留时长。缩短时，下一次写盘就会清掉超出的部分。
func (s *Sampler) SetRetention(r Retention) error {
	if !r.Valid() {
		return errors.New("保留时长不是可选的值")
	}
	if s.db != nil {
		v, _ := json.Marshal(r)
		if _, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, retentionKey, string(v)); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.retention = r
	s.pruned = time.Time{}
	s.mu.Unlock()
	return nil
}

// Stored 返回数据库里存着的指标点数（两级合计）。
func (s *Sampler) Stored(ctx context.Context) (int64, error) {
	if s.db == nil {
		return 0, nil
	}
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM metrics").Scan(&n)
	return n, err
}

// bucket 是一个还没写盘的桶：累加，写盘时取平均。
type bucket struct {
	id  int64
	n   float64
	sum Sample
}

type row struct {
	host string
	res  int64
	s    Sample
}

func (b *bucket) add(s Sample) {
	b.n++
	b.sum.CPU += s.CPU
	b.sum.Mem += s.Mem
	b.sum.Disk += s.Disk
	b.sum.Load += s.Load
	b.sum.Swap += s.Swap
	b.sum.Rx += s.Rx
	b.sum.Tx += s.Tx
	b.sum.Cache += s.Cache
	b.sum.Load5 += s.Load5
	b.sum.Load15 += s.Load15
	b.sum.DR += s.DR
	b.sum.DW += s.DW
	b.sum.Temp += s.Temp
}

func (b *bucket) avg(res int64) Sample {
	n := b.n
	return Sample{TS: b.id * res * 1000, CPU: b.sum.CPU / n, Mem: b.sum.Mem / n, Disk: b.sum.Disk / n,
		Load: b.sum.Load / n, Swap: b.sum.Swap / n, Rx: b.sum.Rx / n, Tx: b.sum.Tx / n,
		Cache: b.sum.Cache / n, Load5: b.sum.Load5 / n, Load15: b.sum.Load15 / n, DR: b.sum.DR / n, DW: b.sum.DW / n, Temp: b.sum.Temp / n}
}

// accumulate 把一次采样记进各级的桶里；跨过桶边界时，把上一个桶交出来等待写盘。
// 调用方持有 s.mu。
func (s *Sampler) accumulate(host string, st *hostState, sm Sample) {
	if st.buckets == nil {
		st.buckets = make([]bucket, len(resolutions))
	}
	for i, res := range resolutions {
		t := tier{res: res}
		id := sm.TS / 1000 / t.res
		b := &st.buckets[i]
		if b.n > 0 && b.id != id {
			s.pending = append(s.pending, row{host, t.res, b.avg(t.res)})
			*b = bucket{}
		}
		b.id = id
		b.add(sm)
	}
}

// flush 把攒下的点一次写进数据库，并定期清掉过期的。
func (s *Sampler) flush() {
	if s.db == nil {
		return
	}
	s.mu.Lock()
	rows := s.pending
	s.pending = nil
	s.mu.Unlock()
	if len(rows) > 0 {
		tx, err := s.db.Begin()
		if err != nil {
			return
		}
		for _, r := range rows {
			tx.Exec(`INSERT OR REPLACE INTO metrics (host, res, ts, cpu, mem, disk, load, swap, rx, tx, cache, load5, load15, dr, dw, temp) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				r.host, r.res, r.s.TS/1000, r.s.CPU, r.s.Mem, r.s.Disk, r.s.Load, r.s.Swap, r.s.Rx, r.s.Tx, r.s.Cache, r.s.Load5, r.s.Load15, r.s.DR, r.s.DW, r.s.Temp)
		}
		tx.Commit()
	}
	s.mu.Lock()
	due := time.Since(s.pruned) > time.Hour
	if due {
		s.pruned = time.Now()
	}
	tiers := s.retention.tiers()
	s.mu.Unlock()
	if due {
		for _, t := range tiers {
			s.db.Exec("DELETE FROM metrics WHERE res = ? AND ts < ?", t.res, time.Now().Add(-t.keep).Unix())
		}
	}
}

// Ranges 是页面上可选的时间范围。
var Ranges = map[string]time.Duration{"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour, "1y": 365 * 24 * time.Hour}

const maxPoints = 360

// History 返回一台主机在一段时间里的指标，最多 maxPoints 个点。
// 一天以内用分钟级数据，更长的用 15 分钟级数据；点太多时再合并。
func (s *Sampler) History(ctx context.Context, host, rng string) ([]Sample, error) {
	span, ok := Ranges[rng]
	if !ok {
		return nil, fmt.Errorf("不支持的时间范围 %q", rng)
	}
	if s.db == nil {
		return []Sample{}, nil
	}
	// 分钟级数据覆盖得到这个范围就用它（更细），否则用 15 分钟级。
	tiers := s.Retention().tiers()
	t := tiers[0]
	if span > tiers[0].keep {
		t = tiers[1]
	}
	since := time.Now().Add(-span).Unix()
	rows, err := s.db.QueryContext(ctx, `SELECT ts, cpu, mem, disk, load, swap, rx, tx, cache, load5, load15, dr, dw, temp FROM metrics WHERE host = ? AND res = ? AND ts >= ? ORDER BY ts`, host, t.res, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// 合并成不超过 maxPoints 个点：每 step 秒一个桶。
	step := int64(span.Seconds()) / maxPoints
	if step < t.res {
		step = t.res
	}
	out := []Sample{}
	var cur bucket
	emit := func() {
		if cur.n > 0 {
			out = append(out, cur.avg(step))
		}
	}
	for rows.Next() {
		var ts int64
		var sm Sample
		if err := rows.Scan(&ts, &sm.CPU, &sm.Mem, &sm.Disk, &sm.Load, &sm.Swap, &sm.Rx, &sm.Tx, &sm.Cache, &sm.Load5, &sm.Load15, &sm.DR, &sm.DW, &sm.Temp); err != nil {
			return nil, err
		}
		if id := ts / step; cur.n > 0 && id != cur.id {
			emit()
			cur = bucket{id: id}
		} else {
			cur.id = id
		}
		cur.add(sm)
	}
	emit()
	return out, rows.Err()
}

var _ = sql.ErrNoRows
