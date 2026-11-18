package metrics

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/store"
)

// 根分区的设备名不一定以 / 开头：容器里是 overlay，ZFS 上是数据集名。
func TestRootDiskWithoutDevicePath(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSampler(nil, time.Second, db)
	df := strings.Join([]string{
		"Filesystem     1024-blocks     Used Available Capacity Mounted on",
		"overlay          1000000   360000    640000      36% /",
		"tmpfs                65536        0     65536       0% /dev",
		"/dev/sdd         1000000   360000    640000      36% /etc/hosts",
	}, "\n")
	sections := []string{
		"0.10 0.20 0.30 1/100 42",
		"MemTotal: 1000 kB\nMemAvailable: 500 kB",
		"cpu 1 0 1 10 0 0 0 0",
		df,
		"100.0 0", "Linux 6.6", `PRETTY_NAME="Test"`, "2", "", "", "", "", "",
	}
	s.record("box-1", strings.Join(sections, "\n---\n"), nil)
	got := s.All(false)["box-1"]
	if got.DiskKB != 1000000 {
		t.Fatalf("根分区容量 = %d，期望 1000000", got.DiskKB)
	}
	if got.Latest == nil || got.Latest.Disk < 35.9 || got.Latest.Disk > 36.1 {
		t.Fatalf("根分区占用 = %+v，期望 36%%", got.Latest)
	}
	// 分区列表里是根分区本身，而不是容器里挂进 /etc 的那个文件。
	if len(got.Disks) != 1 || got.Disks[0].Mount != "/" {
		t.Fatalf("分区列表 = %+v，期望只有根分区", got.Disks)
	}
}

// 持续采样：变化慢的段落不是每轮都有，没有的那一轮沿用上一次的值；速率按主机自己的开机时长算。
func TestStreamRounds(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSampler(nil, time.Second, db)
	net := func(rx int) string {
		return "Inter-|   Receive  |  Transmit\n face |bytes packets|bytes packets\n  eth0: " + strconv.Itoa(rx) + " 1 0 0 0 0 0 0 0 1 0 0 0 0 0 0"
	}
	full := []string{
		"0.10 0.20 0.30 1/100 42",
		"MemTotal: 1000 kB\nMemAvailable: 400 kB\nSwapTotal: 200 kB\nSwapFree: 150 kB",
		"cpu 100 0 100 800 0 0 0 0",
		"Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 1000 250 750 25% /",
		"1000.00 0", "Linux 6.6", `PRETTY_NAME="Test OS"`, "4", net(1000), "", "1\n2\n3\nself", "", "",
	}
	s.record("box-1", strings.Join(full, "\n---\n"), nil)
	// 第二轮：主机上过了 5 秒，但假装这条消息在路上耽搁了很久才到——速率仍然按 5 秒算。
	time.Sleep(30 * time.Millisecond)
	light := []string{
		"0.10 0.20 0.30 1/100 42",
		"MemTotal: 1000 kB\nMemAvailable: 400 kB\nSwapTotal: 200 kB\nSwapFree: 150 kB",
		"cpu 150 0 150 1100 0 0 0 0",
		"", "1005.00 0", "", "", "", net(6000), "", "", "", "",
	}
	// 按脚本真实的输出方式拼：空段落什么都不输出，于是分隔线一条紧挨着一条。
	var raw strings.Builder
	for i, sec := range light {
		if sec != "" {
			raw.WriteString(sec + "\n")
		}
		if i < len(light)-1 {
			raw.WriteString("---\n")
		}
	}
	s.record("box-1", raw.String(), nil)
	got := s.All(false)["box-1"]
	if got.OS != "Test OS" || got.Kernel != "Linux 6.6" || got.NCPU != 4 || got.Procs != 3 || got.DiskKB != 1000 || len(got.Disks) != 1 {
		t.Fatalf("慢的段落没有沿用：%+v", got)
	}
	l := got.Latest
	if l == nil || l.Disk != 25 {
		t.Fatalf("磁盘占用没有沿用：%+v", l)
	}
	if l.Rx < 999 || l.Rx > 1001 {
		t.Fatalf("网络速率应该是 (6000-1000)/5 = 1000 B/s，得到 %v", l.Rx)
	}
	if l.CPU < 24.9 || l.CPU > 25.1 {
		t.Fatalf("CPU 应该是 25%%，得到 %v", l.CPU)
	}
	if l.Swap != 25 || got.SwapKB != 200 {
		t.Fatalf("交换分区应该从内存那一段里读出来：%v %v", l.Swap, got.SwapKB)
	}
	if got.UptimeSec != 1005 {
		t.Fatalf("开机时长 = %v", got.UptimeSec)
	}
	// 连接断了：记为离线。
	s.record("box-1", "", nil)
	if got = s.All(false)["box-1"]; got.Online || got.Error == "" {
		t.Fatalf("断开后应该是离线：%+v", got)
	}
}

func TestBlockWriter(t *testing.T) {
	var blocks []string
	w := &blockWriter{end: "===", emit: func(b string) { blocks = append(blocks, b) }}
	for _, chunk := range []string{"a\nb\n=", "==\nc\n===\nd", "\n===\npartial"} {
		w.Write([]byte(chunk))
	}
	if len(blocks) != 3 || blocks[0] != "a\nb" || blocks[1] != "c" || blocks[2] != "d" {
		t.Fatalf("切出来的不对：%q", blocks)
	}
}

