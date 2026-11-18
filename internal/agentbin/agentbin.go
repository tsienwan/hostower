// Package agentbin 带着编译好的增强采集程序（hostower-agent），按主机的处理器架构取用。
//
// 这些文件在构建 Hostower 之前生成（go run ./tools/buildagent），随后被编译进 Hostower 的可执行文件里：
// 安装到主机上的采集程序就是 Hostower 自己带着的那一份，不从网络上下载，所以它的来源和完整性
// 跟 Hostower 本身一致。没有生成过的构建（比如直接 go build）里没有它，页面上会说明。
package agentbin

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Arch 把 uname -m 的输出换成 Go 的架构名；不支持的返回空。
func Arch(uname string) string {
	switch strings.TrimSpace(uname) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	}
	return ""
}

// Get 返回某个架构的采集程序和它的 SHA-256。
func Get(arch string) (data []byte, sum string, ok bool) {
	if arch == "" {
		return nil, "", false
	}
	data, err := dist.ReadFile("dist/hostower-agent_linux_" + arch)
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	h := sha256.Sum256(data)
	return data, hex.EncodeToString(h[:]), true
}

// Available 报告这个构建里带了哪些架构的采集程序。
func Available() []string {
	var out []string
	for _, a := range []string{"amd64", "arm64"} {
		if _, _, ok := Get(a); ok {
			out = append(out, a)
		}
	}
	return out
}
