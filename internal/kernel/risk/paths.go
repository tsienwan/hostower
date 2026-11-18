package risk

import (
	"regexp"
	"strings"
)

var secretRes = compileAll(
	`(^|/)\.ssh/id_[^/]*$`,
	`(^|/)\.ssh/[^/]*_key$`,
	`^/etc/ssh/ssh_host_[^/]*_key$`,
	`^/etc/g?shadow-?$`,
	`(^|/)\.aws/credentials$`,
	`(^|/)\.(netrc|pgpass|git-credentials|my\.cnf)$`,
	`(^|/)\.kube/config$`,
	`(^|/)\.docker/config\.json$`,
	`^/etc/ssl/private(/|$)`,
	`/privkey[^/]*\.pem$`,
	`(^|/)\.gnupg(/|$)`,
	`^/proc/[^/]+/environ$`,
	`^/run/secrets(/|$)`,
)

var criticalConfigRes = compileAll(
	`^/etc/ssh(/|$)`,
	`^/etc/sudoers(\.d)?(/|$)`,
	`^/etc/(passwd|group|g?shadow)-?$`,
	`^/etc/pam\.d(/|$)`,
	`^/etc/fstab$`,
	`^/etc/hosts\.(allow|deny)$`,
	`^/etc/(nftables|iptables|ufw|firewalld)(\.conf)?(/|$)`,
	`^/etc/security(/|$)`,
	`^/boot(/|$)`,
	`(^|/)\.ssh/authorized_keys2?$`,
	`(^|/)\.ssh/config$`,
)

var blockDevRe = regexp.MustCompile(`^/dev/(sd|vd|hd|xvd|nvme|mmcblk|dm-|md|mapper/|disk/|loop|sr|zd|rbd|nbd)`)

var sshDirRe = regexp.MustCompile(`(^|/)\.ssh/?$`)
var sshDirInsideRe = regexp.MustCompile(`(^|/)\.ssh/`)

func compileAll(exprs ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(exprs))
	for i, e := range exprs {
		out[i] = regexp.MustCompile(e)
	}
	return out
}

func matchAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// isSecretPath 报告路径是否是私钥或凭据文件。公钥（.pub）不算。
func isSecretPath(p string) bool {
	if strings.HasSuffix(p, ".pub") {
		return false
	}
	return matchAny(secretRes, p)
}

// isSSHDirWhole 报告参数是否指向整个 .ssh 目录，或用通配符扫它里面的文件。
func isSSHDirWhole(p string, glob bool) bool {
	if sshDirRe.MatchString(p) {
		return true
	}
	return glob && sshDirInsideRe.MatchString(p)
}

func isCriticalConfig(p string) bool { return matchAny(criticalConfigRes, p) }

func isBlockDevice(p string) bool { return blockDevRe.MatchString(p) }

func harmlessSink(p string) bool {
	switch p {
	case "/dev/null", "/dev/stdout", "/dev/stderr", "/dev/tty", "/dev/zero":
		return true
	}
	return strings.HasPrefix(p, "/dev/fd/")
}

// criticalRoots 是删掉或递归改权限就等于毁掉系统的目录。
var criticalRoots = set("/ /bin /boot /dev /etc /home /lib /lib32 /lib64 /opt /proc /root /run /sbin /srv /sys /usr /var ~ $HOME /usr/bin /usr/lib /usr/sbin /var/lib")

func isCriticalRoot(p string) bool {
	p = strings.TrimSuffix(p, "/*")
	p = strings.TrimSuffix(p, "/.")
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		p = "/"
	}
	return criticalRoots[p]
}

// ReadPathLevel 给“读文件 / 列目录”这类非 shell 操作判级。
func ReadPathLevel(p string) (Level, string) {
	if isSecretPath(p) {
		return L3, "读取私钥或凭据文件"
	}
	return L0, "只读"
}

// WritePathLevel 给“写文件”判级。
func WritePathLevel(p string) (Level, string) {
	switch {
	case isBlockDevice(p):
		return L3, "写块设备"
	case isSecretPath(p):
		return L2, "覆盖私钥或凭据文件"
	case isCriticalConfig(p):
		return L2, "修改登录、提权或防火墙相关配置"
	}
	return L1, "写文件"
}
