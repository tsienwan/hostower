// buildagent 把增强采集程序（cmd/hostower-agent）编译成 Linux 上各架构的静态可执行文件，
// 放到 internal/agentbin/dist/ 下。之后再构建 Hostower，这些文件就被编译进去了。
//
//	go run ./tools/buildagent            版本号写 dev
//	go run ./tools/buildagent v1.2.3     版本号和 Hostower 的发布版本一致
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	version := "dev"
	if len(os.Args) > 1 {
		version = os.Args[1]
	}
	out := filepath.Join("internal", "agentbin", "dist")
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		target := filepath.Join(out, "hostower-agent_linux_"+arch)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", target, "./cmd/hostower-agent")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fail(err)
		}
		if st, err := os.Stat(target); err == nil {
			fmt.Printf("%s  %.1f MB\n", target, float64(st.Size())/1e6)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "错误：", err)
	os.Exit(1)
}
