package gateway

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 列文件的命令走内核的只读查询：必须是 L0。分发是写文件，不能是 L0。
func TestFileSyncLevels(t *testing.T) {
	e := risk.New()
	c := collectQuery{dir: "/var/log", pattern: "*.log", depth: 3}
	if v := e.Classify(collectCommand(c)); v.Level != risk.L0 {
		t.Fatalf("列文件的命令被判为 %s：%+v", v.Level, v.Findings)
	}
}

func TestParseCollect(t *testing.T) {
	out := "120\t/var/log/a.log\n/var/log/b.log\n7\t/etc/passwd\n9\t/var/log/../x.log\nnot a path\n120\t/var/log/a.log\n"
	files, truncated := parseCollect(out, "/var/log")
	if truncated || len(files) != 2 || files[0].Path != "/var/log/a.log" || files[0].Size != 120 || files[1].Path != "/var/log/b.log" || files[1].Size != -1 {
		t.Fatalf("解析结果不对：%+v", files)
	}
}

// 分发一个文件到两台主机，批准后在两台主机上都能按规则找到，再收集成一个 zip。
func TestDistributeAndCollect(t *testing.T) {
	s := appSetup(t)
	ctx := context.Background()
	// 暂存一个文件（和页面上传走同一个接口）。
	body := "listen: 8080\n"
	up := httptest.NewRecorder()
	ur := httptest.NewRequest("POST", "/api/v1/uploads?name=app.conf", strings.NewReader(body))
	s.upload(up, asOwner(ur))
	var info uploadInfo
	if up.Code != 200 || json.Unmarshal(up.Body.Bytes(), &info) != nil || info.ID == "" {
		t.Fatalf("暂存失败：%d %s", up.Code, up.Body.String())
	}

	in, _ := json.Marshal(map[string]any{"hosts": []string{"web-1", "web-2"}, "dir": "/opt/app", "files": []map[string]string{{"name": "app.conf", "upload": info.ID}}})
	w := httptest.NewRecorder()
	s.fileDistribute(w, asOwner(httptest.NewRequest("POST", "/api/v1/files/distribute", bytes.NewReader(in))))
	var res struct{ Requests []*kernel.Request }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &res) != nil || len(res.Requests) != 1 {
		t.Fatalf("分发：%d %s", w.Code, w.Body.String())
	}
	req := res.Requests[0]
	if len(req.Targets) != 2 || req.Level == risk.L0 || req.State != kernel.StatePending || req.Input == nil || req.Input.SHA256 != info.SHA256 {
		t.Fatalf("分发的请求不对：%+v", req)
	}
	if _, err := s.Kernel.Approve(ctx, req.ID, testOwner, kernel.Proof{Digest: req.Digest, Reauthed: true, Typed: req.Confirm}); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, req.ID, kernel.StateDone)

	// 列出来。
	w = httptest.NewRecorder()
	s.fileCollect(w, asOwner(httptest.NewRequest("GET", "/api/v1/files/collect?hosts=web-1,web-2&dir=/opt/app&pattern=*.conf", nil)))
	var list struct{ Hosts []collectHost }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list.Hosts) != 2 {
		t.Fatalf("列文件：%d %s", w.Code, w.Body.String())
	}
	for _, h := range list.Hosts {
		if h.Error != "" || len(h.Files) != 1 || h.Files[0].Path != "/opt/app/app.conf" || h.Files[0].Size != int64(len(body)) {
			t.Fatalf("%s 上的文件不对：%+v", h.Host, h)
		}
	}

	// 打包下载。
	w = httptest.NewRecorder()
	s.fileCollectDownload(w, asOwner(httptest.NewRequest("GET", "/api/v1/files/collect/download?hosts=web-1,web-2&dir=/opt/app&pattern=*.conf", nil)))
	if w.Code != 200 {
		t.Fatalf("打包：%d %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	if !names["web-1/opt/app/app.conf"] || !names["web-2/opt/app/app.conf"] || len(names) != 2 {
		t.Fatalf("包里的文件不对：%v", names)
	}

	// 不合法的参数。
	for _, q := range []string{
		"hosts=web-1&dir=opt&pattern=*.conf",           // 相对路径
		"hosts=web-1&dir=/opt&pattern=../*",            // 规则里带路径
		"hosts=web-1&dir=/opt&pattern=$(reboot)",       // 夹带命令
		"hosts=nope&dir=/opt&pattern=*.conf",           // 没有这台主机
		"hosts=web-1&dir=/opt&pattern=*.conf&depth=99", // 层数太多
	} {
		w := httptest.NewRecorder()
		s.fileCollect(w, asOwner(httptest.NewRequest("GET", "/api/v1/files/collect?"+strings.ReplaceAll(q, "$", "%24"), nil)))
		if w.Code == 200 {
			t.Errorf("%s 应该被拒绝", q)
		}
	}
}
