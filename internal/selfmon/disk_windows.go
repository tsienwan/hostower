//go:build windows

package selfmon

import "golang.org/x/sys/windows"

// diskUsage 返回 path 所在卷的总量和已用量（字节）。
func diskUsage(path string) (total, used uint64, ok bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, false
	}
	var free, all, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &all, &totalFree); err != nil || all < free {
		return 0, 0, false
	}
	return all, all - free, true
}
