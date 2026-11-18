package executor

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/inventory"
)

// Sim 是模拟主机执行器：不连接任何真实机器，只用于开发模式、演示和测试。
// 它只在 --dev 下注册。
type Sim struct {
	mu    sync.Mutex
	hosts map[string]*simHost
	start time.Time
}

type simHost struct {
	seed            float64
	total, idle     float64
	last            time.Time
	files           map[string][]byte
	rx, tx, dr, dw  float64
	stopped         map[string]bool
	images          []string
	ncpu            int
	memKB, diskKB   int64
	diskUse, memUse float64
	os, kernel      string
	curlInstalled   bool               // 测速时自动装过 curl
	traceInstalled  bool               // 测速时自动装过 traceroute
	ooklaInstalled  bool               // 测速时下载过 Ookla 的程序
	apps            map[string]*simApp // 应用市场装过的实例
	netState        *simNet            // 防火墙和 Caddy
}

// hasCurl：模拟一部分主机上没有 curl，用来演示“先安装再测速”。
func (sh *simHost) hasCurl() bool { return sh.seed < 0.7 || sh.curlInstalled }

// hasTrace：模拟一部分主机上没有 traceroute 和 tracepath，用来演示顺便安装。
func (sh *simHost) hasTrace() bool { return sh.seed < 0.4 || sh.traceInstalled }

// NewSim 创建模拟执行器。
func NewSim() *Sim {
	return &Sim{hosts: map[string]*simHost{}, start: time.Now().Add(-37 * 24 * time.Hour)}
}

func (s *Sim) host(h inventory.Host) *simHost {
	sh, ok := s.hosts[h.Name]
	if ok {
		return sh
	}
	hf := fnv.New32a()
	hf.Write([]byte(h.Name))
	seed := float64(hf.Sum32()%1000) / 1000
	oses := []string{"Debian GNU/Linux 12 (bookworm)", "Ubuntu 24.04.1 LTS", "Rocky Linux 9.4 (Blue Onyx)", "Alpine Linux v3.20"}
	kernels := []string{"6.1.0-26-amd64", "6.8.0-48-generic", "5.14.0-427.el9.x86_64", "6.6.58-0-lts"}
	idx := int(seed*1000) % len(oses)
	sh = &simHost{
		seed:    seed,
		last:    time.Now(),
		ncpu:    []int{2, 4, 8, 16}[int(seed*977)%4],
		memKB:   []int64{2, 4, 8, 16, 32}[int(seed*613)%5] * 1024 * 1024,
		diskKB:  []int64{40, 80, 160, 320}[int(seed*389)%4] * 1024 * 1024,
		diskUse: 0.18 + 0.6*math.Mod(seed*7.3, 1),
		memUse:  0.25 + 0.5*math.Mod(seed*3.7, 1),
		os:      oses[idx],
		kernel:  kernels[idx],
	}
	sh.files = map[string][]byte{
		"/etc/hostname":            []byte(h.Name + "\n"),
		"/etc/os-release":          []byte(fmt.Sprintf("PRETTY_NAME=%q\nID=linux\n", sh.os)),
		"/etc/hosts":               []byte("127.0.0.1 localhost\n::1 localhost\n"),
		"/etc/motd":                []byte("模拟主机：这里的一切都是假的。\n"),
		"/etc/nginx/nginx.conf":    []byte("worker_processes auto;\n\nevents {\n    worker_connections 1024;\n}\n\nhttp {\n    server {\n        listen 80;\n        server_name example.com;\n        root /var/www/html;\n    }\n}\n"),
		"/opt/stack/compose.yaml":  []byte("services:\n  web:\n    image: nginx:1.27\n    ports: [\"80:80\"]\n  app:\n    image: example/app:2.4.1\n    restart: unless-stopped\n"),
		"/opt/stack/.env.example":  []byte("APP_ENV=production\nLOG_LEVEL=info\n"),
		"/var/log/syslog":          []byte("Nov 13 09:00:01 " + h.Name + " CRON[812]: (root) CMD (run-parts /etc/cron.hourly)\nNov 13 09:17:42 " + h.Name + " systemd[1]: Started Daily apt download activities.\n"),
		"/var/www/html/index.html": []byte("<!doctype html>\n<title>example</title>\n<h1>It works</h1>\n"),
		"/home/ops/README":         []byte("ops 账号的家目录。\n"),
	}
	s.hosts[h.Name] = sh
	return sh
}