// 容器占用直接从采样流里来：CPU 用两轮的累计值相减，内存去掉可回收的缓存，名字来自对照表。
func TestStreamContainers(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSampler(nil, time.Second, db)
	round := func(up string, ctr, names string) string {
		sec := []string{"0.1 0.1 0.1 1/1 1", "MemTotal: 1000000 kB\nMemAvailable: 500000 kB", "cpu 1 0 1 8 0 0 0 0", "", up + " 0", "", "", "", "", "", "", "", "", ctr, names}
		var b strings.Builder
		for i, x := range sec {
			if x != "" {
				b.WriteString(x + "\n")
			}
			if i < len(sec)-1 {
				b.WriteString("---\n")
			}
		}
		return b.String()
	}
	const a, b = "aaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbb"
	s.record("box-1", round("100.00", a+" 1000000 209715200 104857600 10 20\n"+b+" 5000000 52428800 0 0 0", a+" web\n"), nil)
	// 主机上过了 5 秒：web 用了 2.5 秒 CPU（50%），另一个用了 0.5 秒（10%）。
	s.record("box-1", round("105.00", a+" 3500000 209715200 104857600 30 50\n"+b+" 5500000 52428800 0 0 0", ""), nil)
	got := s.All(true)["box-1"].Containers
	if len(got) != 2 || got[0].Name != "web" || got[1].Name != b[:12] {
		t.Fatalf("容器列表不对：%+v", got)
	}
	if got[0].CPU < 49.9 || got[0].CPU > 50.1 || got[1].CPU < 9.9 || got[1].CPU > 10.1 {
		t.Fatalf("CPU 不对：%v %v", got[0].CPU, got[1].CPU)
	}
	if got[0].MemBytes != 104857600 || got[0].NetRx != 30 || got[0].NetTx != 50 {
		t.Fatalf("内存或网络不对：%+v", got[0])
	}
	if got[0].Mem < 10.2 || got[0].Mem > 10.3 {
		t.Fatalf("内存占比应该约 10.24%%，得到 %v", got[0].Mem)
	}
	if len(got[0].History) != 2 {
		t.Fatalf("趋势应该有两个点：%v", got[0].History)
	}
	// 容器都停了：列表清空。
	s.record("box-1", round("110.00", "", ""), nil)
	if got = s.All(false)["box-1"].Containers; len(got) != 0 {
		t.Fatalf("容器停了之后应该清空：%+v", got)
	}
}

// 装了增强采集程序的主机：最后多出进程排行和版本两段；换回脚本后这两样消失。
func TestAgentSections(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewSampler(nil, time.Second, db)
	base := "0.1 0.1 0.1 1/1 1\n---\nMemTotal: 1000 kB\n---\ncpu 1 0 1 8 0 0 0 0\n---\n---\n10.00 0\n---\n---\n---\n---\n---\n---\n---\n---\n---\n---\n"
	s.record("box-1", base+"---\n42 12.5 1048576 nginx: worker process\n7 0.0 2048 sh\n---\nagent v1.2.3\n", nil)
	got := s.All(false)["box-1"]
	if got.Agent != "v1.2.3" || len(got.TopProcs) != 2 {
		t.Fatalf("采集程序的段落没有解析出来：%+v", got)
	}
	if p := got.TopProcs[0]; p.PID != 42 || p.CPU != 12.5 || p.MemBytes != 1048576 || p.Name != "nginx: worker process" {
		t.Fatalf("进程排行不对：%+v", p)
	}
	s.record("box-1", base, nil)
	if got = s.All(false)["box-1"]; got.Agent != "" || len(got.TopProcs) != 0 {
		t.Fatalf("换回脚本后应该清空：%+v", got)
	}
}

func TestAgentChanged(t *testing.T) {
	handled := map[string]bool{}
	req := &kernel.Request{ID: "rq_1", Targets: []*kernel.Target{
		{Host: "web-1", Command: executor.AgentInstallCommand, State: "ok"},
		{Host: "web-2", Command: executor.AgentInstallCommand, State: "failed"},
		{Host: "web-3", Command: "uptime", State: "ok"},
	}}
	got := agentChanged(kernel.Event{Type: "request", Data: req}, handled)
	if len(got) != 1 || got[0] != "web-1" {
		t.Fatalf("got %v", got)
	}
	// 同一个请求的后续事件不再重连。
	if got := agentChanged(kernel.Event{Type: "request", Data: req}, handled); len(got) != 0 {
		t.Fatalf("重复事件又重连了：%v", got)
	}
	rm := &kernel.Request{ID: "rq_2", Targets: []*kernel.Target{{Host: "web-1", Command: executor.AgentRemoveCommand, State: "ok"}}}
	if got := agentChanged(kernel.Event{Type: "request", Data: rm}, handled); len(got) != 1 {
		t.Fatalf("卸载没有触发重连：%v", got)
	}
	if got := agentChanged(kernel.Event{Type: "output", Data: rm}, handled); len(got) != 0 {
		t.Fatalf("非请求事件触发了重连：%v", got)
	}
}
