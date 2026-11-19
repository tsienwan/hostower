// Package metrics 定期采样主机的少量指标，只够总览和健康判断用。
// 要完整监控请对接已有的监控系统；这里不重复存。
package metrics

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel"
)

// Sample 是一次采样。百分比都是 0–100。
type Sample struct {
	TS   int64   `json:"ts"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
	Load float64 `json:"load"`
	Swap float64 `json:"swap"`
	Rx   float64 `json:"rx"` // 入站字节每秒
	Tx   float64 `json:"tx"` // 出站字节每秒
	// Cache 是缓冲与缓存占内存的百分比（它包含在“可回收”里，不算进 Mem）。
	Cache  float64 `json:"cache"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
	DR     float64 `json:"dr"`   // 磁盘读，字节每秒
	DW     float64 `json:"dw"`   // 磁盘写，字节每秒
	Temp   float64 `json:"temp"` // 最高的传感器温度（摄氏度）；0 表示没有传感器
}

// Disk 是一个挂载的文件系统。
type Disk struct {
	Mount   string  `json:"mount"`
	Device  string  `json:"device"`
	TotalKB int64   `json:"totalKB"`
	Used    float64 `json:"used"` // 百分比
}

// ContainerStat 是一个容器的当前占用。
type ContainerStat struct {
	Name     string    `json:"name"`
	CPU      float64   `json:"cpu"`      // 百分比（100 = 一个核）
	MemBytes float64   `json:"memBytes"` // 已用内存
	Mem      float64   `json:"mem"`      // 占主机内存的百分比
	NetRx    float64   `json:"netRx"`    // 累计入站字节
	NetTx    float64   `json:"netTx"`    // 累计出站字节
	History  []float64 `json:"history,omitempty"`
}

// ProcStat 是进程排行里的一项（装了增强采集程序才有）。
type ProcStat struct {
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	CPU      float64 `json:"cpu"`      // 百分比（100 = 一个核）
	MemBytes float64 `json:"memBytes"` // 常驻内存
}

// Status 是一台主机的当前状态。
type Status struct {
	Host       string          `json:"host"`
	Online     bool            `json:"online"`
	Known      bool            `json:"known"` // 是否已经采过样
	Error      string          `json:"error,omitempty"`
	LastSeen   int64           `json:"lastSeen,omitempty"`
	OS         string          `json:"os,omitempty"`
	Kernel     string          `json:"kernel,omitempty"`
	NCPU       int             `json:"ncpu,omitempty"`
	MemKB      int64           `json:"memKB,omitempty"`
	DiskKB     int64           `json:"diskKB,omitempty"`
	UptimeSec  float64         `json:"uptimeSec,omitempty"`
	SwapKB     int64           `json:"swapKB,omitempty"`
	Disks      []Disk          `json:"disks,omitempty"`
	Containers []ContainerStat `json:"containers,omitempty"`
	Procs      int             `json:"procs,omitempty"`
	// Agent 是主机上增强采集程序的版本；空表示用的是内置脚本。
	Agent    string     `json:"agent,omitempty"`
	TopProcs []ProcStat `json:"topProcs,omitempty"`
	// Hardware 是 CPU 型号、整机型号、磁盘这些几乎不变的信息；连上主机后才有。
	Hardware *Hardware `json:"hardware,omitempty"`
	// Quota 是本周期的流量用量；没有设流量配额时为空。
	Quota   *QuotaStatus `json:"quota,omitempty"`
	Latest  *Sample      `json:"latest,omitempty"`
	History []Sample     `json:"history,omitempty"`
}

const historyLen = 90

