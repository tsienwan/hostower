//go:build linux

package selfmon

import "syscall"

// diskUsage 返回 path 所在文件系统的总量和已用量（字节）。
func diskUsage(path string) (total, used uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	total = st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	if total < free {
		return 0, 0, false
	}
	return total, total - free, true
}
