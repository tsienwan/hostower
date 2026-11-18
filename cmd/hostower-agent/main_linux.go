//go:build linux

// hostower-agent 是可选的“增强采集”程序。
//
// 它不监听端口、不常驻、不需要任何配置：Hostower 通过已有的 SSH 连接启动它（hostower-agent stream），
// 它每隔几秒把一轮指标写到标准输出，连接一断它就退出。它只读 /proc、/sys 和文件系统用量，不改动主机上的任何东西。
//
// 输出格式和 Hostower 内置的采样脚本完全一样（十五段，用 --- 分隔，=== 结束一轮），后面多两段：
// 进程占用排行，和它自己的版本。所以装不装它，Hostower 那边的解析是同一套；它多出来的是脚本做不了的东西。
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// version 在构建时注入，和 Hostower 的版本一致。
var version = "dev"

const (
	interval = 5 * time.Second
	slowEach = 12 // 每十二轮（一分钟）输出一次变化慢的段落
	topN     = 15
	clockTck = 100 // Linux 上 USER_HZ 固定是 100
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "用法：hostower-agent stream | version")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("hostower-agent", version)
	case "stream":
		stream()
	default:
		fmt.Fprintln(os.Stderr, "不认识的子命令：", os.Args[1])
		os.Exit(2)
	}
}

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(b), "\n")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// self 记着自己这个文件的样子：被删掉或被换成新版本时退出，让 Hostower 重新连上来（用新的，或退回脚本）。
type self struct {
	path string
	info os.FileInfo
}

func (s *self) changed() bool {
	if s.path == "" || s.info == nil {
		return false
	}
	now, err := os.Stat(s.path)
	return err != nil || !os.SameFile(s.info, now) || now.ModTime() != s.info.ModTime() || now.Size() != s.info.Size()
}

func stream() {
	me := &self{}
	if p, err := os.Executable(); err == nil {
		if info, err := os.Stat(p); err == nil {
			me.path, me.info = p, info
		}
	}
	out := bufio.NewWriterSize(os.Stdout, 64<<10)
	procs := &procTable{prev: map[int]float64{}}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for i := 0; ; i++ {
		slow := i%slowEach == 0
		if slow && i > 0 && me.changed() {
			return
		}
		var b bytes.Buffer
		sec := func(s string) {
			if s != "" {
				b.WriteString(s)
				b.WriteByte('\n')
			}
			b.WriteString("---\n")
		}
		pids, top := procs.scan()
		sec(read("/proc/loadavg"))
		sec(read("/proc/meminfo"))
		sec(firstLine(read("/proc/stat")))
		sec(when(slow, diskUsage))
		sec(read("/proc/uptime"))
		sec(when(slow, func() string {
			return strings.TrimSpace(read("/proc/sys/kernel/ostype") + " " + read("/proc/sys/kernel/osrelease"))
		}))
		sec(when(slow, prettyName))
		sec(when(slow, cpuCount))
		sec(read("/proc/net/dev"))
		sec("")
		sec(when(slow, func() string { return pids }))
		sec(read("/proc/diskstats"))
		sec(temps())
		sec(containers())
		sec(when(slow, containerNames))
		sec(top)
		b.WriteString("agent " + version + "\n===\n")
		if _, err := out.Write(b.Bytes()); err != nil {
			return
		}
		if err := out.Flush(); err != nil {
			return // 对面断开了
		}
		<-tick.C
	}
}

func when(ok bool, f func() string) string {
	if !ok {
		return ""
	}
	return f()
}

func prettyName() string {
	for _, line := range strings.Split(read("/etc/os-release"), "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return line
		}
	}
	return ""
}

func cpuCount() string {
	n := 0
	for _, line := range strings.Split(read("/proc/stat"), "\n") {
		if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
			n++
		}
	}
	return strconv.Itoa(n)
}

