package metrics

import (
	"regexp"
	"strings"
)

// 统计口径：整机的网络速率和月流量都只算“出口网卡”，同一份流量只算一遍。
//
// VirtualIface 已经去掉了回环和容器、虚拟网络的网卡。剩下的网卡里还有几类会把同一份流量再算一遍：
//   - 隧道（WireGuard、OpenVPN、Tailscale、GRE、PPP 等）：流量在隧道网卡上算一遍，封装后在物理网卡上又算一遍；
//   - 网桥（br0、vmbr0）：桥上的流量同时出现在网桥和挂在它下面的物理网卡上；
//   - VLAN 子接口（eth0.100）：同时算在父网卡上；
//   - ifb（tc 限速用的镜像网卡）、dummy；
//   - 绑定（bond0、team0）：等于下面几块网卡之和。有绑定网卡时只算它，不再算 eth、en 开头的成员网卡。
// 服务商按出口计费，出口就是剩下的这些。页面上可以为配额单独指定一块网卡（包括隧道），那时只算那一块。

var overlayPrefixes = []string{"wg", "tun", "tap", "tailscale", "zt", "ppp", "gre", "erspan", "sit", "ip6tnl", "ip6gre", "ipip", "ip_vti", "ip6_vti", "vti",
	"ipsec", "nebula", "utun", "he-ipv6", "ifb", "dummy", "nlmon", "macvlan", "macvtap", "ipvlan", "vmbr", "bridge"}

var (
	bridgeRe = regexp.MustCompile(`^br\d`)
	bondRe   = regexp.MustCompile(`^(bond|team)\d`)
	memberRe = regexp.MustCompile(`^(eth|en|em|p\d)`)
)

// overlayIface 报告一个（已经不是虚拟网卡的）网卡是不是隧道、网桥、VLAN 子接口这类会重复计数的网卡。
func overlayIface(name string) bool {
	if strings.ContainsAny(name, ".@") || bridgeRe.MatchString(name) {
		return true
	}
	for _, p := range overlayPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

func ifaceNames(m map[string]ifaceCounter) []string {
	out := make([]string, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	return out
}

// UplinkIfaces 从一台主机的网卡里挑出出口网卡（见上面的说明）。全被排除时（比如只有一块 PPP 网卡）退回全部非虚拟网卡，免得什么都不算。
func UplinkIfaces(names []string) map[string]bool {
	out := map[string]bool{}
	bond := false
	for _, n := range names {
		if !VirtualIface(n) && bondRe.MatchString(n) {
			bond = true
		}
	}
	for _, n := range names {
		if VirtualIface(n) || overlayIface(n) || (bond && memberRe.MatchString(n)) {
			continue
		}
		out[n] = true
	}
	if len(out) == 0 {
		for _, n := range names {
			if !VirtualIface(n) {
				out[n] = true
			}
		}
	}
	return out
}
