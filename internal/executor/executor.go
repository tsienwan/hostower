// Package executor 定义“命令怎么到达主机”的唯一接口。
// 执行器只是通道：判级、审批、审计都在内核里完成，执行器不做任何决策。
package executor

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/tsienwan/hostower/internal/inventory"
)

// FileInfo 是目录里的一项。
type FileInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"modTime"`
	Dir     bool      `json:"dir"`
	Link    bool      `json:"link"`
}

// Executor 是所有上层依赖的唯一接口。
type Executor interface {
	// Exec 执行一条命令，把输出流式写进 out，返回退出码。
	Exec(ctx context.Context, h inventory.Host, command string, out io.Writer) (exitCode int, err error)
	// ReadFile 读文件，最多读 limit 字节。
	ReadFile(ctx context.Context, h inventory.Host, path string, limit int64) ([]byte, error)
	// WriteFile 写文件（先写临时文件再改名）。
	WriteFile(ctx context.Context, h inventory.Host, path string, data []byte) error
	// ListDir 列目录。
	ListDir(ctx context.Context, h inventory.Host, path string) ([]FileInfo, error)
}

// StdinExecutor 是能给命令喂标准输入的执行器（docker login 的密码、docker load 的镜像包）。
type StdinExecutor interface {
	ExecStdin(ctx context.Context, h inventory.Host, command string, stdin io.Reader, out io.Writer) (exitCode int, err error)
}

// Reconnecter 是保持着长连接的执行器（SSH）：断开一台主机的连接，下一个会话重新登录。
type Reconnecter interface {
	Reconnect(host string)
}

// ErrNotExist 表示远端路径不存在。
var ErrNotExist = errors.New("路径不存在")

// Registry 按名字找执行器。
type Registry map[string]Executor

// ProbeCommand 是指标采样用的固定只读命令。它是编译期常量，
// 内核在启动时确认它被判为 L0，否则拒绝启动。
const ProbeCommand = `cat /proc/loadavg; echo ---; grep -E '^(MemTotal|MemAvailable|Buffers|Cached|SReclaimable):' /proc/meminfo; echo ---; head -n 1 /proc/stat; echo ---; df -P -k; echo ---; cat /proc/uptime; echo ---; uname -sr; echo ---; grep -E '^PRETTY_NAME=' /etc/os-release; echo ---; nproc; echo ---; cat /proc/net/dev; echo ---; grep -E '^(SwapTotal|SwapFree):' /proc/meminfo; echo ---; ls /proc; echo ---; cat /proc/diskstats; echo ---; cat /sys/class/thermal/thermal_zone*/temp 2>/dev/null`

// StreamProbeCommand 是持续采样用的固定只读脚本：在主机上循环运行，每 StreamProbeInterval 输出一轮，
// 每轮的前十三段和 ProbeCommand 的格式相同（用 --- 分隔），后面多两段容器的数据，以单独一行 === 结束。
//
// 第十四段是各容器的占用，直接从 cgroup 文件里读（每行：容器编号、累计 CPU 微秒、内存字节、其中可回收的文件缓存、
// 累计入站字节、累计出站字节），不调用 docker stats——那条命令自己要跑一两秒。支持 cgroup v2 和 v1、
// systemd 和 cgroupfs 两种驱动。第十五段是“编号 名字”的对照表，每十二轮用一次 docker ps 取。
//
// /etc/os-release 的最后一行可能没有换行（比如 Docker Desktop 的虚拟机），读的时候用 `|| [ -n "$l" ]` 把它也读进来。
// 它和 ProbeCommand 的区别在开销：这里读 /proc 用的全是 shell 内建命令（read、echo、case），不启动任何进程；
// 每轮只有一次 sleep，每十二轮（一分钟）跑一次 df。系统信息、进程数这些变化慢的段落也是每十二轮才输出，
// 其余时候留空，由采样器沿用上一次的值。它同样是编译期常量，内核启动时确认是 L0。
const StreamProbeCommand = `i=0; while :; do if [ $((i % 12)) -eq 0 ] && [ -x ~/.hostower/bin/hostower-agent ]; then ~/.hostower/bin/hostower-agent stream 2>/dev/null; fi; read -r l </proc/loadavg; echo "$l"; echo ---; while IFS= read -r l; do echo "$l"; done </proc/meminfo; echo ---; read -r l </proc/stat; echo "$l"; echo ---; if [ $((i % 12)) -eq 0 ]; then df -P -k; fi; echo ---; read -r l </proc/uptime; echo "$l"; echo ---; if [ $((i % 12)) -eq 0 ]; then read -r a </proc/sys/kernel/ostype; read -r b </proc/sys/kernel/osrelease; echo "$a $b"; fi; echo ---; if [ $((i % 12)) -eq 0 ]; then while IFS= read -r l || [ -n "$l" ]; do case $l in PRETTY_NAME=*) echo "$l";; esac; done </etc/os-release; fi; echo ---; if [ $((i % 12)) -eq 0 ]; then n=0; while read -r a b; do case $a in cpu[0-9]*) n=$((n+1));; esac; done </proc/stat; echo $n; fi; echo ---; while IFS= read -r l; do echo "$l"; done </proc/net/dev; echo ---; echo ---; if [ $((i % 12)) -eq 0 ]; then for d in /proc/[0-9]*; do echo "${d#/proc/}"; done; fi; echo ---; while IFS= read -r l; do echo "$l"; done </proc/diskstats; echo ---; for f in /sys/class/thermal/thermal_zone*/temp; do if [ -r "$f" ]; then read -r l <"$f"; echo "$l"; fi; done; echo ---; for d in /sys/fs/cgroup/docker/*/ /sys/fs/cgroup/system.slice/docker-*.scope/ /sys/fs/cgroup/cpuacct/docker/*/ /sys/fs/cgroup/cpuacct/system.slice/docker-*.scope/; do d=${d%/}; u=; m=0; f=0; if [ -r "$d/cpu.stat" ] && [ -r "$d/memory.current" ]; then read -r k u <"$d/cpu.stat"; read -r m <"$d/memory.current"; while read -r k v; do case $k in inactive_file) f=$v; break;; esac; done <"$d/memory.stat"; elif [ -r "$d/cpuacct.usage" ]; then read -r u <"$d/cpuacct.usage"; u=$((u / 1000)); g="/sys/fs/cgroup/memory/${d#/sys/fs/cgroup/cpuacct/}"; if [ -r "$g/memory.usage_in_bytes" ]; then read -r m <"$g/memory.usage_in_bytes"; fi; fi; if [ -n "$u" ]; then id=${d##*/}; id=${id#docker-}; id=${id%.scope}; rx=0; tx=0; p=; if [ -r "$d/cgroup.procs" ]; then read -r p <"$d/cgroup.procs"; fi; if [ -n "$p" ] && [ -r "/proc/$p/net/dev" ]; then while read -r n a b c e h j q r t z; do case $n in lo:) ;; *:) rx=$((rx + a)); tx=$((tx + t));; esac; done <"/proc/$p/net/dev"; fi; echo "$id $u $m $f $rx $tx"; fi; done; echo ---; if [ $((i % 12)) -eq 0 ]; then docker ps --no-trunc --format '{{.ID}} {{.Names}}' 2>/dev/null; fi; echo ===; i=$((i+1)); sleep 5; done`

// 脚本每十二轮看一眼主机上有没有装增强采集程序（risk.AgentPath）：有就运行它，由它接着输出同样格式的数据，
// 外加进程排行；它退出或起不来，脚本接着自己采。所以装不装采集程序，用的是同一条连接、同一套解析。

// AgentInstallCommand 把从标准输入送来的采集程序放到固定位置（先写临时文件再改名）。它是一次变更，走审批。
const AgentInstallCommand = `mkdir -p ~/.hostower/bin && cat > ~/.hostower/bin/hostower-agent.new && chmod 755 ~/.hostower/bin/hostower-agent.new && mv -f ~/.hostower/bin/hostower-agent.new ~/.hostower/bin/hostower-agent`

// AgentRemoveCommand 删掉采集程序。
const AgentRemoveCommand = `rm -f ~/.hostower/bin/hostower-agent`

// StreamProbeInterval 是 StreamProbeCommand 两轮输出之间的间隔（写死在脚本的 sleep 里）。
const StreamProbeInterval = 5 * time.Second

// StreamProbeEnd 是一轮输出结束的标记行。
const StreamProbeEnd = "==="

// HardwareProbeCommand 是读硬件信息用的固定只读命令：CPU 型号和规格（lscpu，没有就读 /proc/cpuinfo）、
// 整机的厂家和型号（/sys/class/dmi/id 里人人可读的那几个文件，不读序列号）、各块磁盘的容量和型号。
// 这些信息几乎不变，所以只在连上主机时跑一次、之后每 6 小时一次。同样是编译期常量，启动时确认是 L0。
// 各段用 --- 分隔；读不到的段落留空（比如虚拟机里常常没有磁盘型号）。
const HardwareProbeCommand = `LC_ALL=C lscpu 2>/dev/null; echo ---; while IFS= read -r l; do case $l in "model name"*|"cpu model"*|Hardware*|"cpu MHz"*|vendor_id*|flags*) echo "$l";; "") break;; esac; done </proc/cpuinfo; echo ---; for f in sys_vendor product_name product_version board_vendor board_name; do if [ -r /sys/class/dmi/id/$f ]; then read -r v </sys/class/dmi/id/$f; echo "$f=$v"; fi; done; echo ---; for d in /sys/block/*; do n=${d##*/}; case $n in loop*|ram*|zram*|dm-*|sr*|fd*|md*|nbd*) continue;; esac; s=0; r=; m=; if [ -r $d/size ]; then read -r s <$d/size; fi; if [ -r $d/queue/rotational ]; then read -r r <$d/queue/rotational; fi; if [ -r $d/device/model ]; then read -r m <$d/device/model; fi; echo "$n $s $r $m"; done`

// ContainerProbeCommand 是容器占用采样用的固定只读命令，同样在启动时确认是 L0。
// 没有 Docker 或没有权限的主机上它会失败，采样器会隔一段时间再试。
const ContainerProbeCommand = `docker stats --no-stream --format '{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}|{{.NetIO}}' 2>/dev/null`
