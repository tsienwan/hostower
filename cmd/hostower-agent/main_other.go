//go:build !linux

// hostower-agent 只在 Linux 上有意义：它读的是 /proc 和 /sys。
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "hostower-agent 只支持 Linux")
	os.Exit(1)
}
