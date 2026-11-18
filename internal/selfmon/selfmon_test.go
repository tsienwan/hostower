package selfmon

import (
	"testing"
)

func TestParsers(t *testing.T) {
	if got := parseLoad("0.42 0.36 0.30 1/123 4567\n"); got != 0.42 {
		t.Errorf("load = %v", got)
	}
	total, avail := parseMeminfo("MemTotal:        8000000 kB\nMemFree:          100000 kB\nMemAvailable:    6000000 kB\n")
	if total != 8000000 || avail != 6000000 {
		t.Errorf("meminfo = %v %v", total, avail)
	}
	// user nice system idle iowait irq softirq steal guest guest_nice
	ct, ci := parseCPU("cpu  100 0 50 800 50 0 0 0 30 0\ncpu0 1 2 3 4\n")
	if ct != 1000 || ci != 850 {
		t.Errorf("cpu = %v %v（guest 不应重复计入）", ct, ci)
	}
	rx, tx := parseNet("Inter-|   Receive  |  Transmit\n face |bytes packets|bytes packets\n    lo: 999 1 0 0 0 0 0 0 999 1 0 0 0 0 0 0\n  eth0: 1000 1 0 0 0 0 0 0 2000 1 0 0 0 0 0 0\n  eth1: 500 1 0 0 0 0 0 0 700 1 0 0 0 0 0 0\n")
	if rx != 1500 || tx != 2700 {
		t.Errorf("net = %v %v（回环不应计入）", rx, tx)
	}
}

func TestStatusAlwaysHasProcessData(t *testing.T) {
	m := New(t.TempDir())
	m.sample()
	s := m.Status()
	if s.ProcBytes == 0 || s.Goroutines == 0 || s.NCPU == 0 || s.Latest == nil {
		t.Fatalf("进程自身的数据在任何系统上都应该有: %+v", s)
	}
	if s.History == nil {
		t.Fatal("History 不应是 nil（前端按数组处理）")
	}
}
