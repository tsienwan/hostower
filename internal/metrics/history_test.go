package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/store"
)

func TestHistoryDownsamples(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSampler(nil, time.Second, db)
	st := &hostState{}
	// 过去 10 分钟，每 5 秒一个采样：CPU 在每一分钟里恒定为“第几分钟”。
	start := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	for i := 0; i < 120; i++ {
		ts := start.Add(time.Duration(i) * 5 * time.Second)
		s.accumulate("web-1", st, Sample{TS: ts.UnixMilli(), CPU: float64(i / 12), Mem: 50})
	}
	s.flush()
	got, err := s.History(context.Background(), "web-1", "1h")
	if err != nil {
		t.Fatal(err)
	}
	// 10 分钟的数据落成 9 个完整的分钟桶（最后一个桶还没结束，不写盘）。
	if len(got) != 9 {
		t.Fatalf("期望 9 个点，得到 %d", len(got))
	}
	for i, p := range got {
		if p.CPU != float64(i) || p.Mem != 50 {
			t.Errorf("第 %d 个点应是那一分钟的平均值：%+v", i, p)
		}
	}
	if other, _ := s.History(context.Background(), "web-2", "1h"); len(other) != 0 {
		t.Error("别的主机不应有数据")
	}
	if _, err := s.History(context.Background(), "web-1", "2y"); err == nil {
		t.Error("不支持的范围应报错")
	}
}

func TestRetention(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSampler(nil, time.Second, db)
	if s.Retention() != DefaultRetention {
		t.Fatalf("默认保留时长不对：%+v", s.Retention())
	}
	if err := s.SetRetention(Retention{FineDays: 5, CoarseDays: 35}); err == nil {
		t.Fatal("不在可选值里的应该被拒绝")
	}
	want := Retention{FineDays: 7, CoarseDays: 365}
	if err := s.SetRetention(want); err != nil {
		t.Fatal(err)
	}
	if again := NewSampler(nil, time.Second, db); again.Retention() != want {
		t.Fatalf("重启后应该读回保存的值：%+v", again.Retention())
	}

	// 分钟级保留 7 天时，“7 天”的图用分钟级数据；15 分钟级的点之外的旧数据会被清掉。
	now := time.Now().Unix()
	db.Exec(`INSERT INTO metrics (host, res, ts, cpu, mem, disk, load, swap, rx, tx, cache, load5, load15, dr, dw, temp) VALUES ('web-1', 60, ?, 1,0,0,0,0,0,0,0,0,0,0,0,0)`, now-3*24*3600)
	db.Exec(`INSERT INTO metrics (host, res, ts, cpu, mem, disk, load, swap, rx, tx, cache, load5, load15, dr, dw, temp) VALUES ('web-1', 60, ?, 1,0,0,0,0,0,0,0,0,0,0,0,0)`, now-9*24*3600)
	got, err := s.History(context.Background(), "web-1", "7d")
	if err != nil || len(got) != 1 {
		t.Fatalf("7 天的图应该用分钟级数据：%v %d", err, len(got))
	}
	s.flush()
	var n int
	db.QueryRow("SELECT COUNT(*) FROM metrics WHERE res = 60").Scan(&n)
	if n != 1 {
		t.Fatalf("超出保留时长的分钟级数据应该被清掉，还剩 %d 个", n)
	}
}