// diskUsage 输出和 df -P -k 同样格式的几行：只列真实设备和根分区。
func diskUsage() string {
	var b strings.Builder
	b.WriteString("Filesystem 1024-blocks Used Available Capacity Mounted on")
	seen := map[string]bool{}
	for _, line := range strings.Split(read("/proc/self/mounts"), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		dev, mount := f[0], unescape(f[1])
		if seen[mount] || (mount != "/" && !strings.HasPrefix(dev, "/")) {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(mount, &st) != nil || st.Blocks == 0 {
			continue
		}
		seen[mount] = true
		bs := uint64(st.Bsize)
		size, free, avail := st.Blocks*bs/1024, st.Bfree*bs/1024, st.Bavail*bs/1024
		used := size - free
		pct := 0
		if used+avail > 0 {
			pct = int((used*100 + used + avail - 1) / (used + avail))
		}
		fmt.Fprintf(&b, "\n%s %d %d %d %d%% %s", dev, size, used, avail, pct, mount)
	}
	return b.String()
}

// unescape 还原 /proc/mounts 里的八进制转义（空格是 \040）。
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func temps() string {
	files, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	var out []string
	for _, f := range files {
		if v := read(f); v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, "\n")
}

// containers 从 cgroup 文件里读各容器的累计 CPU、内存和网络，和采样脚本的输出一行一行对应。
func containers() string {
	var out []string
	for _, pattern := range []string{
		"/sys/fs/cgroup/docker/*", "/sys/fs/cgroup/system.slice/docker-*.scope",
		"/sys/fs/cgroup/cpuacct/docker/*", "/sys/fs/cgroup/cpuacct/system.slice/docker-*.scope",
	} {
		dirs, _ := filepath.Glob(pattern)
		for _, d := range dirs {
			var usec, mem, inactive uint64
			if stat := read(d + "/cpu.stat"); stat != "" && read(d+"/memory.current") != "" {
				f := strings.Fields(firstLine(stat))
				if len(f) < 2 {
					continue
				}
				usec, _ = strconv.ParseUint(f[1], 10, 64)
				mem, _ = strconv.ParseUint(read(d+"/memory.current"), 10, 64)
				for _, line := range strings.Split(read(d+"/memory.stat"), "\n") {
					if v, ok := strings.CutPrefix(line, "inactive_file "); ok {
						inactive, _ = strconv.ParseUint(v, 10, 64)
						break
					}
				}
			} else if v := read(d + "/cpuacct.usage"); v != "" {
				ns, _ := strconv.ParseUint(v, 10, 64)
				usec = ns / 1000
				m := "/sys/fs/cgroup/memory/" + strings.TrimPrefix(d, "/sys/fs/cgroup/cpuacct/")
				mem, _ = strconv.ParseUint(read(m+"/memory.usage_in_bytes"), 10, 64)
			} else {
				continue
			}
			id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(d), "docker-"), ".scope")
			var rx, tx uint64
			if pid := firstLine(read(d + "/cgroup.procs")); pid != "" {
				for _, line := range strings.Split(read("/proc/"+pid+"/net/dev"), "\n") {
					name, rest, ok := strings.Cut(line, ":")
					f := strings.Fields(rest)
					if !ok || len(f) < 9 || strings.TrimSpace(name) == "lo" {
						continue
					}
					a, _ := strconv.ParseUint(f[0], 10, 64)
					b, _ := strconv.ParseUint(f[8], 10, 64)
					rx, tx = rx+a, tx+b
				}
			}
			out = append(out, fmt.Sprintf("%s %d %d %d %d %d", id, usec, mem, inactive, rx, tx))
		}
	}
	return strings.Join(out, "\n")
}

// containerNames 用 docker ps 取“编号 名字”的对照表。没有 docker 或没有权限时为空，Hostower 会显示短编号。
func containerNames() string {
	path, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	cmd := exec.Command(path, "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}")
	done := make(chan []byte, 1)
	go func() { b, _ := cmd.Output(); done <- b }()
	select {
	case b := <-done:
		return strings.TrimSpace(string(b))
	case <-time.After(4 * time.Second):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return ""
	}
}

// procTable 记着每个进程上一轮的累计 CPU 时间，用来算这一轮的占用。
type procTable struct {
	prev map[int]float64
	at   time.Time
}

type proc struct {
	pid  int
	name string
	cpu  float64 // 百分比，100 = 一个核
	rss  uint64  // 字节
}

// scan 扫一遍 /proc，返回进程号列表（每行一个）和占用排行（每行：进程号 CPU百分比 内存字节 名字）。
func (t *procTable) scan() (pids, top string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return "", ""
	}
	now := time.Now()
	elapsed := now.Sub(t.at).Seconds()
	page := uint64(os.Getpagesize())
	cur := make(map[int]float64, len(entries))
	var list []proc
	var ids strings.Builder
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ids.WriteString(e.Name())
		ids.WriteByte('\n')
		stat := read("/proc/" + e.Name() + "/stat")
		// 名字在括号里，可能含空格和括号：从第一个 ( 到最后一个 ) 之间。
		l, r := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
		if l < 0 || r < l {
			continue
		}
		f := strings.Fields(stat[r+1:])
		if len(f) < 22 {
			continue
		}
		utime, _ := strconv.ParseFloat(f[11], 64)
		stime, _ := strconv.ParseFloat(f[12], 64)
		rss, _ := strconv.ParseUint(f[21], 10, 64)
		ticks := utime + stime
		cur[pid] = ticks
		p := proc{pid: pid, name: stat[l+1 : r], rss: rss * page}
		if before, ok := t.prev[pid]; ok && elapsed > 0 && ticks >= before {
			p.cpu = (ticks - before) / clockTck / elapsed * 100
		}
		list = append(list, p)
	}
	t.prev, t.at = cur, now
	sort.Slice(list, func(i, j int) bool {
		if list[i].cpu != list[j].cpu {
			return list[i].cpu > list[j].cpu
		}
		return list[i].rss > list[j].rss
	})
	if len(list) > topN {
		list = list[:topN]
	}
	var b strings.Builder
	for i, p := range list {
		if i > 0 {
			b.WriteByte('\n')
		}
		// 名字放在最后：它可能含空格。
		fmt.Fprintf(&b, "%d %.1f %d %s", p.pid, p.cpu, p.rss, strings.ReplaceAll(p.name, "\n", " "))
	}
	return strings.TrimRight(ids.String(), "\n"), b.String()
}
