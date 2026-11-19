package risk

import (
	"regexp"
	"strings"
)

// AgentPath 是增强采集程序在主机上的固定位置（登录账号的家目录下）。
const AgentPath = "~/.hostower/bin/hostower-agent"

// command 是剥掉包装之后的一条简单命令。
type command struct {
	name      string
	full      string // 命令名的原样写法（带路径）
	args      []word
	stdinArgs bool
}

// has 报告是否带某个短选项（可以在 -abc 这样的组合里）或长选项。
func (c *command) has(shorts string, longs ...string) bool {
	for _, a := range c.args {
		t := a.text
		if len(t) < 2 || t[0] != '-' {
			continue
		}
		if strings.HasPrefix(t, "--") {
			name := strings.TrimPrefix(t, "--")
			if i := strings.IndexByte(name, '='); i >= 0 {
				name = name[:i]
			}
			for _, l := range longs {
				if l == name {
					return true
				}
			}
			continue
		}
		if strings.ContainsAny(t[1:], shorts) && shorts != "" {
			return true
		}
	}
	return false
}

// pos 返回位置参数（不以 - 开头的参数）。
func (c *command) pos() []string {
	var out []string
	dashdash := false
	for _, a := range c.args {
		if !dashdash && a.text == "--" {
			dashdash = true
			continue
		}
		if !dashdash && strings.HasPrefix(a.text, "-") && len(a.text) > 1 {
			continue
		}
		out = append(out, a.text)
	}
	return out
}

func (c *command) sub() string {
	if p := c.pos(); len(p) > 0 {
		return p[0]
	}
	return ""
}

func (c *command) anyPos(words string) bool {
	m := set(words)
	for _, p := range c.pos() {
		if m[p] {
			return true
		}
	}
	return false
}

func (c *command) anyArgPrefix(prefixes ...string) bool {
	for _, a := range c.args {
		for _, p := range prefixes {
			if strings.HasPrefix(a.text, p) {
				return true
			}
		}
	}
	return false
}

func (c *command) joined() string { return joinWords(c.args) }

// 只读命令：自动执行。
var readOnly = set(`
	df du free uptime ps who w id whoami uname pwd true false : echo printf test [ cd
	lsblk lscpu lsmem lsusb lspci nproc arch groups last lastlog stat file which whereis type
	wc cut tr uniq head tail cat tac nl less more grep egrep fgrep zgrep rg strings od xxd hexdump
	base64 md5sum sha1sum sha256sum sha512sum cksum diff cmp basename dirname realpath readlink
	seq sleep ls tree vmstat iostat mpstat pidstat sar ss netstat ping ping6 traceroute tracepath
	mtr dig nslookup host getent pgrep lsof printenv locale tty jq yq column expr getconf
	zcat bzcat xzcat numfmt fold rev paste join comm top htop findmnt blkid history
	export unset set local declare readonly shift return exit wait break continue umask alias
	read getopts let lsmod lsns lsattr smartctl ethtool dpkg-query apt-cache
`)

// 高危命令。
var alwaysL2 = set(`
	reboot shutdown poweroff halt kexec init telinit
	useradd userdel usermod groupadd groupdel groupmod adduser deluser passwd chpasswd gpasswd visudo
	umount swapoff swapon truncate modprobe rmmod insmod setenforce
	lvremove vgremove pvremove lvreduce lvresize lvextend resize2fs fsck e2fsck xfs_repair cryptsetup
	iptables-restore ip6tables-restore grub-install update-grub
`)

// 禁止命令：直接写块设备或文件系统。
var alwaysL3 = set(`mkfs mke2fs mkswap wipefs sgdisk sfdisk blkdiscard`)

// contentReaders 会把文件内容送回输出；参数不确定时不能自动放行。
var contentReaders = set(`
	cat tac nl head tail less more grep egrep fgrep zgrep rg strings od xxd hexdump base64
	cut sort uniq awk gawk mawk sed diff cmp zcat bzcat xzcat tar jq yq paste join comm fold rev find
`)

var usesIdentity = set("ssh scp sftp rsync ssh-keygen ssh-add")

var metadataCommands = set("ls stat chmod chown chgrp test [ touch rm mkdir cd ssh-keygen")