type hostState struct {
	status      Status
	history     []Sample
	total, idle float64
	hasCPU      bool
	rx, tx      float64
	netAt       time.Time
	dr, dw      float64
	ctrHist     map[string][]float64
	ctrNext     time.Time          // 下次采容器占用的时间
	ctrBusy     bool               // 正在采容器占用
	ctrNative   bool               // 采样流里直接带着容器占用（读 cgroup）：这时不再另外跑 docker stats
	ctrNames    map[string]string  // 容器编号 → 名字
	ctrCPU      map[string]float64 // 容器编号 → 上一轮的累计 CPU 微秒
	upAt        float64            // 上一轮时主机的开机时长（秒）：用它算两轮之间真正隔了多久
	buckets     []bucket           // 各级还没写盘的桶
	hwNext      time.Time          // 下次读硬件信息的时间
	hwBusy      bool               // 正在读硬件信息
}

// Sampler 通过内核的只读采样入口取指标。
//
// 每台主机一条长连接：主机上跑着那段固定的只读脚本，每 5 秒输出一轮，采样器收到一轮就记一个点。
// 比起“每隔一会儿连上去跑一次命令”，这样主机上几乎不启动进程，中心这边也不用反复建会话；
// 速率类的指标（网络、磁盘读写）用主机自己的开机时长来算间隔，不受网络抖动影响。
type Sampler struct {
	k        *kernel.Kernel
	interval time.Duration
	mu       sync.RWMutex
	hosts    map[string]*hostState
	db       *sql.DB // 可以为 nil：不落盘
	pending  []row
	pruned   time.Time
	// retention 是历史的保留时长（受 mu 保护）。
	retention Retention
	tr        trafficState // 月流量（受 mu 保护）
}

// NewSampler 创建采样器。interval 是向页面推送和落盘的节奏；每台主机的采样间隔由脚本决定。
func NewSampler(k *kernel.Kernel, interval time.Duration, db *sql.DB) *Sampler {
	if interval <= 0 || interval > executor.StreamProbeInterval {
		interval = executor.StreamProbeInterval
	}
	s := &Sampler{k: k, interval: interval, hosts: map[string]*hostState{}, db: db, retention: loadRetention(db), tr: newTraffic()}
	s.loadTraffic()
	return s
}

// Run 给清单里的每台主机维持一条采样连接，直到 ctx 结束。清单变了（新增、删除主机）会跟着调整。
func (s *Sampler) Run(ctx context.Context) {
	running := map[string]context.CancelFunc{}
	defer func() {
		for _, cancel := range running {
			cancel()
		}
	}()
	t := time.NewTicker(s.interval)
	defer t.Stop()
	// 装上或卸掉增强采集程序之后，马上重连那台主机：脚本重新开始时第一轮就会检查采集程序，
	// 不用等它下一次每分钟的检查，页面上的状态和进程排行几秒内就跟上。
	events, unsub := s.k.Subscribe()
	defer unsub()
	handled := map[string]bool{} // 请求编号/主机：每台主机的每个请求只重连一次
	for {
		live := map[string]bool{}
		for _, h := range s.k.Inventory().Hosts() {
			live[h.Name] = true
			if _, ok := running[h.Name]; !ok {
				hctx, cancel := context.WithCancel(ctx)
				running[h.Name] = cancel
				go s.follow(hctx, h.Name)
			}
		}
		for name, cancel := range running {
			if !live[name] {
				cancel()
				delete(running, name)
				s.mu.Lock()
				delete(s.hosts, name)
				s.forgetCountersLocked(name)
				s.mu.Unlock()
			}
		}
		s.flush()
		s.k.Publish(kernel.Event{Type: "hosts", Data: s.All(false)})
		for waiting := true; waiting; {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				waiting = false
			case e := <-events:
				hosts := agentChanged(e, handled)
				if name, ok := e.Data.(string); ok && e.Type == "reconnected" {
					hosts = append(hosts, name) // 主机连接被重新建立：采样也换到新连接上
				}
				for _, name := range hosts {
					if cancel, ok := running[name]; ok {
						cancel()
						delete(running, name) // 下一轮立刻重新连上
						waiting = false
					}
				}
			}
		}
	}
}

