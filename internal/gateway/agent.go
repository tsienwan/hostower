package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tsienwan/hostower/internal/agentbin"
	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/kernel"
)

// 增强采集程序的安装和卸载。两者都只是生成一个请求：安装是“把 Hostower 自带的那份程序从标准输入写到主机上的固定位置”，
// 卸载是删掉它。判级、审批、摘要绑定、审计和手敲命令一样。执行成功后采样器会马上重连那台主机，脚本第一轮就发现它并切换过去（卸载同理）。

func (s *Server) hostAgent(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.PathValue("name")
	h, ok := s.Kernel.Inventory().Host(name)
	if !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	var in struct{ Action string }
	if !readJSON(w, r, &in) {
		return
	}
	a := actor(sessionOf(r))
	spec := kernel.CommandSpec{Hosts: []string{name}, Commands: map[string]string{}}
	switch in.Action {
	case "remove":
		spec.Commands[name] = executor.AgentRemoveCommand
		spec.Intent = "卸载增强采集程序"
	case "install":
		if h.Executor == "sim" {
			writeErr(w, 400, "模拟主机上装不了采集程序")
			return
		}
		// 先看主机是什么处理器，取对应的那一份。
		out, code, err := s.Kernel.Query(r.Context(), a, name, "uname -m")
		if err != nil || code != 0 {
			writeErr(w, 502, "读不到主机的处理器架构")
			return
		}
		arch := agentbin.Arch(out)
		data, sum, ok := agentbin.Get(arch)
		if !ok {
			if arch == "" {
				writeErr(w, 400, "不支持这台主机的处理器架构："+strings.TrimSpace(out))
			} else {
				writeErr(w, 400, "这个构建里没有带采集程序（构建前要先运行 go run ./tools/buildagent）")
			}
			return
		}
		// 和上传文件走同一个暂存区：请求的摘要绑定这份文件的大小和 SHA-256。
		b := make([]byte, 16)
		rand.Read(b)
		info := uploadInfo{ID: hex.EncodeToString(b), Name: "hostower-agent " + s.Version + " linux/" + arch, Size: int64(len(data)), SHA256: sum}
		p, _ := s.Kernel.UploadPath(info.ID)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			writeErr(w, 500, "无法创建上传目录")
			return
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			writeErr(w, 500, "无法暂存采集程序")
			return
		}
		meta, _ := json.Marshal(info)
		os.WriteFile(p+".json", meta, 0o600)
		spec.Commands[name] = executor.AgentInstallCommand
		spec.Intent = "安装增强采集程序 " + s.Version + "（linux/" + arch + "）"
		spec.Input = &kernel.Input{Kind: "upload", Label: info.Name, Upload: info.ID, Size: info.Size, SHA256: info.SHA256}
	default:
		writeErr(w, 400, "未知的操作")
		return
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), a, spec)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}
