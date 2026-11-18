package metrics

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Hardware 是一台主机的硬件信息：CPU 型号和规格、整机厂家和型号、磁盘。几乎不变，所以只偶尔读一次。
// 读不到的字段留空（容器里、部分虚拟机里没有厂家信息，云盘通常没有型号）。
type Hardware struct {
	Arch      string  `json:"arch,omitempty"`
	CPUVendor string  `json:"cpuVendor,omitempty"` // Intel、AMD、ARM……
	CPUModel  string  `json:"cpuModel,omitempty"`
	Sockets   int     `json:"sockets,omitempty"`
	Cores     int     `json:"cores,omitempty"`   // 物理核心总数
	Threads   int     `json:"threads,omitempty"` // 逻辑处理器总数
	MaxMHz    float64 `json:"maxMHz,omitempty"`
	L3        string  `json:"l3,omitempty"`
	// Hypervisor 是虚拟化平台（KVM、Xen、Microsoft……）；Virtual 表示这是虚拟机（有时知道是虚拟机，但不知道平台）。
	Hypervisor string   `json:"hypervisor,omitempty"`
	Virtual    bool     `json:"virtual,omitempty"`
	Vendor     string   `json:"vendor,omitempty"`  // 整机厂家（云厂商、服务器厂商）
	Product    string   `json:"product,omitempty"` // 整机型号或实例规格
	Board      string   `json:"board,omitempty"`   // 主板（厂家 型号）
	Disks      []HWDisk `json:"disks,omitempty"`
	At         int64    `json:"at"` // 采集时间（毫秒）
}

// HWDisk 是一块物理（或虚拟）磁盘。
type HWDisk struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	// Kind 是 ssd、hdd，或者空（不知道）。虚拟磁盘常把自己报成机械盘，所以虚拟机里不下结论。
	Kind  string `json:"kind,omitempty"`
	Model string `json:"model,omitempty"`
}

// hardwareEvery 是重新读硬件信息的间隔。
const hardwareEvery = 6 * time.Hour

// 主板固件里常见的占位值：没有意义，不显示。
var dmiPlaceholders = []string{"to be filled by o.e.m.", "default string", "system product name", "system manufacturer",
	"not specified", "not applicable", "none", "o.e.m.", "oem", "unknown", "type1productconfigid", "x.x", "0123456789"}

func cleanDMI(v string) string {
	v = strings.TrimSpace(v)
	low := strings.ToLower(v)
	for _, p := range dmiPlaceholders {
		if low == p {
			return ""
		}
	}
	return v
}

var cpuVendors = map[string]string{"GenuineIntel": "Intel", "AuthenticAMD": "AMD", "HygonGenuine": "Hygon", "CentaurHauls": "VIA",
	"  Shanghai  ": "Zhaoxin", "ARM": "ARM", "Apple": "Apple", "HiSilicon": "HiSilicon", "Phytium": "Phytium", "Ampere": "Ampere"}

