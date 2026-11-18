//go:build linux

package update

import (
	"os"
	"syscall"
)

// Relaunch 用同样的参数和环境原地重新启动（从页面重启时用）。成功时不返回。
func Relaunch(path string) error {
	return syscall.Exec(path, os.Args, os.Environ())
}

// Exec 把当前进程原地换成 path（进程号不变，容器不会因此停止）。成功时不返回。
func Exec(path string) error {
	env := append(os.Environ(), HandoffEnv+"=1")
	return syscall.Exec(path, os.Args, env)
}