func (s *Sim) offline(h inventory.Host) error {
	if h.Vars["sim_state"] == "offline" {
		return fmt.Errorf("连接 %s 失败：连接超时", h.Addr)
	}
	return nil
}

// util 返回 0..1 的 CPU 利用率，随时间平滑波动。
func (sh *simHost) util(t time.Time) float64 {
	x := float64(t.UnixMilli()) / 1000
	u := 0.08 + 0.35*sh.seed +
		0.12*math.Sin(x/47+sh.seed*20) +
		0.08*math.Sin(x/11+sh.seed*50) +
		0.04*math.Sin(x/3.1+sh.seed*90)
	if sh.seed > 0.82 {
		u += 0.35
	}
	return math.Max(0.01, math.Min(0.99, u))
}

func (s *Sim) Exec(ctx context.Context, h inventory.Host, command string, out io.Writer) (int, error) {
	if err := s.offline(h); err != nil {
		return -1, err
	}
	s.mu.Lock()
	sh := s.host(h)
	s.mu.Unlock()

	if command == StreamProbeCommand {
		// 持续采样：每个间隔输出一轮，直到被取消。
		for {
			if _, err := s.Exec(ctx, h, ProbeCommand, out); err != nil {
				return -1, err
			}
			io.WriteString(out, "\n"+StreamProbeEnd+"\n")
			select {
			case <-ctx.Done():
				return -1, ctx.Err()
			case <-time.After(StreamProbeInterval):
			}
		}
	}
	if command == ProbeCommand {
		s.mu.Lock()
		defer s.mu.Unlock()
		now := time.Now()
		dt := now.Sub(sh.last).Seconds()
		sh.last = now
		u := sh.util(now)
		sh.total += dt * 100 * float64(sh.ncpu)
		sh.idle += dt * 100 * float64(sh.ncpu) * (1 - u)
		load := u * float64(sh.ncpu)
		mem := sh.memUse + 0.04*math.Sin(float64(now.Unix())/90+sh.seed*10)
		used := int64(float64(sh.diskKB) * sh.diskUse)
		fmt.Fprintf(out, "%.2f %.2f %.2f 2/312 4711\n---\n", load, load*0.9, load*0.8)
		fmt.Fprintf(out, "MemTotal: %d kB\nMemAvailable: %d kB\nBuffers: %d kB\nCached: %d kB\nSReclaimable: %d kB\n---\n", sh.memKB, int64(float64(sh.memKB)*(1-mem)), sh.memKB/50, int64(float64(sh.memKB)*0.22), sh.memKB/40)
		fmt.Fprintf(out, "cpu  %d 0 0 %d 0 0 0 0 0 0\n---\n", int64(sh.total-sh.idle), int64(sh.idle))
		fmt.Fprintf(out, "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/vda1 %d %d %d %d%% /\ntmpfs 1024000 0 1024000 0%% /dev/shm\n/dev/vda15 126710 11800 114910 10%% /boot/efi\n/dev/vdb1 %d %d %d %d%% /data\n---\n", sh.diskKB, used, sh.diskKB-used, int(sh.diskUse*100), sh.diskKB*2, sh.diskKB/3, sh.diskKB*2-sh.diskKB/3, 16)
		fmt.Fprintf(out, "%.2f 0.00\n---\nLinux %s\n---\nPRETTY_NAME=%q\n---\n%d\n---\n", now.Sub(s.start).Seconds()*(0.3+sh.seed), sh.kernel, sh.os, sh.ncpu)
		sh.rx += dt * (40e3 + 9e6*u*sh.seed)
		sh.tx += dt * (25e3 + 4e6*u*(1-sh.seed))
		fmt.Fprintf(out, "Inter-|   Receive  |  Transmit\n face |bytes packets|bytes packets\n    lo: 1000 10 0 0 0 0 0 0 1000 10 0 0 0 0 0 0\n  eth0: %d 100 0 0 0 0 0 0 %d 100 0 0 0 0 0 0\n---\n", int64(sh.rx), int64(sh.tx))
		fmt.Fprintf(out, "SwapTotal: %d kB\nSwapFree: %d kB\n---\n", sh.memKB/4, int64(float64(sh.memKB/4)*(1-0.3*sh.seed)))
		for i := 0; i < 90+int(sh.seed*140); i++ {
			fmt.Fprintf(out, "%d\n", 1+i*37)
		}
		fmt.Fprint(out, "cpuinfo\nmeminfo\nstat\n---\n")
		sh.dr += dt * (2e3 + 3e6*u*sh.seed) / 512
		sh.dw += dt * (40e3 + 9e6*u*(1-sh.seed)) / 512
		fmt.Fprintf(out, " 254 0 vda 1000 0 %d 100 2000 0 %d 300 0 400 400\n 254 1 vda1 900 0 1 90 1900 0 1 290 0 380 380\n   7 0 loop0 5 0 40 1 0 0 0 0 0 1 1\n---\n", int64(sh.dr), int64(sh.dw))
		if sh.seed > 0.35 {
			fmt.Fprintf(out, "%d\n%d\n", int(38000+22000*u), int(34000+15000*u))
		}
		return 0, nil
	}

	if command == HardwareProbeCommand {
		// 模拟几种常见的机器：云主机（KVM）、物理服务器、ARM 小机器。
		cpus := []struct {
			vendor, model, hyper, sys, product string
			mhz                                float64
			l3                                 string
		}{
			{"AuthenticAMD", "AMD EPYC 7B13", "KVM", "Google", "Google Compute Engine", 0, "32 MiB (1 instance)"},
			{"GenuineIntel", "Intel(R) Xeon(R) Platinum 8259CL CPU @ 2.50GHz", "KVM", "Amazon EC2", "c5.xlarge", 3500, "35.8 MiB (1 instance)"},
			{"GenuineIntel", "Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz", "", "Dell Inc.", "PowerEdge R650", 3200, "48 MiB (1 instance)"},
			{"ARM", "Neoverse-N1", "KVM", "QEMU", "KVM Virtual Machine", 0, ""},
		}
		c := cpus[int(sh.seed*float64(len(cpus)))%len(cpus)]
		threads := 1
		if c.hyper == "" && sh.ncpu >= 4 {
			threads = 2
		}
		fmt.Fprintf(out, "Architecture:            %s\nCPU(s):                  %d\nVendor ID:               %s\nModel name:              %s\nThread(s) per core:      %d\nCore(s) per socket:      %d\nSocket(s):               1\n",
			map[bool]string{true: "aarch64", false: "x86_64"}[c.vendor == "ARM"], sh.ncpu, c.vendor, c.model, threads, sh.ncpu/threads)
		if c.mhz > 0 {
			fmt.Fprintf(out, "CPU max MHz:             %.4f\n", c.mhz)
		}
		if c.l3 != "" {
			fmt.Fprintf(out, "L3 cache:                %s\n", c.l3)
		}
		if c.hyper != "" {
			fmt.Fprintf(out, "Hypervisor vendor:       %s\nVirtualization type:     full\n", c.hyper)
		}
		fmt.Fprintf(out, "---\nmodel name\t: %s\n---\nsys_vendor=%s\nproduct_name=%s\n---\nvda %d 1 \n", c.model, c.sys, c.product, sh.diskKB*2)
		if c.hyper == "" {
			fmt.Fprintf(out, "nvme0n1 %d 0 SAMSUNG MZQL2960HCJR-00A07\nsda %d 1 ST8000NM000A-2KE101\n", int64(1920383410), int64(15628053168))
		} else {
			fmt.Fprintf(out, "vdb %d 1 \n", sh.diskKB*4)
		}
		return 0, nil
	}

	s.mu.Lock()
	lines, code := s.canned(h, sh, command)
	if command == ContainerProbeCommand {
		lines, code = nil, 1
		if h.HasTag("docker") {
			u, code0 := sh.util(time.Now()), 0
			for i, c := range []string{"stack-web-1", "stack-app-1", "stack-cache-1"} {
				if sh.stopped[c] {
					continue
				}
				cpu := u * 100 * []float64{0.18, 0.55, 0.07}[i]
				mem := []float64{24, 310, 46}[i] * (1 + 0.1*u)
				lines = append(lines, fmt.Sprintf("%s|%.2f%%|%.1fMiB / %.2fGiB|%.2f%%|%.1fMB / %.1fMB", c, cpu, mem, float64(sh.memKB)/1048576, mem*1024*100/float64(sh.memKB), 120+40*float64(i), 60+25*float64(i)))
			}
			code = code0
		}
	}
	s.mu.Unlock()
	for i, line := range lines {
		if i > 0 && len(lines) > 3 {
			select {
			case <-ctx.Done():
				return -1, ctx.Err()
			case <-time.After(time.Duration(40+len(line)%120) * time.Millisecond):
			}
		}
		io.WriteString(out, line+"\n")
	}
	return code, nil
}