// agentChanged 从内核事件里挑出“采集程序刚装上或卸掉”的主机（安装或卸载命令在这台主机上执行成功）。
func agentChanged(e kernel.Event, handled map[string]bool) []string {
	r, ok := e.Data.(*kernel.Request)
	if e.Type != "request" || !ok {
		return nil
	}
	var hosts []string
	for _, t := range r.Targets {
		if t.State != "ok" || (t.Command != executor.AgentInstallCommand && t.Command != executor.AgentRemoveCommand) {
			continue
		}
		if key := r.ID + "/" + t.Host; !handled[key] {
			handled[key] = true
			hosts = append(hosts, t.Host)
		}
	}
	return hosts
}

// follow 跟着一台主机的采样流；断了就记为离线，过一会儿重连（间隔逐步拉长到一分钟）。
func (s *Sampler) follow(ctx context.Context, name string) {
	wait := executor.StreamProbeInterval
	var quick time.Time // 上一次“断了立刻重连”的时间
	for ctx.Err() == nil {
		got, err := s.stream(ctx, name)
		if ctx.Err() != nil {
			return
		}
		// 刚才还好好的、突然断了（连接被重新建立、网络抖了一下）：先立刻重连一次，不急着记离线。
		if got && time.Since(quick) > 30*time.Second {
			quick = time.Now()
			continue
		}
		s.record(name, "", err)
		if got {
			wait = executor.StreamProbeInterval // 连上过，说明主机是好的：很快重试
		} else if wait < time.Minute {
			wait *= 2
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// stream 跑一次采样流，直到它断开。got 报告这期间有没有收到过完整的一轮。
func (s *Sampler) stream(ctx context.Context, name string) (got bool, err error) {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	last := time.Now()
	var lmu sync.Mutex
	w := &blockWriter{end: executor.StreamProbeEnd, emit: func(block string) {
		lmu.Lock()
		last, got = time.Now(), true
		lmu.Unlock()
		s.record(name, block, nil)
		s.containersIfDue(cctx, name)
		s.hardwareIfDue(cctx, name)
	}}
	// 看门狗：连接还在但迟迟没有新的一轮（主机卡住、网络半断），就当它断了。
	var stalled atomic.Bool
	go func() {
		t := time.NewTicker(executor.StreamProbeInterval)
		defer t.Stop()
		for {
			select {
			case <-cctx.Done():
				return
			case <-t.C:
				lmu.Lock()
				idle := time.Since(last)
				lmu.Unlock()
				if idle > 4*executor.StreamProbeInterval {
					stalled.Store(true)
					cancel()
					return
				}
			}
		}
	}()
	err = s.k.ProbeStream(cctx, name, w)
	if stalled.Load() {
		err = errors.New("采样没有响应")
	}
	lmu.Lock()
	defer lmu.Unlock()
	return got, err
}

// containersIfDue 到点了就另起一个协程采一次容器占用（docker stats 自己要跑一两秒，不能卡住采样流）。
func (s *Sampler) containersIfDue(ctx context.Context, name string) {
	s.mu.Lock()
	st := s.hosts[name]
	due := st != nil && !st.ctrNative && !st.ctrBusy && time.Now().After(st.ctrNext)
	if due {
		st.ctrBusy = true
	}
	s.mu.Unlock()
	if !due {
		return
	}
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		out, err := s.k.ProbeContainers(cctx, name)
		cancel()
		s.recordContainers(name, out, err)
		s.mu.Lock()
		if st := s.hosts[name]; st != nil {
			st.ctrBusy = false
		}
		s.mu.Unlock()
	}()
}

// blockWriter 把采样流按结束标记切成一轮一轮的。
type blockWriter struct {
	end  string
	emit func(block string)
	buf  []byte
}

func (w *blockWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	mark := []byte("\n" + w.end + "\n")
	for {
		i := bytes.Index(w.buf, mark)
		if i < 0 {
			break
		}
		w.emit(string(w.buf[:i]))
		w.buf = w.buf[i+len(mark):]
	}
	// 一轮正常只有几 KB；远超这个数说明对面不是我们的脚本，丢掉重来。
	if len(w.buf) > 1<<20 {
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func (s *Sampler) record(name, out string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.hosts[name]
	if st == nil {
		st = &hostState{status: Status{Host: name}}
		s.hosts[name] = st
	}
	st.status.Known = true
	if err != nil || out == "" {
		st.status.Online = false
		if err != nil {
			st.status.Error = err.Error()
		}
		if err == kernel.ErrPaused {
			st.status.Error = "执行已暂停，采样停止"
		}
		if err == nil {
			st.status.Error = "采样连接断开"
		}
		st.hasCPU = false
		st.netAt = time.Time{}
		st.upAt = 0
		st.ctrCPU = nil
		return
	}
	now := time.Now()
	first := !st.hasCPU
	sm := Sample{TS: now.UnixMilli()}
	// 按“单独一行 ---”分段。不能直接按 "\n---\n" 切：空段落连在一起时，相邻两条分隔线会共用一个换行。
	var sec []string
	var cur []string
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimRight(line, "\r") == "---" {
			sec = append(sec, strings.Join(cur, "\n"))
			cur = cur[:0]
		} else {
			cur = append(cur, line)
		}
	}
	sec = append(sec, strings.Join(cur, "\n"))
	get := func(i int) string {
		if i < len(sec) {
			return strings.TrimSpace(sec[i])
		}
		return ""
	}
	if f := strings.Fields(get(0)); len(f) > 0 {
		sm.Load, _ = strconv.ParseFloat(f[0], 64)
		if len(f) >= 3 {
			sm.Load5, _ = strconv.ParseFloat(f[1], 64)
			sm.Load15, _ = strconv.ParseFloat(f[2], 64)
		}
	}
	var memTotal, memAvail, memCache float64
	for _, line := range strings.Split(get(1), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseFloat(f[1], 64)
		switch f[0] {
		case "MemTotal:":
			memTotal = v
		case "Buffers:", "Cached:", "SReclaimable:":
			memCache += v
		case "MemAvailable:":
			memAvail = v
		}
	}
	if memTotal > 0 {
		sm.Mem = 100 * (memTotal - memAvail) / memTotal
		sm.Cache = 100 * memCache / memTotal
		st.status.MemKB = int64(memTotal)
	}
	if f := strings.Fields(get(2)); len(f) >= 5 && f[0] == "cpu" {
		var total, idle float64
		for i, x := range f[1:] {
			v, _ := strconv.ParseFloat(x, 64)
			if i < 8 { // guest 时间已含在 user 里
				total += v
			}
			if i == 3 || i == 4 {
				idle += v
			}
		}
		if st.hasCPU && total > st.total {
			sm.CPU = 100 * (1 - (idle-st.idle)/(total-st.total))
			if sm.CPU < 0 {
				sm.CPU = 0
			}
		} else if len(st.history) > 0 {
			sm.CPU = st.history[len(st.history)-1].CPU
		}
		st.total, st.idle, st.hasCPU = total, idle, true
	}
	// 两轮之间隔了多久：优先用主机自己的开机时长来算（不受网络延迟和抖动影响），没有就退回到收到的时间。
	up := 0.0
	if f := strings.Fields(get(4)); len(f) > 0 {
		up, _ = strconv.ParseFloat(f[0], 64)
	}
	elapsed := 0.0
	if st.upAt > 0 && up > st.upAt {
		elapsed = up - st.upAt
	} else if !st.netAt.IsZero() {
		elapsed = now.Sub(st.netAt).Seconds()
	}
	// 变化慢的段落（文件系统、系统信息、进程数）不是每轮都有：这一轮没有就沿用上一次的。
	slow := get(3) != ""
	if !slow && st.status.Latest != nil {
		sm.Disk = st.status.Latest.Disk
	}
	// 文件系统：根分区进趋势；其余真实设备（/dev/ 开头、非 loop）列在“分区”里。
	var disks []Disk
	seenDev := map[string]bool{}
	for _, line := range strings.Split(get(3), "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		mount := f[len(f)-1]
		// 根分区不看设备名：容器里是 overlay，ZFS 上是数据集名，都不以 / 开头。
		if mount != "/" && !strings.HasPrefix(f[0], "/") {
			continue
		}
		size, _ := strconv.ParseFloat(f[1], 64)
		used, _ := strconv.ParseFloat(f[2], 64)
		if size <= 0 {
			continue
		}
		if mount == "/" {
			sm.Disk = 100 * used / size
			st.status.DiskKB = int64(size)
		}
		// 分区列表：根分区一定列出；其余只列真实设备，并跳过容器里挂进 /etc 的单个文件。
		real := strings.HasPrefix(f[0], "/dev/") && !strings.HasPrefix(f[0], "/dev/loop") && !strings.HasPrefix(mount, "/etc/")
		if (mount == "/" || real) && !seenDev[f[0]] {
			seenDev[f[0]] = true
			disks = append(disks, Disk{Mount: mount, Device: f[0], TotalKB: int64(size), Used: 100 * used / size})
		}
	}
	sort.SliceStable(disks, func(i, j int) bool { return disks[i].Mount == "/" && disks[j].Mount != "/" })
	if slow {
		st.status.Disks = disks
	}
	if up > 0 {
		st.status.UptimeSec = up
	}
	if v := get(5); v != "" {
		st.status.Kernel = v
	}
	if v, ok := strings.CutPrefix(get(6), "PRETTY_NAME="); ok {
		st.status.OS = strings.Trim(v, `"'`)
	}
	if n, err := strconv.Atoi(get(7)); err == nil && n > 0 {
		st.status.NCPU = n
	}
	var rx, tx float64
	ifaces := map[string]ifaceCounter{}
	for _, line := range strings.Split(get(8), "\n") {
		iface, rest, ok := strings.Cut(line, ":")
		iface = strings.TrimSpace(iface)
		f := strings.Fields(rest)
		if !ok || len(f) < 9 || VirtualIface(iface) {
			continue
		}
		a, _ := strconv.ParseFloat(f[0], 64)
		b, _ := strconv.ParseFloat(f[8], 64)
		ifaces[iface] = ifaceCounter{RX: a, TX: b}
	}
	// 速率和月流量同一个口径：只算出口网卡，隧道、网桥、VLAN、绑定成员上的同一份流量不重复算（见 uplink.go）。
	for n := range UplinkIfaces(ifaceNames(ifaces)) {
		rx, tx = rx+ifaces[n].RX, tx+ifaces[n].TX
	}
	// 月流量按网卡的累计计数器记账（见 traffic.go），不靠速率积分：断线、面板重启期间的流量也能补上。
	if len(ifaces) > 0 && up > 0 {
		s.meterLocked(name, ifaces, up, now)
	}
	if dt := elapsed; dt > 0 && rx >= st.rx && tx >= st.tx {
		sm.Rx, sm.Tx = (rx-st.rx)/dt, (tx-st.tx)/dt
	}
	// 磁盘读写：只算整块设备（不重复算分区），扇区数乘 512。
	var dr, dw float64
	for _, line := range strings.Split(get(11), "\n") {
		f := strings.Fields(line)
		if len(f) < 10 || !wholeDisk.MatchString(f[2]) {
			continue
		}
		a, _ := strconv.ParseFloat(f[5], 64)
		b, _ := strconv.ParseFloat(f[9], 64)
		dr, dw = dr+a*512, dw+b*512
	}
	if dt := elapsed; dt > 0 && dr >= st.dr && dw >= st.dw {
		sm.DR, sm.DW = (dr-st.dr)/dt, (dw-st.dw)/dt
	}
	st.dr, st.dw = dr, dw
	for _, line := range strings.Fields(get(12)) {
		if v, err := strconv.ParseFloat(line, 64); err == nil && v/1000 > sm.Temp && v/1000 < 150 {
			sm.Temp = v / 1000
		}
	}
	st.rx, st.tx, st.netAt, st.upAt = rx, tx, now, up
	// 交换分区：单独的一段里有就用它，没有就从内存那一段里找（持续采样时整份 meminfo 都在那里）。
	var swapTotal, swapFree float64
	for _, line := range strings.Split(get(9)+"\n"+get(1), "\n") {
		if f := strings.Fields(line); len(f) >= 2 {
			v, _ := strconv.ParseFloat(f[1], 64)
			if f[0] == "SwapTotal:" {
				swapTotal = v
			} else if f[0] == "SwapFree:" {
				swapFree = v
			}
		}
	}
	st.status.SwapKB = int64(swapTotal)
	if swapTotal > 0 {
		sm.Swap = 100 * (swapTotal - swapFree) / swapTotal
	}
	procs := 0
	for _, name := range strings.Fields(get(10)) {
		if name[0] >= '0' && name[0] <= '9' {
			procs++
		}
	}
	if procs > 0 {
		st.status.Procs = procs
	}
	s.streamContainers(st, get(13), get(14), elapsed)
	// 增强采集程序在最后多两段：进程排行（进程号 CPU 内存 名字）和它自己的版本。
	if v, ok := strings.CutPrefix(get(16), "agent "); ok {
		st.status.Agent = strings.TrimSpace(v)
		var top []ProcStat
		for _, line := range strings.Split(get(15), "\n") {
			f := strings.SplitN(strings.TrimSpace(line), " ", 4)
			if len(f) < 4 {
				continue
			}
			p := ProcStat{Name: f[3]}
			p.PID, _ = strconv.Atoi(f[0])
			p.CPU, _ = strconv.ParseFloat(f[1], 64)
			p.MemBytes, _ = strconv.ParseFloat(f[2], 64)
			top = append(top, p)
		}
		st.status.TopProcs = top
	} else {
		st.status.Agent, st.status.TopProcs = "", nil
	}
	st.status.Online, st.status.Error, st.status.LastSeen = true, "", now.UnixMilli()
	st.status.Latest = &sm
	if first {
		return // CPU 和网络速率要两次采样求差，第一次没有值，不画进趋势
	}
	s.accumulate(name, st, sm)
	st.history = append(st.history, sm)
	if len(st.history) > historyLen {
		st.history = st.history[len(st.history)-historyLen:]
	}
}

var wholeDisk = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+|nvme\d+n\d+|mmcblk\d+)$`)

var sizeRe = regexp.MustCompile(`^([0-9.]+)\s*([kKMGT]?i?B)$`)

// parseSize 解析 docker 输出里的 "12.5MiB"、"1.2kB" 这类大小。
func parseSize(s string) float64 {
	m := sizeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	mult := map[string]float64{"B": 1, "kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12, "KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40}
	return v * mult[m[2]]
}

// recordContainers 解析容器占用。失败（没有 Docker、没有权限）时清空，并隔十分钟再试。
// streamContainers 解析采样流里直接带着的容器占用（从 cgroup 读出来的累计值），算出这一轮的 CPU 百分比。
// rows 每行是：编号、累计 CPU 微秒、内存字节、可回收的文件缓存、累计入站、累计出站；names 是“编号 名字”的对照表。
// 调用方已持有 s.mu。
func (s *Sampler) streamContainers(st *hostState, rows, names string, elapsed float64) {
	if names != "" {
		st.ctrNames = map[string]string{}
		for _, line := range strings.Split(names, "\n") {
			if f := strings.Fields(line); len(f) >= 2 {
				st.ctrNames[f[0]] = f[1]
			}
		}
	}
	if rows == "" {
		if st.ctrNative {
			// 之前有、现在一个都没有了：容器都停了。
			st.status.Containers, st.ctrHist, st.ctrCPU = nil, nil, nil
		}
		return
	}
	st.ctrNative = true
	cpuNow := map[string]float64{}
	hist := map[string][]float64{}
	var list []ContainerStat
	for _, line := range strings.Split(rows, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		num := func(i int) float64 { v, _ := strconv.ParseFloat(f[i], 64); return v }
		id, usec := f[0], num(1)
		cpuNow[id] = usec
		c := ContainerStat{Name: st.ctrNames[id], NetRx: num(4), NetTx: num(5)}
		if c.Name == "" {
			// 没有权限用 docker 命令时拿不到名字，用短编号代替。
			c.Name = id
			if len(c.Name) > 12 {
				c.Name = c.Name[:12]
			}
		}
		// 内存和 docker stats 的口径一致：去掉随时可以回收的文件缓存。
		if c.MemBytes = num(2) - num(3); c.MemBytes < 0 {
			c.MemBytes = num(2)
		}
		if st.status.MemKB > 0 {
			c.Mem = 100 * c.MemBytes / (float64(st.status.MemKB) * 1024)
		}
		if prev, ok := st.ctrCPU[id]; ok && elapsed > 0 && usec >= prev {
			c.CPU = 100 * (usec - prev) / (elapsed * 1e6)
		}
		h := append(st.ctrHist[c.Name], c.CPU)
		if len(h) > 60 {
			h = h[len(h)-60:]
		}
		hist[c.Name] = h
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CPU != list[j].CPU {
			return list[i].CPU > list[j].CPU
		}
		return list[i].Name < list[j].Name
	})
	st.status.Containers, st.ctrHist, st.ctrCPU = list, hist, cpuNow
}

func (s *Sampler) recordContainers(name, out string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.hosts[name]
	if st == nil || st.ctrNative {
		return
	}
	if err != nil {
		st.status.Containers, st.ctrHist = nil, nil
		st.ctrNext = time.Now().Add(10 * time.Minute)
		return
	}
	st.ctrNext = time.Now().Add(55 * time.Second)
	hist := map[string][]float64{}
	var list []ContainerStat
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimSpace(line), "|")
		if len(f) < 5 || f[0] == "" {
			continue
		}
		c := ContainerStat{Name: f[0]}
		c.CPU, _ = strconv.ParseFloat(strings.TrimSuffix(f[1], "%"), 64)
		c.Mem, _ = strconv.ParseFloat(strings.TrimSuffix(f[3], "%"), 64)
		if used, _, ok := strings.Cut(f[2], "/"); ok {
			c.MemBytes = parseSize(used)
		}
		if rx, tx, ok := strings.Cut(f[4], "/"); ok {
			c.NetRx, c.NetTx = parseSize(rx), parseSize(tx)
		}
		h := append(st.ctrHist[c.Name], c.CPU)
		if len(h) > 60 {
			h = h[len(h)-60:]
		}
		hist[c.Name] = h
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CPU > list[j].CPU })
	st.status.Containers, st.ctrHist = list, hist
}

// All 返回全部主机的状态。
func (s *Sampler) All(withHistory bool) map[string]Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Status, len(s.hosts))
	now := time.Now()
	for name, st := range s.hosts {
		c := st.status
		c.Quota = s.quotaStatusLocked(name, now)
		if st.status.Latest != nil {
			l := *st.status.Latest
			c.Latest = &l
		}
		c.Disks = append([]Disk(nil), st.status.Disks...)
		c.Containers = append([]ContainerStat(nil), st.status.Containers...)
		c.TopProcs = append([]ProcStat(nil), st.status.TopProcs...)
		if withHistory {
			c.History = append([]Sample(nil), st.history...)
			for i := range c.Containers {
				c.Containers[i].History = append([]float64(nil), st.ctrHist[c.Containers[i].Name]...)
			}
		}
		out[name] = c
	}
	return out
}