// metadataOnly 报告命令是否只碰元数据，不会把文件内容带出来。
func metadataOnly(c *command) bool {
	if metadataCommands[c.name] {
		return true
	}
	if c.name == "find" {
		for _, a := range c.args {
			if strings.HasPrefix(a.text, "-exec") || strings.HasPrefix(a.text, "-ok") {
				return false
			}
		}
		return true
	}
	return false
}

var criticalUnitRe = regexp.MustCompile(`^(ssh|sshd|firewalld|ufw|nftables|iptables|netfilter-persistent|fail2ban|networking|network|NetworkManager|systemd-networkd|systemd-resolved)(\.service|\.socket)?$`)

var destructiveSQLRe = regexp.MustCompile(`(?i)\b(drop\s+(table|database|schema|index|user)|truncate|delete\s+from|flushall|flushdb|dropdatabase)\b`)

var fdiskLike = set("fdisk gdisk cfdisk parted")

// builtin 是内置规则表。返回 (等级, 规则名, 理由)。
func builtin(e *Engine, v *Verdict, c *command, depth int) (Level, string, string) {
	if strings.HasPrefix(c.name, "mkfs.") || alwaysL3[c.name] {
		return L3, "blockdev.format", "格式化或直接改写块设备"
	}
	if alwaysL2[c.name] {
		return L2, "builtin.high", "高危操作：" + c.name
	}
	if fdiskLike[c.name] {
		if c.has("l", "list") || c.anyPos("print") {
			return L0, "builtin.read", "只读"
		}
		return L3, "blockdev.partition", "改写分区表"
	}
	// Hostower 自己的增强采集程序：只认这一个固定位置、这一种调用（stream），它只读不改。
	// 别的路径上的同名程序、别的参数都不算。
	if c.full == AgentPath && len(c.args) == 1 && c.args[0].static && c.args[0].text == "stream" {
		return L0, "hostower.agent", "Hostower 的采集程序（只读）"
	}
	if readOnly[c.name] {
		return L0, "builtin.read", "只读"
	}

	switch c.name {
	case "rm":
		return ruleRm(c)
	case "dd":
		for _, a := range c.args {
			if strings.HasPrefix(a.text, "of=") && isBlockDevice(a.text[3:]) {
				return L3, "blockdev.write", "写块设备"
			}
		}
		return L1, "builtin.change", "dd 写数据"
	case "shred":
		for _, p := range c.pos() {
			if isBlockDevice(p) {
				return L3, "blockdev.write", "写块设备"
			}
		}
		return L2, "data.delete", "不可恢复地销毁文件"
	case "tee", "cp", "mv", "install":
		pos := c.pos()
		for i, p := range pos {
			if isBlockDevice(p) && (c.name == "tee" || i == len(pos)-1) {
				return L3, "blockdev.write", "写块设备"
			}
			if c.name == "mv" && i < len(pos)-1 && isCriticalRoot(p) {
				return L3, "system.destroy", "移走系统关键目录"
			}
		}
		return L1, "builtin.change", "写文件"
	case "chmod", "chown", "chgrp":
		if c.has("R", "recursive") {
			for _, p := range c.pos() {
				if isCriticalRoot(p) {
					return L3, "system.destroy", "递归修改系统关键目录的权限"
				}
			}
		}
		return L1, "builtin.change", "改权限或属主"
	case "sed":
		if c.has("i", "in-place") {
			return L1, "builtin.change", "原地修改文件"
		}
		return L0, "builtin.read", "只读"
	case "awk", "gawk", "mawk":
		j := c.joined()
		if strings.Contains(j, "system(") || strings.Contains(j, ">") || strings.Contains(j, `| "`) || strings.Contains(j, `|"`) || c.has("i") {
			return L1, "builtin.change", "awk 脚本可能写文件或执行命令"
		}
		return L0, "builtin.read", "只读"
	case "sort":
		if c.has("o", "output") {
			return L1, "builtin.change", "写文件"
		}
		return L0, "builtin.read", "只读"
	case "find":
		return ruleFind(e, v, c, depth)
	case "date":
		if c.has("s", "set") {
			return L2, "builtin.high", "修改系统时间"
		}
		return L0, "builtin.read", "只读"
	case "hostname":
		if len(c.pos()) > 0 {
			return L1, "builtin.change", "修改主机名"
		}
		return L0, "builtin.read", "只读"
	case "hostnamectl", "timedatectl", "localectl", "loginctl", "resolvectl", "networkctl":
		s := c.sub()
		if s == "" || s == "status" || strings.HasPrefix(s, "show") || strings.HasPrefix(s, "list") || s == "query" {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "修改系统设置"
	case "curl":
		if c.has("oOdFTX", "output", "remote-name", "request", "data", "data-raw", "data-binary", "data-urlencode", "form", "upload-file", "json") {
			return L1, "builtin.change", "写文件或发送非只读请求"
		}
		return L0, "builtin.read", "只读请求"
	case "journalctl":
		if c.anyArgPrefix("--vacuum", "--rotate", "--flush") {
			return L1, "builtin.change", "清理日志"
		}
		return L0, "builtin.read", "只读"
	case "dmesg":
		if c.has("cC", "clear", "read-clear") {
			return L1, "builtin.change", "清空内核日志"
		}
		return L0, "builtin.read", "只读"
	case "systemctl":
		return ruleSystemctl(c)
	case "service":
		pos := c.pos()
		if len(pos) >= 2 && pos[1] == "status" || c.has("", "status-all") {
			return L0, "builtin.read", "只读"
		}
		if len(pos) >= 1 && criticalUnitRe.MatchString(pos[0]) {
			return L2, "service.critical", "影响远程连接或防火墙的服务"
		}
		return L1, "builtin.change", "变更服务状态"
	case "docker", "podman", "nerdctl":
		return ruleDocker(c)
	case "fail2ban-client":
		// Fail2ban 管着防火墙的封禁规则：除了查看，改设置、封禁解封、重载、启停都按改防火墙算。
		if set("status ping version get banned")[c.sub()] {
			return L0, "builtin.read", "只读"
		}
		return L2, "firewall.change", "修改 Fail2ban 的封禁规则或服务（它管着防火墙）"
	case "certbot":
		switch c.sub() {
		case "certificates":
			return L0, "builtin.read", "只读"
		case "delete", "revoke":
			return L2, "data.delete", "删除或吊销证书"
		}
		return L1, "builtin.change", "申请或续期证书"
	case "apt", "apt-get", "dnf", "yum", "apk", "zypper", "snap", "flatpak":
		s := c.sub()
		if set("list show search policy info depends rdepends madison showpkg check-update repolist provides version changelog")[s] {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "变更软件包"
	case "dpkg":
		if c.has("lLsSpc", "list", "status", "listfiles", "search", "get-selections", "print-architecture", "contents") && !c.has("irP", "install", "remove", "purge", "configure") {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "变更软件包"
	case "rpm":
		if c.has("q", "query", "verify") && !c.has("eiUF", "erase", "install", "upgrade") {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "变更软件包"
	case "pacman":
		if c.anyArgPrefix("-Q") {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "变更软件包"
	case "ip":
		if c.anyPos("add del delete set flush change replace append") {
			return L2, "network.change", "修改网络配置"
		}
		if c.anyPos("exec") {
			return L1, "builtin.change", "在网络命名空间里执行命令"
		}
		return L0, "builtin.read", "只读"
	case "ifconfig":
		if len(c.pos()) <= 1 {
			return L0, "builtin.read", "只读"
		}
		return L2, "network.change", "修改网络配置"
	case "route":
		if c.anyPos("add del delete flush") {
			return L2, "network.change", "修改路由"
		}
		return L0, "builtin.read", "只读"
	case "nmcli":
		if c.anyPos("add delete modify up down set reload connect disconnect off on edit import") {
			return L2, "network.change", "修改网络配置"
		}
		return L0, "builtin.read", "只读"
	case "netplan":
		if s := c.sub(); s == "get" || s == "status" || s == "info" {
			return L0, "builtin.read", "只读"
		}
		return L2, "network.change", "修改网络配置"
	case "iptables", "ip6tables", "ebtables", "arptables":
		if c.has("ADIRFZNXPE", "append", "delete", "insert", "replace", "flush", "zero", "new-chain", "delete-chain", "policy", "rename-chain") {
			return L2, "firewall.change", "修改防火墙规则"
		}
		if c.has("LSV", "list", "list-rules", "version") {
			return L0, "builtin.read", "只读"
		}
		return L2, "firewall.change", "修改防火墙规则"
	case "nft":
		if c.sub() == "list" {
			return L0, "builtin.read", "只读"
		}
		return L2, "firewall.change", "修改防火墙规则"
	case "ufw":
		if s := c.sub(); s == "status" || s == "show" || s == "version" {
			return L0, "builtin.read", "只读"
		}
		return L2, "firewall.change", "修改防火墙规则"
	case "firewall-cmd":
		if c.anyArgPrefix("--add", "--remove", "--set", "--reload", "--complete-reload", "--change", "--new", "--delete", "--panic", "--runtime-to", "--permanent") {
			return L2, "firewall.change", "修改防火墙规则"
		}
		return L0, "builtin.read", "只读"
	case "kill", "pkill", "killall":
		if c.name == "kill" && c.has("lL", "list") && len(c.pos()) == 0 {
			return L0, "builtin.read", "只读"
		}
		for _, a := range c.args {
			if a.text == "1" || a.text == "-1" || criticalUnitRe.MatchString(a.text) || a.text == "systemd" || a.text == "dockerd" {
				return L2, "process.critical", "终止关键进程"
			}
		}
		return L1, "builtin.change", "终止进程"
	case "crontab":
		if c.has("r") {
			return L2, "data.delete", "删除全部定时任务"
		}
		if c.has("l") {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "修改定时任务"
	case "mount":
		if len(c.pos()) == 0 && !c.has("a", "all") {
			return L0, "builtin.read", "只读"
		}
		return L2, "builtin.high", "挂载文件系统"
	case "git":
		s := c.sub()
		if set("status log diff show rev-parse describe ls-files blame shortlog reflog version")[s] {
			return L0, "builtin.read", "只读"
		}
		if s == "clean" || (s == "reset" && c.has("", "hard")) || (s == "push" && c.has("f", "force")) {
			return L2, "data.delete", "丢弃未保存的改动或强制覆盖"
		}
		return L1, "builtin.change", "变更仓库"
	case "tar", "bsdtar":
		mode := ""
		if len(c.args) > 0 {
			mode = strings.TrimPrefix(c.args[0].text, "-")
		}
		if (strings.Contains(mode, "t") && !strings.ContainsAny(mode, "cxru") && !strings.HasPrefix(c.args[0].text, "--")) || c.has("", "list") {
			return L0, "builtin.read", "只读"
		}
		return L1, "builtin.change", "打包或解包"
	case "mysql", "mariadb", "psql", "sqlite3", "mongosh", "mongo", "redis-cli", "clickhouse-client":
		if destructiveSQLRe.MatchString(c.joined()) {
			return L2, "data.delete", "删除数据库数据"
		}
		return L1, "builtin.change", "数据库客户端，语句效果未知"
	case "kubectl", "oc":
		s := c.sub()
		if set("get describe logs top version cluster-info api-resources api-versions explain")[s] {
			return L0, "builtin.read", "只读"
		}
		if s == "delete" || s == "drain" {
			return L2, "data.delete", "删除集群资源"
		}
		return L1, "builtin.change", "变更集群"
	case "sysctl":
		if c.has("wp", "write", "load", "system") || strings.Contains(c.joined(), "=") {
			return L1, "builtin.change", "修改内核参数"
		}
		return L0, "builtin.read", "只读"
	case "command":
		if c.has("vV") {
			return L0, "builtin.read", "只读"
		}
	case "env":
		return L0, "builtin.read", "只读"
	}
	if interpreters[c.name] {
		return L1, "interpreter", "运行解释器，脚本效果未知"
	}
	return L1, "command.unknown", "未收录的命令，按变更处理"
}

func ruleRm(c *command) (Level, string, string) {
	if c.has("", "no-preserve-root") {
		return L3, "system.destroy", "删除根目录"
	}
	pos := c.pos()
	tmpOnly := len(pos) > 0
	for i, p := range pos {
		if isCriticalRoot(p) {
			return L3, "system.destroy", "删除系统关键目录"
		}
		if isBlockDevice(p) {
			return L3, "blockdev.write", "删除块设备节点"
		}
		a := word{}
		for _, w := range c.args {
			if w.text == p {
				a = w
				break
			}
		}
		_ = i
		if !a.static || a.glob && !strings.HasPrefix(p, "/tmp/") || strings.Contains(p, "..") ||
			!(strings.HasPrefix(p, "/tmp/") || strings.HasPrefix(p, "/var/tmp/")) {
			tmpOnly = false
		}
	}
	if tmpOnly && !c.stdinArgs {
		return L1, "data.delete.tmp", "删除临时文件"
	}
	return L2, "data.delete", "删除数据"
}

func ruleFind(e *Engine, v *Verdict, c *command, depth int) (Level, string, string) {
	lvl, rule, reason := L0, "builtin.read", "只读"
	for i := 0; i < len(c.args); i++ {
		switch t := c.args[i].text; t {
		case "-delete":
			return L2, "data.delete", "find 删除匹配的文件"
		case "-fprint", "-fprintf", "-fls", "-fprint0":
			if lvl < L1 {
				lvl, rule, reason = L1, "builtin.change", "写文件"
			}
		case "-exec", "-execdir", "-ok", "-okdir":
			j := i + 1
			for j < len(c.args) && c.args[j].text != ";" && c.args[j].text != "+" {
				j++
			}
			inner := c.args[i+1 : j]
			if len(inner) > 0 {
				if f := e.classifyCall(v, inner, "find "+t+" "+joinWords(inner), depth, true); f != nil {
					v.add(*f)
				}
			}
			if lvl < L1 {
				lvl, rule, reason = L1, "find.exec", "对匹配的文件执行命令，目标由运行时决定"
			}
			i = j
		}
	}
	return lvl, rule, reason
}

func ruleSystemctl(c *command) (Level, string, string) {
	pos := c.pos()
	if len(pos) == 0 {
		return L0, "builtin.read", "只读"
	}
	sub := pos[0]
	if set("status show is-active is-enabled is-failed list-units list-unit-files list-timers list-sockets list-dependencies list-jobs cat get-default show-environment is-system-running")[sub] {
		return L0, "builtin.read", "只读"
	}
	if set("reboot poweroff halt kexec suspend hibernate rescue emergency isolate soft-reboot")[sub] {
		return L2, "builtin.high", "重启或关闭主机"
	}
	for _, u := range pos[1:] {
		if criticalUnitRe.MatchString(u) {
			return L2, "service.critical", "影响远程连接或防火墙的服务"
		}
	}
	return L1, "builtin.change", "变更服务状态"
}

func ruleDocker(c *command) (Level, string, string) {
	pos := c.pos()
	if len(pos) == 0 {
		return L0, "builtin.read", "只读"
	}
	read := set("ps images logs inspect stats version info top port diff history events search ls list df config show")
	sub := pos[0]
	group := set("container image network volume system compose context node service stack plugin secret builder buildx manifest")
	if group[sub] {
		if len(pos) < 2 {
			return L0, "builtin.read", "只读"
		}
		sub2 := pos[1]
		if read[sub2] {
			return L0, "builtin.read", "只读"
		}
		if sub2 == "prune" || (sub == "volume" && sub2 == "rm") ||
			(sub == "compose" && sub2 == "down" && c.has("v", "volumes")) {
			return L2, "data.delete", "删除容器数据卷或批量清理"
		}
		return L1, "builtin.change", "变更容器资源"
	}
	if read[sub] {
		return L0, "builtin.read", "只读"
	}
	if sub == "run" || sub == "exec" || sub == "create" {
		j := c.joined()
		if c.has("", "privileged") || strings.Contains(j, "--pid=host") || strings.Contains(j, "--net=host") && c.has("", "privileged") ||
			regexp.MustCompile(`(^|\s)(-v|--volume)[= ]?/:`).MatchString(j) || strings.Contains(j, "docker.sock") {
			return L2, "container.escape", "容器获得宿主机级权限"
		}
	}
	return L1, "builtin.change", "变更容器"
}
