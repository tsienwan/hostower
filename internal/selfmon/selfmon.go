// Package selfmon 监测 Hostower 自己所在的这台机器：整机的 CPU、内存、磁盘、负载、网络，
// 以及 Hostower 进程本身的占用。数据直接在本机读取（Linux 上读 /proc），不经过任何执行器，
// 也不执行任何命令。
package selfmon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/metrics"
)

// Sample 是一次采样。百分比都是 0–100，速率是字节每秒。
type Sample struct {
	TS   int64   `json:"ts"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Load float64 `json:"load"`
	Rx   float64 `json:"rx"`
	Tx   float64 `json:"tx"`
}

// Status 是本机的当前状态。
type Status struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	NCPU     int    `json:"ncpu"`
	// System 表示拿得到整机指标（目前只有 Linux）。其他系统上只有磁盘和进程自身的数据。
	System bool `json:"system"`
	// Container 表示 Hostower 自己跑在容器里。这时 CPU、内存仍是整机的（同一个内核），
	// 但网络只看得到这个容器自己的流量，系统名是容器镜像的，磁盘是数据卷所在的那块。
	Container bool     `json:"container"`
	MemBytes  uint64   `json:"memBytes"`
	DiskPath  string   `json:"diskPath"`
	DiskTotal uint64   `json:"diskTotal"`
	DiskUsed  uint64   `json:"diskUsed"`
	Latest    *Sample  `json:"latest,omitempty"`
	History   []Sample `json:"history"`
	// 进程自身
	ProcBytes  uint64 `json:"procBytes"`
	Goroutines int    `json:"goroutines"`
	DBBytes    int64  `json:"dbBytes"`
	GoVersion  string `json:"goVersion"`
}

const historyLen = 120

// Monitor 定期采样本机。
type Monitor struct {
	dataDir string
	mu      sync.RWMutex
	status  Status
	history []Sample

	hasCPU      bool
	total, idle float64
	rx, tx      float64
	netAt       time.Time
}

// New 创建本机监测。dataDir 用来看数据目录所在磁盘的占用。
func New(dataDir string) *Monitor {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = dataDir
	}
	host, _ := os.Hostname()
	m := &Monitor{dataDir: abs}
	m.status = Status{Hostname: host, OS: osName(), Arch: runtime.GOARCH, NCPU: runtime.NumCPU(),
		System: runtime.GOOS == "linux", Container: inContainer(), DiskPath: abs, GoVersion: runtime.Version(), History: []Sample{}}
	return m
}

// Run 循环采样直到 ctx 结束。
func (m *Monitor) Run(ctx context.Context, interval time.Duration) {
	m.sample()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sample()
		}
	}
}

// Status 返回当前状态的副本。
func (m *Monitor) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.status
	s.History = append([]Sample{}, m.history...)
	if m.status.Latest != nil {
		l := *m.status.Latest
		s.Latest = &l
	}
	return s
}

// inContainer 用容器运行时放在根目录下的标记文件判断自己是不是在容器里（Docker 和 Podman）。
func inContainer() bool {
	for _, p := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func osName() string {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
				return strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
	}
	return runtime.GOOS
}

func (m *Monitor) sample() {
	now := time.Now()
	sm := Sample{TS: now.UnixMilli()}
	m.mu.Lock()
	defer m.mu.Unlock()

	first := true
	if m.status.System {
		if data, err := os.ReadFile("/proc/loadavg"); err == nil {
			sm.Load = parseLoad(string(data))
		}
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			total, avail := parseMeminfo(string(data))
			if total > 0 {
				sm.Mem = 100 * (total - avail) / total
				m.status.MemBytes = uint64(total) * 1024
			}
		}
		if data, err := os.ReadFile("/proc/stat"); err == nil {
			total, idle := parseCPU(string(data))
			if m.hasCPU && total > m.total {
				sm.CPU = 100 * (1 - (idle-m.idle)/(total-m.total))
				if sm.CPU < 0 {
					sm.CPU = 0
				}
				first = false
			}
			m.total, m.idle, m.hasCPU = total, idle, true
		}
		if data, err := os.ReadFile("/proc/net/dev"); err == nil {
			rx, tx := parseNet(string(data))
			if dt := now.Sub(m.netAt).Seconds(); !m.netAt.IsZero() && dt > 0 && rx >= m.rx && tx >= m.tx {
				sm.Rx, sm.Tx = (rx-m.rx)/dt, (tx-m.tx)/dt
			}
			m.rx, m.tx, m.netAt = rx, tx, now
		}
		if data, err := os.ReadFile("/proc/self/statm"); err == nil {
			if f := strings.Fields(string(data)); len(f) >= 2 {
				pages, _ := strconv.ParseUint(f[1], 10, 64)
				m.status.ProcBytes = pages * uint64(os.Getpagesize())
			}
		}
	}
	if m.status.ProcBytes == 0 || !m.status.System {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		m.status.ProcBytes = ms.Sys
	}
	if total, used, ok := diskUsage(m.dataDir); ok {
		m.status.DiskTotal, m.status.DiskUsed = total, used
	}
	m.status.Goroutines = runtime.NumGoroutine()
	var db int64
	for _, name := range []string{"hostower.db", "hostower.db-wal"} {
		if st, err := os.Stat(filepath.Join(m.dataDir, name)); err == nil {
			db += st.Size()
		}
	}
	m.status.DBBytes = db
	m.status.Latest = &sm
	// CPU 和网络速率要两次采样求差，第一次没有值，不画进趋势。
	if m.status.System && !first {
		m.history = append(m.history, sm)
		if len(m.history) > historyLen {
			m.history = m.history[len(m.history)-historyLen:]
		}
	}
}

// ---------- 解析 /proc 里的文本（单独成函数，方便测试） ----------

func parseLoad(s string) float64 {
	if f := strings.Fields(s); len(f) > 0 {
		v, _ := strconv.ParseFloat(f[0], 64)
		return v
	}
	return 0
}

// parseMeminfo 返回总内存和可用内存（kB）。
func parseMeminfo(s string) (total, avail float64) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "MemTotal:":
			total = v
		case "MemAvailable:":
			avail = v
		}
	}
	return
}

// parseCPU 从 /proc/stat 的第一行算出累计的总时间和空闲时间。
func parseCPU(s string) (total, idle float64) {
	line, _, _ := strings.Cut(s, "\n")
	f := strings.Fields(line)
	if len(f) < 5 || f[0] != "cpu" {
		return
	}
	for i, x := range f[1:] {
		v, _ := strconv.ParseFloat(x, 64)
		if i < 8 { // guest 时间已含在 user 里
			total += v
		}
		if i == 3 || i == 4 {
			idle += v
		}
	}
	return
}

// parseNet 把除回环以外所有网卡的收发字节加起来。
func parseNet(s string) (rx, tx float64) {
	for _, line := range strings.Split(s, "\n") {
		name, rest, ok := strings.Cut(line, ":")
		f := strings.Fields(rest)
		if !ok || len(f) < 9 || metrics.VirtualIface(strings.TrimSpace(name)) {
			continue
		}
		a, _ := strconv.ParseFloat(f[0], 64)
		b, _ := strconv.ParseFloat(f[8], 64)
		rx, tx = rx+a, tx+b
	}
	return
}
