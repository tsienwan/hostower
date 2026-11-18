package metrics

import "testing"

func TestParseHardware(t *testing.T) {
	// 物理服务器：有 lscpu、有厂家信息、NVMe 和机械盘。
	server := "Architecture:            x86_64\nCPU(s):                  64\nVendor ID:               GenuineIntel\n" +
		"Model name:              Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz\nThread(s) per core:      2\nCore(s) per socket:      16\n" +
		"Socket(s):               2\nCPU max MHz:             3200.0000\nL3 cache:                96 MiB (2 instances)\n" +
		"---\nvendor_id\t: GenuineIntel\nmodel name\t: Intel(R) Xeon(R) Gold 6338 CPU @ 2.00GHz\n" +
		"---\nsys_vendor=Dell Inc.\nproduct_name=PowerEdge R650\nboard_vendor=Dell Inc.\nboard_name=0X0X0X\n" +
		"---\nnvme0n1 1875385008 0 SAMSUNG MZQL2960HCJR-00A07\nsda 15628053168 1 ST8000NM000A-2KE101\n"
	hw := parseHardware(server)
	if hw == nil || hw.CPUVendor != "Intel" || hw.Cores != 32 || hw.Threads != 64 || hw.Sockets != 2 || hw.MaxMHz != 3200 || hw.L3 != "96 MiB" {
		t.Fatalf("CPU 解析不对：%+v", hw)
	}
	if hw.Vendor != "Dell Inc." || hw.Product != "PowerEdge R650" || hw.Board != "Dell Inc. 0X0X0X" || hw.Hypervisor != "" {
		t.Fatalf("整机信息不对：%+v", hw)
	}
	if len(hw.Disks) != 2 || hw.Disks[0].Kind != "ssd" || hw.Disks[1].Kind != "hdd" || hw.Disks[1].Bytes != 15628053168*512 || hw.Disks[1].Model != "ST8000NM000A-2KE101" {
		t.Fatalf("磁盘不对：%+v", hw.Disks)
	}

	// 没有 lscpu 的小系统（比如 Alpine）、厂家信息是占位值的虚拟机：从 /proc/cpuinfo 补，占位值不显示。
	small := "---\nvendor_id\t: AuthenticAMD\nmodel name\t: AMD EPYC 7B13\ncpu MHz\t\t: 2449.998\nflags\t\t: fpu vme hypervisor lahf_lm\n" +
		"---\nsys_vendor=To Be Filled By O.E.M.\nproduct_name=Default string\n---\nvda 167772160 1 \n"
	hw = parseHardware(small)
	if hw == nil || hw.CPUVendor != "AMD" || hw.CPUModel != "AMD EPYC 7B13" || hw.Vendor != "" || hw.Product != "" {
		t.Fatalf("小系统解析不对：%+v", hw)
	}
	if !hw.Virtual || len(hw.Disks) != 1 || hw.Disks[0].Bytes != 167772160*512 || hw.Disks[0].Model != "" || hw.Disks[0].Kind != "" {
		t.Fatalf("磁盘不对：%+v", hw.Disks)
	}

	// 虚拟机里的“机械盘”不可信：不下结论。
	vm := "Hypervisor vendor:       KVM\nModel name:              Neoverse-N1\n---\n---\n---\nvda 100 1 \n"
	if hw = parseHardware(vm); hw == nil || hw.Disks[0].Kind != "" || hw.Hypervisor != "KVM" {
		t.Fatalf("虚拟机解析不对：%+v", hw)
	}

	if parseHardware("---\n---\n---\n") != nil {
		t.Fatal("什么都没读到时应该返回 nil")
	}
}

func TestVirtualIface(t *testing.T) {
	for _, n := range []string{"lo", "veth1a2b3c", "docker0", "br-0123456789ab", "virbr0", "cni0", "flannel.1", "cali123", "vxlan.calico"} {
		if !VirtualIface(n) {
			t.Errorf("%s 应该算虚拟网卡", n)
		}
	}
	for _, n := range []string{"eth0", "ens3", "enp1s0", "wlan0", "bond0", "wg0", "tun0"} {
		if VirtualIface(n) {
			t.Errorf("%s 不应该算虚拟网卡", n)
		}
	}
}
