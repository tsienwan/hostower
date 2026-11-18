package gateway

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/tsienwan/hostower/internal/inventory"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/snippet"
)

// 容器管理没有自己的执行通道：列表是内核的只读查询，其余动作都拼成一条 docker 命令，
// 走和手敲命令完全相同的判级、审批、审计。

var (
	ctrNameRe  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	imageRe    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]{0,254}$`)
	portRe     = regexp.MustCompile(`^([0-9.]+:)?[0-9]{1,5}:[0-9]{1,5}(/tcp|/udp)?$`)
	envKeyRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	volumeRe   = regexp.MustCompile(`^(/[^:\x00]*|[a-zA-Z0-9][a-zA-Z0-9_.-]*):/[^:\x00]*(:ro|:rw)?$`)
	registryRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
	serverRe   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.:-]{0,254}$`)
	unixUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_.-]{0,31}$`)
)

func jsonLines(out string) []map[string]any {
	rows := []map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			rows = append(rows, m)
		}
	}
	return rows
}

func (s *Server) containers(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	a := actor(sessionOf(r))
	out, code, err := s.Kernel.Query(r.Context(), a, host, `docker ps -a --no-trunc --format '{{json .}}'`)
	if err != nil {
		fail(w, err)
		return
	}
	if code != 0 {
		resp := map[string]any{"available": false, "message": strings.TrimSpace(out), "containers": []any{}, "images": []any{}}
		// 账号没有权限访问 docker.sock：查出是哪个账号（只读查询），页面上给出加进 docker 组的命令。
		// 面板自己不做这一步——那是给自己提权（docker 组等同 root），只能由人在主机上执行。
		if strings.Contains(out, "permission denied") && strings.Contains(out, "docker.sock") {
			if u, c, err := s.Kernel.Query(r.Context(), a, host, "id -un"); err == nil && c == 0 && unixUserRe.MatchString(strings.TrimSpace(u)) {
				resp["fix"] = "docker-group"
				resp["user"] = strings.TrimSpace(u)
				resp["local"] = host == inventory.LocalName
			}
		}
		writeJSON(w, 200, resp)
		return
	}
	imgs, _, _ := s.Kernel.Query(r.Context(), a, host, `docker images --format '{{json .}}'`)
	writeJSON(w, 200, map[string]any{"available": true, "containers": jsonLines(out), "images": jsonLines(imgs)})
}

func (s *Server) containerLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("name")
	if !ctrNameRe.MatchString(name) {
		writeErr(w, 400, "容器名不合法")
		return
	}
	out, _, err := s.Kernel.Query(r.Context(), actor(sessionOf(r)), q.Get("host"), "docker logs --tail 300 "+snippet.Quote(name)+" 2>&1")
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"logs": kernel.Redact(out)})
}

type deployBody struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Ports   []string `json:"ports"`
	Env     []string `json:"env"`
	Volumes []string `json:"volumes"`
	Restart string   `json:"restart"`
	Replace bool     `json:"replace"`
	// Secrets 把应用密钥注入成环境变量：只写“变量名 ← 密钥名”，值由塔台在部署时下发。
	Secrets []struct {
		Env    string `json:"env"`
		Secret string `json:"secret"`
	} `json:"secrets"`
}

type containerAction struct {
	Host     string      `json:"host"`
	Action   string      `json:"action"` // start | stop | restart | remove | pull | rmi | login | load | deploy
	Name     string      `json:"name"`
	Image    string      `json:"image"`
	Registry string      `json:"registry"`
	Upload   string      `json:"upload"`
	Deploy   *deployBody `json:"deploy"`
}

func deployCommand(d *deployBody) (string, error) {
	if !ctrNameRe.MatchString(d.Name) {
		return "", errors.New("容器名不合法")
	}
	if !imageRe.MatchString(d.Image) {
		return "", errors.New("镜像地址不合法")
	}
	switch d.Restart {
	case "":
		d.Restart = "unless-stopped"
	case "no", "always", "unless-stopped", "on-failure":
	default:
		return "", errors.New("重启策略不合法")
	}
	args := []string{"docker run -d --name", snippet.Quote(d.Name), "--restart", d.Restart}
	for _, p := range d.Ports {
		if !portRe.MatchString(p) {
			return "", errors.New("端口映射 " + p + " 不合法（形如 8080:80）")
		}
		args = append(args, "-p", p)
	}
	for _, e := range d.Env {
		k, _, ok := strings.Cut(e, "=")
		if !ok || !envKeyRe.MatchString(k) || strings.ContainsAny(e, "\n\r\x00") {
			return "", errors.New("环境变量 " + k + " 不合法（形如 KEY=value）")
		}
		args = append(args, "-e", snippet.Quote(e))
	}
	for _, v := range d.Volumes {
		if !volumeRe.MatchString(v) || strings.Contains(v, "..") {
			return "", errors.New("挂载 " + v + " 不合法（形如 /data/app:/app 或 卷名:/app）")
		}
		args = append(args, "-v", snippet.Quote(v))
	}
	prefix := ""
	if len(d.Secrets) > 0 {
		// 密钥从标准输入写进主机上只有本人可读的环境文件，容器用 --env-file 读它。
		// 路径用 ~ 开头且不加引号，由主机上的 shell 展开成登录账号的家目录。
		file := "~/.hostower/env/" + d.Name + ".env"
		prefix = "umask 077 && mkdir -p ~/.hostower/env && cat > " + file + " && "
		args = append(args, "--env-file", file)
	}
	args = append(args, snippet.Quote(d.Image))
	run := strings.Join(args, " ")
	pull := "docker pull " + snippet.Quote(d.Image)
	if d.Replace {
		return prefix + pull + " && docker rm -f " + snippet.Quote(d.Name) + " && " + run, nil
	}
	return prefix + pull + " && " + run, nil
}

var appSecretRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// envInput 校验“变量名 ← 密钥名”并生成标准输入的描述（不含任何值）。
func (s *Server) envInput(d *deployBody) (*kernel.Input, error) {
	if len(d.Secrets) == 0 {
		return nil, nil
	}
	in := &kernel.Input{Kind: "envfile", Label: d.Name + ".env"}
	seen := map[string]bool{}
	for _, e := range d.Secrets {
		if !envKeyRe.MatchString(e.Env) || seen[e.Env] {
			return nil, errors.New("变量名 " + e.Env + " 不合法或重复")
		}
		seen[e.Env] = true
		if !appSecretRe.MatchString(e.Secret) {
			return nil, errors.New("密钥名 " + e.Secret + " 不合法")
		}
		if _, err := s.Secrets.Get("app/" + e.Secret); err != nil {
			return nil, errors.New("应用密钥 " + e.Secret + " 不存在或读不出来")
		}
		in.Env = append(in.Env, kernel.EnvRef{Env: e.Env, Ref: "app/" + e.Secret})
	}
	return in, nil
}

// ---------- 应用密钥（只写不读） ----------

func (s *Server) listAppSecrets(w http.ResponseWriter, r *http.Request) {
	list, err := s.Secrets.List()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	out := []map[string]string{}
	for _, m := range list {
		if name, ok := strings.CutPrefix(m.Name, "app/"); ok {
			out = append(out, map[string]string{"name": name, "updatedAt": m.UpdatedAt})
		}
	}
	writeJSON(w, 200, map[string]any{"secrets": out, "available": s.Secrets.Available()})
}

func (s *Server) putAppSecret(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct{ Name, Value string }
	if !readJSON(w, r, &in) {
		return
	}
	if !appSecretRe.MatchString(in.Name) {
		writeErr(w, 400, "名称只能用小写字母、数字、点、下划线、连字符")
		return
	}
	if in.Value == "" || len(in.Value) > 16<<10 || strings.ContainsAny(in.Value, "\n\r\x00") {
		writeErr(w, 400, "值不能为空、不能含换行，最长 16 KB")
		return
	}
	if err := s.Secrets.Put("app/"+in.Name, []byte(in.Value)); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "secret.updated", Message: "写入应用密钥 " + in.Name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteAppSecret(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.URL.Query().Get("name")
	if !appSecretRe.MatchString(name) {
		writeErr(w, 400, "名称不合法")
		return
	}
	if err := s.Secrets.Delete("app/" + name); err != nil {
		writeErr(w, 500, "删除失败")
		return
	}
	s.audit(r, audit.Entry{Type: "secret.deleted", Message: "删除应用密钥 " + name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) containerAct(w http.ResponseWriter, r *http.Request) {
	var in containerAction
	if !readJSON(w, r, &in) {
		return
	}
	spec := kernel.CommandSpec{Hosts: []string{in.Host}, Commands: map[string]string{}}
	var cmd string
	needName := func() bool {
		if !ctrNameRe.MatchString(in.Name) {
			writeErr(w, 400, "容器名不合法")
			return false
		}
		return true
	}
	needImage := func() bool {
		if !imageRe.MatchString(in.Image) {
			writeErr(w, 400, "镜像地址不合法")
			return false
		}
		return true
	}
	switch in.Action {
	case "start", "stop", "restart":
		if !needName() {
			return
		}
		cmd = "docker " + in.Action + " " + snippet.Quote(in.Name)
		spec.Intent = map[string]string{"start": "启动", "stop": "停止", "restart": "重启"}[in.Action] + "容器 " + in.Name
	case "remove":
		if !needName() {
			return
		}
		cmd = "docker rm -f " + snippet.Quote(in.Name)
		spec.Intent = "删除容器 " + in.Name
	case "pull":
		if !needImage() {
			return
		}
		cmd = "docker pull " + snippet.Quote(in.Image)
		spec.Intent = "拉取镜像 " + in.Image
	case "rmi":
		if !needImage() {
			return
		}
		cmd = "docker rmi " + snippet.Quote(in.Image)
		spec.Intent = "删除镜像 " + in.Image
	case "deploy":
		if in.Deploy == nil {
			writeErr(w, 400, "缺少部署参数")
			return
		}
		c, err := deployCommand(in.Deploy)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		input, err := s.envInput(in.Deploy)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		cmd, spec.Input = c, input
		spec.Intent = "部署容器 " + in.Deploy.Name + "（" + in.Deploy.Image + "）"
	case "login":
		reg, err := s.registry(in.Registry)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		// 密码从标准输入送进去：不出现在命令行、请求和审计里。
		cmd = "docker login " + snippet.Quote(reg.Server) + " -u " + snippet.Quote(reg.Username) + " --password-stdin"
		spec.Intent = "登录镜像仓库 " + reg.Server
		spec.Input = &kernel.Input{Kind: "secret", Label: "仓库 " + in.Registry + " 的密码", Ref: "registry/" + in.Registry, Field: "password"}
	case "load":
		p, err := s.Kernel.UploadPath(in.Upload)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		meta, err := os.ReadFile(p + ".json")
		var up uploadInfo
		if err != nil || json.Unmarshal(meta, &up) != nil {
			writeErr(w, 400, "上传的文件不存在或已过期")
			return
		}
		cmd = "docker load"
		spec.Intent = "导入镜像包 " + up.Name
		spec.Input = &kernel.Input{Kind: "upload", Label: up.Name, Upload: up.ID, Size: up.Size, SHA256: up.SHA256}
	default:
		writeErr(w, 400, "未知的操作")
		return
	}
	spec.Commands[in.Host] = cmd
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), spec)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}

// ---------- 镜像仓库（凭据只写不读） ----------

type registryCred struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

func (s *Server) registry(name string) (registryCred, error) {
	var c registryCred
	if !registryRe.MatchString(name) {
		return c, errors.New("仓库名称不合法")
	}
	raw, err := s.Secrets.Get("registry/" + name)
	if err != nil {
		return c, err
	}
	if json.Unmarshal(raw, &c) != nil || c.Server == "" {
		return c, errors.New("仓库凭据格式不对")
	}
	c.Password = ""
	return c, nil
}

func (s *Server) listRegistries(w http.ResponseWriter, r *http.Request) {
	list, err := s.Secrets.List()
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	out := []map[string]string{}
	for _, m := range list {
		name, ok := strings.CutPrefix(m.Name, "registry/")
		if !ok {
			continue
		}
		item := map[string]string{"name": name, "updatedAt": m.UpdatedAt}
		if c, err := s.registry(name); err == nil {
			item["server"], item["username"] = c.Server, c.Username
		}
		out = append(out, item)
	}
	writeJSON(w, 200, map[string]any{"registries": out, "available": s.Secrets.Available()})
}

func (s *Server) putRegistry(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct{ Name, Server, Username, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	if !registryRe.MatchString(in.Name) || !serverRe.MatchString(in.Server) || in.Username == "" || in.Password == "" ||
		strings.ContainsAny(in.Username+in.Password, "\n\r\x00") {
		writeErr(w, 400, "需要名称（小写字母数字）、仓库地址、用户名和密码或访问令牌")
		return
	}
	body, _ := json.Marshal(registryCred{Server: in.Server, Username: in.Username, Password: in.Password})
	if err := s.Secrets.Put("registry/"+in.Name, body); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.audit(r, audit.Entry{Type: "secret.updated", Message: "写入镜像仓库凭据 " + in.Name + "（" + in.Server + "）"})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) deleteRegistry(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := r.URL.Query().Get("name")
	if !registryRe.MatchString(name) {
		writeErr(w, 400, "仓库名称不合法")
		return
	}
	if err := s.Secrets.Delete("registry/" + name); err != nil {
		writeErr(w, 500, "删除失败")
		return
	}
	s.audit(r, audit.Entry{Type: "secret.deleted", Message: "删除镜像仓库凭据 " + name})
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------- 镜像包上传 ----------

type uploadInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

const maxUpload = 4 << 30

// upload 把镜像包流式存到塔台的数据目录（不整个读进内存），算出摘要。
// 它只是暂存：送到主机要另外提交“导入镜像”请求并经过批准。
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	name := filepath.Base(strings.ReplaceAll(r.URL.Query().Get("name"), "\\", "/"))
	if name == "" || name == "." || len(name) > 200 {
		name = "upload"
	}
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	p, _ := s.Kernel.UploadPath(id)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		writeErr(w, 500, "无法创建上传目录")
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		writeErr(w, 500, "无法保存上传的文件")
		return
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r.Body)
	f.Close()
	if err != nil || n == 0 {
		os.Remove(p)
		msg := "上传中断"
		if n >= maxUpload {
			msg = "文件超过 4 GB 上限"
		}
		writeErr(w, 400, msg)
		return
	}
	info := uploadInfo{ID: id, Name: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
	meta, _ := json.Marshal(info)
	os.WriteFile(p+".json", meta, 0o600)
	s.audit(r, audit.Entry{Type: "upload.stored", Message: "暂存上传的文件 " + name, Detail: map[string]any{"size": n, "sha256": info.SHA256}})
	writeJSON(w, 200, info)
}
