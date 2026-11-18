//go:build !linux

package update

import "errors"

// Exec 在其他系统上不可用。
func Exec(path string) error { return errors.New("自动更新只支持 Linux") }

// Relaunch 只支持 Linux。
func Relaunch(path string) error { return errors.New("从页面重启只支持 Linux") }
