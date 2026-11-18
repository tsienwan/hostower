//go:build !linux && !windows

package selfmon

// diskUsage 在其他系统上暂不支持。
func diskUsage(path string) (total, used uint64, ok bool) { return 0, 0, false }