func (s *Sim) canned(h inventory.Host, sh *simHost, command string) ([]string, int) {
	cmd := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(command), "sudo "))
	gb := func(kb int64) string { return fmt.Sprintf("%.0fG", float64(kb)/1024/1024) }
	if lines, code, ok := s.netCommand(h, sh, cmd); ok {
		return lines, code
	}
	if lines, code, ok := s.appCommand(h, sh, cmd); ok {
		return lines, code
	}
	switch {
	case strings.HasPrefix(cmd, "if command -v docker >/dev/null 2>&1; then echo docker; fi;"):
		// Docker 检查（apps.DockerStatusCommand）：带 docker 标签的模拟主机算装好了；其余是一台能装的 Debian。
		out := []string{"root", "has apt-get", "has systemctl", "has curl", "arch amd64", "os ID=debian", "os VERSION_CODENAME=bookworm"}
		if h.HasTag("docker") {
			out = append([]string{"docker", "compose"}, out...)
		}
		return out, 0
	case cmd == "command -v curl":
		if sh.hasCurl() {
			return []string{"/usr/bin/curl"}, 0
		}
		return nil, 1
	case strings.HasPrefix(cmd, "for c in curl traceroute tracepath;"):
		var out []string
		if sh.hasCurl() {
			out = append(out, "/usr/bin/curl")
		}
		if sh.hasTrace() {
			out = append(out, "/usr/sbin/traceroute")
		}
		if len(out) == 0 {
			return nil, 1
		}
		return out, 0
	case (strings.HasPrefix(cmd, "if ! curl --version") || strings.HasPrefix(cmd, "if ! command -v curl") || strings.Contains(cmd, "ookla-speedtest-")) && strings.Contains(cmd, "speedtest "):
		var pre []string
		ookla := strings.Contains(cmd, "ookla-speedtest-")
		if !sh.hasCurl() && !(ookla && (sh.ooklaInstalled || sh.seed > 0.8)) {
			if !strings.HasPrefix(cmd, "if ! command -v curl") {
				return []string{"speedtest error=no-curl"}, 0
			}
			sh.curlInstalled = true
			pre = []string{"speedtest installing curl", "Reading package lists...", "Setting up curl (7.88.1-10+deb12u8) ..."}
		}
		if !sh.hasTrace() && strings.Contains(cmd, "speedtest installing traceroute") {
			sh.traceInstalled = true
			pre = append(pre, "speedtest installing traceroute", "Setting up traceroute (1:2.1.2-1) ...")
		}
		// 测速：按主机给出一组稳定但各不相同的数字。
		ping := 8 + sh.seed*180
		down := (20 + (1-sh.seed)*900) * 1e6 / 8
		out := pre
		if !ookla {
			out = append(out, "speedtest trace ip=203.0.113."+strconv.Itoa(10+int(sh.seed*200)), "speedtest trace colo=SJC",
				"speedtest trace loc=US", "speedtest trace tls=TLSv1.3")
			for i, k := range []float64{1.1, 1, 1.05, 1.02, 1.08} {
				c := 0.001 + ping/1000*k
				out = append(out, fmt.Sprintf("speedtest ping dns=0.00%d connect=%.4f tls=%.4f ttfb=%.4f", 1+i%2, c, c+ping/1000*1.02, c+ping/1000*1.4))
			}
		}
		// 路由追踪：命令里有几个目标就给几条。第二个起演示经过骨干网段的回程（这些是公开的骨干网段，用来演示线路识别）。
		demo := [][]string{
			{"59.43.182.1", "59.43.180.6"},  // CN2 GIA hygiene:allow
			{"218.105.2.1", "218.105.9.33"}, // 联通 9929 hygiene:allow
			{"223.120.6.1", "221.183.55.2"}, // 移动 CMI hygiene:allow
		}
		if m := regexp.MustCompile(`for t in((?: '[^']*')+); do`).FindStringSubmatch(cmd); m != nil {
			for j, f := range strings.Fields(m[1]) {
				t := strings.Trim(f, "'")
				if !sh.hasTrace() {
					out = append(out, "speedtest route "+t+" error=no-traceroute")
					continue
				}
				hops := []string{"10.0.0.1", "192.0.2.1", "198.51.100.33"} // 内网网关（私有地址） hygiene:allow
				if j > 0 {
					hops = append(hops, demo[(j-1)%len(demo)]...)
					hops = append(hops, "*")
				}
				// traceroute -n 打印的是地址：目标是域名时最后一跳给一个示例地址。
				last := t
				if strings.Trim(t, "0123456789.:") != "" {
					last = "203.0.113.5"
				}
				hops = append(hops, last)
				out = append(out, "speedtest route "+t+" traceroute to "+t+", 30 hops max, 60 byte packets")
				for n, h := range hops {
					if h == "*" {
						out = append(out, fmt.Sprintf("speedtest route %s %2d  *", t, n+1))
						continue
					}
					out = append(out, fmt.Sprintf("speedtest route %s %2d  %s  %.3f ms", t, n+1, h, 0.4+ping*float64(n)/float64(len(hops))))
				}
			}
		}
		up := down * (0.3 + sh.seed*0.5)
		// 一部分主机演示“没有 IPv6”：Ookla 找不到 IPv6 出口地址，Cloudflare 连不上（没有下载和上传的结果）。
		if (strings.Contains(cmd, "ip -6 route get") || strings.Contains(cmd, "curl -6 ")) && sh.seed > 0.5 {
			if ookla {
				return append(out, "speedtest error=no-ipv6"), 0
			}
			return out, 0
		}
		if ookla {
			// Ookla 的程序：第一次先“下载”，然后给一行 JSON 结果（数据量随带宽变）。
			// 一部分主机演示“系统里已经装了官方的 speedtest”。
			if sh.seed > 0.8 {
				out = append(out, "speedtest version Speedtest by Ookla 1.2.0.84 (5ed00a8b2d) Linux/x86_64-linux-musl 6.1.0 x86_64", "speedtest using=system") // 版本号，不是地址 hygiene:allow
			} else {
				if !sh.ooklaInstalled {
					sh.ooklaInstalled = true
					out = append(out, "speedtest installing ookla")
				}
				out = append(out, "speedtest using=pinned")
			}
			server, name, loc := 0, "Example Networks", "San Jose, CA"
			if m := regexp.MustCompile(`-f json -p no -s ([0-9]+) `).FindStringSubmatch(cmd); m != nil {
				server, _ = strconv.Atoi(m[1])
				name, loc = "Example Telecom", "Shanghai"
			}
			if server == 0 {
				server = 10000 + int(sh.seed*5000)
			}
			loss := 0.0
			if sh.seed > 0.6 {
				loss = 0.4
			}
			out = append(out, fmt.Sprintf(`speedtest ookla {"type":"result","timestamp":"%s","ping":{"jitter":%.3f,"latency":%.3f,"low":%.3f,"high":%.3f},`+
				`"download":{"bandwidth":%.0f,"bytes":%.0f,"elapsed":10012,"latency":{"iqm":%.3f,"low":%.3f,"high":%.3f,"jitter":2.1}},`+
				`"upload":{"bandwidth":%.0f,"bytes":%.0f,"elapsed":9010,"latency":{"iqm":%.3f,"low":%.3f,"high":%.3f,"jitter":3.4}},"packetLoss":%.1f,"isp":"Example Hosting",`+
				`"interface":{"internalIp":"192.0.2.10","name":"eth0","macAddr":"02:00:00:00:00:01","isVpn":false,"externalIp":"203.0.113.%d"},`+
				`"server":{"id":%d,"host":"speedtest.example.com","port":8080,"name":"%s","location":"%s","country":"Example","ip":"198.51.100.%d"},`+
				`"result":{"id":"00000000-0000-4000-8000-%012d","url":"https://www.speedtest.net/result/c/00000000-0000-4000-8000-%012d","persisted":true}}`,
				time.Now().UTC().Format(time.RFC3339), ping*0.04, ping, ping*0.95, ping*1.2,
				down, down*10, ping*2.5, ping, ping*4, up, up*9, ping*3, ping, ping*5, loss,
				10+int(sh.seed*200), server, name, loc, 20+int(sh.seed*100), server, server))
			return out, 0
		}
		// 多线程：四条连接各传四分之一，合起来比单条连接快一些。
		if strings.Contains(cmd, "& done; wait") {
			for i := 0; i < 4; i++ {
				d := down * 0.3 * (1 + float64(i)*0.05)
				out = append(out, fmt.Sprintf("speedtest down speed=%.0f bytes=6250000 time=%.3f ttfb=%.4f ip=203.0.113.5 http=2", d, 6250000/d, ping/1000*1.5))
			}
			for i := 0; i < 4; i++ {
				u := up * 0.3 * (1 + float64(i)*0.05)
				out = append(out, fmt.Sprintf("speedtest up speed=%.0f bytes=1562500 time=%.3f", u, 1562500/u))
			}
			return out, 0
		}
		return append(out,
			fmt.Sprintf("speedtest down speed=%.0f bytes=25000000 time=%.3f ttfb=%.4f ip=203.0.113.5 http=2", down, 25000000/down, ping/1000*1.5),
			fmt.Sprintf("speedtest up speed=%.0f bytes=6250000 time=%.3f", up, 6250000/up),
		), 0
	case strings.HasPrefix(cmd, "df"):
		used := int64(float64(sh.diskKB) * sh.diskUse)
		return []string{
			"Filesystem      Size  Used Avail Use% Mounted on",
			fmt.Sprintf("/dev/vda1       %4s  %4s  %4s  %2d%% /", gb(sh.diskKB), gb(used), gb(sh.diskKB-used), int(sh.diskUse*100)),
			"tmpfs           1.0G     0  1.0G   0% /dev/shm",
			"/dev/vda15      124M   12M  113M  10% /boot/efi",
		}, 0
	case strings.HasPrefix(cmd, "uptime"):
		u := sh.util(time.Now()) * float64(sh.ncpu)
		return []string{fmt.Sprintf(" %s up 37 days,  4:12,  1 user,  load average: %.2f, %.2f, %.2f", time.Now().Format("15:04:05"), u, u*0.9, u*0.8)}, 0
	case strings.HasPrefix(cmd, "free"):
		t := sh.memKB / 1024
		used := int64(float64(t) * sh.memUse)
		return []string{
			"               total        used        free      shared  buff/cache   available",
			fmt.Sprintf("Mem:        %8d    %8d    %8d          12    %8d    %8d", t, used, (t-used)/3, (t-used)*2/3, t-used),
			"Swap:              0           0           0",
		}, 0
	case strings.HasPrefix(cmd, "docker ") && !h.HasTag("docker"):
		return []string{"bash: docker: command not found"}, 127
	case strings.HasPrefix(cmd, "docker ps -a --format '{{.Names}}	{{.Image}}"):
		// 容器页的更新检测：容器和镜像，再加本机镜像的摘要（模拟的摘要和仓库里的不一样）。
		out := []string{"stack-web-1	nginx:1.27	", "stack-app-1	example/app:2.4.1	", "stack-cache-1	redis:7-alpine	stack"}
		var digests []string
		for _, n := range sortedKeys(sh.apps) {
			for i, img := range sh.apps[n].images {
				out = append(out, n+"-"+sh.apps[n].services[i]+"-1	"+img+"	"+n)
				digests = append(digests, img+"	sha256:"+strings.Repeat("0", 64))
			}
		}
		out = append(out, "---", "nginx:1.27	sha256:"+strings.Repeat("1", 64), "redis:7-alpine	sha256:"+strings.Repeat("2", 64))
		return append(out, digests...), 0
	case strings.HasPrefix(cmd, "docker ps -a --no-trunc --format"):
		var out []string
		for _, c := range [][4]string{{"3f2a9c1d7b10", "nginx:1.27", "stack-web-1", "0.0.0.0:80->80/tcp"}, {"a81c04e5f2d3", "example/app:2.4.1", "stack-app-1", "8080/tcp"}, {"c7d9e0b1a234", "redis:7-alpine", "stack-cache-1", "6379/tcp"}} {
			state, status := "running", "Up 12 days"
			if sh.stopped[c[2]] {
				state, status = "exited", "Exited (0) 2 minutes ago"
			}
			out = append(out, fmt.Sprintf(`{"ID":%q,"Image":%q,"Names":%q,"Ports":%q,"State":%q,"Status":%q,"CreatedAt":"2026-10-07 09:12:44 +0000 UTC"}`, c[0], c[1], c[2], c[3], state, status))
		}
		return append(out, sh.appContainers()...), 0
	case strings.HasPrefix(cmd, "docker images --format"):
		out := []string{
			`{"ID":"sha256:7f553e8bbc89","Repository":"nginx","Tag":"1.27","Size":"192MB","CreatedSince":"5 weeks ago"}`,
			`{"ID":"sha256:b1c2d3e4f5a6","Repository":"example/app","Tag":"2.4.1","Size":"84.3MB","CreatedSince":"2 weeks ago"}`,
			`{"ID":"sha256:9a8b7c6d5e4f","Repository":"redis","Tag":"7-alpine","Size":"41.2MB","CreatedSince":"3 months ago"}`,
		}
		for _, img := range sh.images {
			repo, tag, _ := strings.Cut(img, ":")
			if tag == "" {
				tag = "latest"
			}
			out = append(out, fmt.Sprintf(`{"ID":"sha256:0123456789ab","Repository":%q,"Tag":%q,"Size":"120MB","CreatedSince":"刚刚"}`, repo, tag))
		}
		return out, 0
	case strings.HasPrefix(cmd, "docker logs"):
		return []string{"2026/11/13 09:00:01 [notice] 1#1: start worker processes", "192.0.2.1 - - [13/Nov/2026:09:17:42 +0000] \"GET / HTTP/1.1\" 200 612", "192.0.2.1 - - [13/Nov/2026:09:17:44 +0000] \"GET /health HTTP/1.1\" 200 2"}, 0
	case strings.HasPrefix(cmd, "docker stop "), strings.HasPrefix(cmd, "docker start "), strings.HasPrefix(cmd, "docker restart "), strings.HasPrefix(cmd, "docker rm -f "):
		f := strings.Fields(cmd)
		name := f[len(f)-1]
		if sh.stopped == nil {
			sh.stopped = map[string]bool{}
		}
		sh.stopped[name] = f[1] == "stop"
		return []string{name}, 0
	case strings.HasPrefix(cmd, "docker pull "):
		img := strings.Trim(strings.Fields(cmd)[2], "'")
		sh.images = append(sh.images, img)
		return []string{"latest: Pulling from " + img, "a2abf6c4d29d: Pull complete", "f3409a9a9e73: Pull complete", "Digest: sha256:0f2d3c4b5a69788796a5b4c3d2e1f00112233445566778899aabbccddeeff001", "Status: Downloaded newer image for " + img}, 0
	case strings.HasPrefix(cmd, "docker login"):
		return []string{"Login Succeeded"}, 0
	case strings.HasPrefix(cmd, "docker rmi "):
		return []string{"Untagged: " + strings.Fields(cmd)[2]}, 0
	case strings.HasPrefix(cmd, "docker ps") || strings.HasPrefix(cmd, "docker compose ps"):
		if !h.HasTag("docker") {
			return []string{"bash: docker: command not found"}, 127
		}
		return []string{
			"CONTAINER ID   IMAGE               STATUS        PORTS                NAMES",
			"3f2a9c1d7b10   nginx:1.27          Up 12 days    0.0.0.0:80->80/tcp   stack-web-1",
			"a81c04e5f2d3   example/app:2.4.1   Up 12 days    8080/tcp             stack-app-1",
			"c7d9e0b1a234   redis:7-alpine      Up 37 days    6379/tcp             stack-cache-1",
		}, 0
	case strings.HasPrefix(cmd, "docker compose pull"), strings.HasPrefix(cmd, "docker compose up"):
		return []string{
			"[+] Pulling 3/3",
			" ✔ web Pulled                                              1.2s",
			" ✔ app Pulled                                              3.8s",
			" ✔ cache Pulled                                            0.9s",
			"[+] Running 3/3",
			" ✔ Container stack-cache-1  Running                        0.0s",
			" ✔ Container stack-app-1    Started                        1.4s",
			" ✔ Container stack-web-1    Started                        0.6s",
		}, 0
	case strings.HasPrefix(cmd, "uname"):
		return []string{fmt.Sprintf("Linux %s %s #1 SMP x86_64 GNU/Linux", h.Name, sh.kernel)}, 0
	case cmd == "hostname":
		return []string{h.Name}, 0
	case cmd == "whoami":
		return []string{"ops"}, 0
	case strings.HasPrefix(cmd, "systemctl status"):
		unit := strings.TrimSpace(strings.TrimPrefix(cmd, "systemctl status"))
		return []string{
			"● " + unit + ".service - " + unit,
			"     Loaded: loaded (/lib/systemd/system/" + unit + ".service; enabled)",
			"     Active: active (running) since Mon 2026-10-07 09:12:44 UTC; 1 month 6 days ago",
			"   Main PID: 812 (" + unit + ")",
			"      Tasks: 5 (limit: 4657)",
			"     Memory: 18.4M",
		}, 0
	case strings.HasPrefix(cmd, "systemctl restart"), strings.HasPrefix(cmd, "systemctl reload"):
		return nil, 0
	case strings.HasPrefix(cmd, "apt"):
		return []string{
			"Hit:1 http://mirror.example.com/debian bookworm InRelease",
			"Get:2 http://mirror.example.com/debian bookworm-updates InRelease [55.4 kB]",
			"Get:3 http://mirror.example.com/debian-security bookworm-security InRelease [48.0 kB]",
			"Fetched 103 kB in 1s (148 kB/s)",
			"Reading package lists... Done",
			"Building dependency tree... Done",
			"3 packages can be upgraded. Run 'apt list --upgradable' to see them.",
		}, 0
	case strings.HasPrefix(cmd, "cat "):
		p := strings.TrimSpace(strings.TrimPrefix(cmd, "cat "))
		if data, ok := sh.files[p]; ok {
			return strings.Split(strings.TrimRight(string(data), "\n"), "\n"), 0
		}
		return []string{"cat: " + p + ": No such file or directory"}, 1
	case strings.HasPrefix(cmd, "echo "):
		return []string{strings.Trim(strings.TrimPrefix(cmd, "echo "), `"'`)}, 0
	case cmd == "false" || strings.HasPrefix(cmd, "exit 1"):
		return []string{"command failed"}, 1
	}
	return []string{"[sim] 已在模拟主机 " + h.Name + " 上执行：" + command}, 0
}

