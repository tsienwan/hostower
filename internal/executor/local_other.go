//go:build !linux

package executor

import "errors"

// Local 只在 Linux 上可用：采样、判级规则和容器管理针对的都是 Linux 主机。
type Local struct{ Executor }

// NewLocal 在其他系统上总是返回 nil。
func NewLocal() *Local { return nil }

// NewHostLocal 在其他系统上不可用。
func NewHostLocal() (*Local, error) { return nil, errors.New("宿主机模式只支持 Linux") }