// parseHardware 解析 executor.HardwareProbeCommand 的输出。
func parseHardware(out string) *Hardware {
	// 按行切段：单独一行 --- 就是下一段。不能直接按 "\n---\n" 切，空段落连在一起时分隔符会互相重叠。
	parts := []string{""}
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if l == "---" {
			parts = append(parts, "")
			continue
		}
		parts[len(parts)-1] += l + "\n"
	}
	sec := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	hw := &Hardware{At: time.Now().UnixMilli()}
	perSocket := 0
	// 第一段：lscpu。
	for _, l := range strings.Split(sec(0), "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Architecture":
			hw.Arch = v
		case "CPU(s)":
			hw.Threads, _ = strconv.Atoi(v)
		case "Vendor ID":
			hw.CPUVendor = v
		case "Model name":
			if v != "-" {
				hw.CPUModel = v
			}
		case "Socket(s)":
			hw.Sockets, _ = strconv.Atoi(v)
		case "Core(s) per socket", "Core(s) per cluster":
			perSocket, _ = strconv.Atoi(v)
		case "CPU max MHz":
			hw.MaxMHz, _ = strconv.ParseFloat(v, 64)
		case "L3 cache":
			hw.L3 = strings.TrimSpace(strings.Split(v, "(")[0])
		case "Hypervisor vendor":
			hw.Hypervisor = v
		}
	}
	if hw.Sockets > 0 && perSocket > 0 {
		hw.Cores = hw.Sockets * perSocket
	} else if perSocket > 0 {
		hw.Cores = perSocket
	}
	// 第二段：/proc/cpuinfo（没有 lscpu 时用它补）。
	for _, l := range strings.Split(sec(1), "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "model name", "cpu model", "Hardware":
			if hw.CPUModel == "" {
				hw.CPUModel = v
			}
		case "vendor_id":
			if hw.CPUVendor == "" {
				hw.CPUVendor = v
			}
		case "cpu MHz":
			if hw.MaxMHz == 0 {
				hw.MaxMHz, _ = strconv.ParseFloat(v, 64)
			}
		case "flags":
			hw.Virtual = hw.Virtual || strings.Contains(" "+v+" ", " hypervisor ")
		}
	}
	if name, ok := cpuVendors[hw.CPUVendor]; ok {
		hw.CPUVendor = name
	}
	hw.CPUModel = strings.Join(strings.Fields(hw.CPUModel), " ")
	// 第三段：整机厂家和型号。
	dmi := map[string]string{}
	for _, l := range strings.Split(sec(2), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			dmi[k] = cleanDMI(v)
		}
	}
	hw.Vendor, hw.Product = dmi["sys_vendor"], dmi["product_name"]
	if hw.Vendor == "" && hw.Product == "" {
		hw.Vendor = dmi["board_vendor"]
	}
	if b := strings.TrimSpace(dmi["board_vendor"] + " " + dmi["board_name"]); b != "" && dmi["board_name"] != "" && dmi["board_name"] != hw.Product {
		hw.Board = b
	}
	hw.Virtual = hw.Virtual || hw.Hypervisor != ""
	for _, v := range []string{"qemu", "kvm", "vmware", "virtualbox", "innotek", "xen", "bochs", "openstack", "parallels"} {
		if strings.Contains(strings.ToLower(hw.Vendor+" "+hw.Product), v) {
			hw.Virtual = true
		}
	}
	// 第四段：磁盘（名字 扇区数 是否机械 型号）。
	for _, l := range strings.Split(sec(3), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		sectors, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || sectors <= 0 {
			continue
		}
		d := HWDisk{Name: f[0], Bytes: sectors * 512}
		if len(f) > 2 && !hw.Virtual {
			switch f[2] {
			case "0":
				d.Kind = "ssd"
			case "1":
				d.Kind = "hdd"
			}
		}
		if strings.HasPrefix(d.Name, "nvme") {
			d.Kind = "ssd"
		}
		if len(f) > 3 {
			d.Model = strings.Join(f[3:], " ")
		}
		hw.Disks = append(hw.Disks, d)
	}
	if hw.CPUModel == "" && hw.Vendor == "" && len(hw.Disks) == 0 {
		return nil
	}
	return hw
}

// hardwareIfDue 到点了就另起一个协程读一次硬件信息：连上主机后的第一轮，之后每 6 小时一次。
func (s *Sampler) hardwareIfDue(ctx context.Context, name string) {
	s.mu.Lock()
	st := s.hosts[name]
	due := st != nil && !st.hwBusy && time.Now().After(st.hwNext)
	if due {
		st.hwBusy = true
	}
	s.mu.Unlock()
	if !due {
		return
	}
	go func() {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		out, err := s.k.ProbeHardware(cctx, name)
		cancel()
		s.mu.Lock()
		defer s.mu.Unlock()
		st := s.hosts[name]
		if st == nil {
			return
		}
		st.hwBusy = false
		if err != nil {
			st.hwNext = time.Now().Add(10 * time.Minute) // 失败了过一会儿再试
			return
		}
		st.hwNext = time.Now().Add(hardwareEvery)
		if hw := parseHardware(out); hw != nil {
			st.status.Hardware = hw
		}
	}()
}

// virtualPrefixes 是容器和虚拟网络在主机上建的网卡：容器的流量会先经过 veth、再经过网桥、最后走物理网卡，
// 把它们也加进来，同一份流量会被算两三遍。整机的网络速率只算物理网卡（和 VPN 这类隧道）。
var virtualPrefixes = []string{"veth", "docker", "br-", "virbr", "cni", "flannel", "cali", "vxlan", "kube-", "weave", "lxcbr", "lxc", "tunl", "cilium", "podman", "vnet"}

// VirtualIface 报告一个网卡名是不是回环或容器、虚拟网络建的网卡。
func VirtualIface(name string) bool {
	if name == "lo" {
		return true
	}
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
