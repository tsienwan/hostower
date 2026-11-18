package kernel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// Input 是随命令一起送进标准输入的内容：一条凭据，或一个已上传到塔台的文件。
// 凭据本体永远不出现在命令、请求或审计里——这里只有引用名。
type Input struct {
	Kind  string `json:"kind"`  // secret | upload | envfile | dotenv
	Label string `json:"label"` // 给人看的说明
	// secret：凭据引用名，以及取 JSON 里的哪个字段。
	Ref   string `json:"ref,omitempty"`
	Field string `json:"field,omitempty"`
	// upload：上传文件的编号、大小和摘要。
	Upload string `json:"upload,omitempty"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	// envfile：把若干条应用密钥拼成 KEY=value 的环境文件内容（给 docker --env-file）。
	// dotenv：同样的内容，但值用单引号包起来（给 docker compose 的 .env，防止值里的 $ 被当成变量展开）。
	// 这里只有变量名和引用名。
	Env []EnvRef `json:"env,omitempty"`
}

// EnvRef 是环境文件里的一行：变量名，以及它的值来自哪条凭据。
type EnvRef struct {
	Env string `json:"env"`
	Ref string `json:"ref"`
}

// identity 是输入进入请求摘要的部分：换了凭据或换了文件，批准作废。
func (in *Input) identity() string {
	if in == nil {
		return ""
	}
	id := in.Kind + "|" + in.Ref + "|" + in.Field + "|" + in.Upload + "|" + in.SHA256
	for _, e := range in.Env {
		id += "|" + e.Env + "=" + e.Ref
	}
	return id
}

var uploadIDRe = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

// UploadPath 返回上传文件在数据目录里的位置。
func (k *Kernel) UploadPath(id string) (string, error) {
	if !uploadIDRe.MatchString(id) {
		return "", errors.New("上传编号不合法")
	}
	return filepath.Join(k.cfg.DataDir, "uploads", id), nil
}

func (k *Kernel) openInput(in *Input) (io.ReadCloser, error) {
	switch in.Kind {
	case "secret":
		if k.cfg.Secret == nil {
			return nil, errors.New("凭据存储不可用")
		}
		raw, err := k.cfg.Secret(in.Ref)
		if err != nil {
			return nil, err
		}
		val := string(raw)
		if in.Field != "" {
			var m map[string]string
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, fmt.Errorf("凭据 %s 的格式不对", in.Ref)
			}
			val = m[in.Field]
		}
		if val == "" {
			return nil, fmt.Errorf("凭据 %s 是空的", in.Ref)
		}
		return io.NopCloser(strings.NewReader(val + "\n")), nil
	case "envfile", "dotenv":
		if k.cfg.Secret == nil {
			return nil, errors.New("凭据存储不可用")
		}
		var sb strings.Builder
		for _, e := range in.Env {
			raw, err := k.cfg.Secret(e.Ref)
			if err != nil {
				return nil, err
			}
			if bytes.ContainsAny(raw, "\n\r\x00") {
				return nil, fmt.Errorf("凭据 %s 含换行，不能写进环境文件", e.Ref)
			}
			if in.Kind == "dotenv" {
				if bytes.ContainsRune(raw, '\'') {
					return nil, fmt.Errorf("凭据 %s 含单引号，不能写进 compose 的 .env 文件", e.Ref)
				}
				sb.WriteString(e.Env + "='" + string(raw) + "'\n")
				continue
			}
			sb.WriteString(e.Env + "=" + string(raw) + "\n")
		}
		return io.NopCloser(strings.NewReader(sb.String())), nil
	case "upload":
		p, err := k.UploadPath(in.Upload)
		if err != nil {
			return nil, err
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, errors.New("上传的文件已经不在了，请重新上传")
		}
		if st, err := f.Stat(); err != nil || st.Size() != in.Size {
			f.Close()
			return nil, errors.New("上传的文件在提交之后变了，批准作废")
		}
		return f, nil
	}
	return nil, fmt.Errorf("未知的输入类型 %q", in.Kind)
}

// cleanUploads 删掉超过一天的上传文件。
func (k *Kernel) cleanUploads() {
	dir := filepath.Join(k.cfg.DataDir, "uploads")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// Query 同步执行一条只读命令并返回输出，给“列容器”“看日志”这类页面用。
// 它和 SubmitCommand 走同一套判级：命令不是 L0 就拒绝。
// 查询属于“日常查看”，要不要写审计由所有者的设置决定（见 SetAuditRoutine），默认不写。
func (k *Kernel) Query(ctx context.Context, actor Actor, host, command string) (string, int, error) {
	h, ex, err := k.readable(ctx, actor, host, "查询")
	if err != nil {
		return "", -1, err
	}
	if lvl, _ := k.Assess(h, command, risk.L0); lvl != risk.L0 {
		return "", -1, fmt.Errorf("%w：查询只能是只读命令（这条被判为 %s）", ErrNotAllowed, lvl)
	}
	if err := k.routine(ctx, audit.Entry{Type: "query", Actor: actor.Name, ActorKind: string(actor.Kind), Entry: actor.Entry,
		Host: host, Command: command, Level: "L0"}); err != nil {
		return "", -1, err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var buf bytes.Buffer
	code, err := ex.Exec(cctx, h, command, &limitWriter{w: &buf, n: 1 << 20})
	return buf.String(), code, err
}