// ExecStdin 读完标准输入再执行；docker load 会报告收到了多少字节。
func (s *Sim) ExecStdin(ctx context.Context, h inventory.Host, command string, stdin io.Reader, out io.Writer) (int, error) {
	if err := s.offline(h); err != nil {
		return -1, err
	}
	n, _ := io.Copy(io.Discard, stdin)
	if strings.HasPrefix(command, "docker load") {
		if !h.HasTag("docker") {
			io.WriteString(out, "bash: docker: command not found\n")
			return 127, nil
		}
		s.mu.Lock()
		sh := s.host(h)
		sh.images = append(sh.images, "uploaded/image:latest")
		s.mu.Unlock()
		fmt.Fprintf(out, "Loaded image: uploaded/image:latest（收到 %d 字节）\n", n)
		return 0, nil
	}
	return s.Exec(ctx, h, command, out)
}

func (s *Sim) ReadFile(ctx context.Context, h inventory.Host, p string, limit int64) ([]byte, error) {
	if err := s.offline(h); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.host(h).files[path.Clean(p)]
	if !ok {
		return nil, ErrNotExist
	}
	if limit > 0 && int64(len(data)) > limit {
		data = data[:limit]
	}
	return append([]byte(nil), data...), nil
}

func (s *Sim) WriteFile(ctx context.Context, h inventory.Host, p string, data []byte) error {
	if err := s.offline(h); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.host(h).files[path.Clean(p)] = append([]byte(nil), data...)
	return nil
}

func (s *Sim) ListDir(ctx context.Context, h inventory.Host, dir string) ([]FileInfo, error) {
	if err := s.offline(h); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir = path.Clean(dir)
	prefix := dir
	if prefix != "/" {
		prefix += "/"
	}
	seen := map[string]FileInfo{}
	mod := time.Now().Add(-72 * time.Hour).Truncate(time.Minute)
	for p, data := range s.host(h).files {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := strings.TrimPrefix(p, prefix)
		name, _, nested := strings.Cut(rest, "/")
		if nested {
			seen[name] = FileInfo{Name: name, Dir: true, Mode: "drwxr-xr-x", Size: 4096, ModTime: mod}
		} else {
			seen[name] = FileInfo{Name: name, Mode: "-rw-r--r--", Size: int64(len(data)), ModTime: mod}
		}
	}
	if len(seen) == 0 && dir != "/" {
		return nil, ErrNotExist
	}
	out := make([]FileInfo, 0, len(seen))
	for _, fi := range seen {
		out = append(out, fi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
