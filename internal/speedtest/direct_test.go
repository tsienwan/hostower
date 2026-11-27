package speedtest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tsienwan/hostower/internal/kernel/risk"
)

// 测试用的口令在运行时拼出来（不是凭据，只是格式对的 32 位十六进制）。
var testToken = strings.Repeat("0123456789abcdef", 2)

// 直连测速的命令：参数只能是合法的端口、口令、数据量；能从命令里认回来；不会被判成 L0。
func TestDirectCommand(t *testing.T) {
	if strings.ContainsAny(directScript, "'`") {
		t.Fatal("脚本放在命令的单引号里，不能有单引号和反引号")
	}
	for _, r := range directScript {
		if r > 126 {
			t.Fatalf("脚本只用 ASCII，出现了 %q", r)
		}
	}
	e := risk.New()
	for _, install := range []bool{false, true} {
		cmd, err := DirectCommand(DirectPort, testToken, 25, install, 0)
		if err != nil {
			t.Fatal(err)
		}
		// 在主机上开端口、跑一段解释器脚本：至少要有人批准，不能是 L0。
		if got := e.Classify(cmd); got.Level != risk.L1 {
			t.Fatalf("应该是 L1，现在是 %s", got.Level)
		}
		spec, ok := SpecOf(cmd)
		if !ok || spec.Method != MethodDirect || spec.SizeMB != 25 {
			t.Fatalf("没有认回来：%+v %v", spec, ok)
		}
		if port, token, ok := DirectInfo(cmd); !ok || port != DirectPort || token != testToken {
			t.Fatalf("读不回端口和口令：%d %s %v", port, token, ok)
		}
		if _, ok := SpecOf(cmd + "; id"); ok {
			t.Fatal("改过的命令不应该被认成测速")
		}
		if _, ok := SpecOf(strings.Replace(cmd, "CAP//4", "CAP", 1)); ok {
			t.Fatal("改过脚本的命令不应该被认成测速")
		}
	}
	// 一次测几台时，排在后面的多等一会儿；也能认回来。排得太靠后的不收。
	later, err := DirectCommand(DirectPort, testToken, 25, false, 3)
	if err != nil || !strings.Contains(later, fmt.Sprintf("time.time()-t0>%d:", DirectWait+3*DirectStep)) {
		t.Fatalf("排在后面的应该多等：%v", err)
	}
	if spec, ok := SpecOf(later); !ok || spec.Method != MethodDirect {
		t.Fatal("排在后面的命令没有认回来")
	}
	if _, err := DirectCommand(DirectPort, testToken, 25, false, DirectMaxHosts); err == nil {
		t.Fatal("超过一次能测的台数应该拒绝")
	}
	for _, bad := range []struct {
		port, size int
		token      string
	}{{80, 25, testToken}, {70000, 25, testToken}, {DirectPort, 26, testToken}, {DirectPort, 25, "abc"},
		{DirectPort, 25, strings.Repeat("g", 32)}, {DirectPort, 25, testToken[:31] + "'"}} {
		if _, err := DirectCommand(bad.port, bad.token, bad.size, false, 0); err == nil {
			t.Fatalf("应该拒绝：%+v", bad)
		}
	}
}

// 直连测速的输出：数字来自临时测速服务的转述，路由和 ping 来自后面的追踪。
func TestDirectParse(t *testing.T) {
	out := strings.Join([]string{
		"speedtest direct port=28080 size=25",
		"speedtest listen port=28080",
		"speedtest client ip=203.0.113.24",
		"speedtest direct pings=31.20,30.10,35.50 lost=1 proto=http/1.1 conn=wifi",
		"speedtest direct down bytes=25000000 ms=2000 conns=4",
		"speedtest direct up bytes=6250000 ms=5000 conns=2",
		"speedtest direct loaded down=55.50 up=40.25",
		"speedtest direct tcp segs=20000 retrans=150 rtt=30.12",
		"speedtest route 203.0.113.24 traceroute to 203.0.113.24, 30 hops max",
		"speedtest route 203.0.113.24  1  192.0.2.1  0.5 ms  0.7 ms  0.6 ms * 0.4 ms",
		"speedtest route 203.0.113.24  2  * * * * *",
		"speedtest route 203.0.113.24  3  203.0.113.24  31.0 ms 198.51.100.9  33.0 ms * * 32.0 ms",
		"speedtest back 10 packets transmitted, 0 received, 100% packet loss, time 9210ms",
	}, "\n")
	rep := Parse(out)
	c := rep.Detail.Client
	if rep.Error != "" || c == nil {
		t.Fatalf("没有解析出来：%+v", rep)
	}
	if c.IP != "203.0.113.24" || len(c.Pings) != 3 || c.Lost != 1 || c.Proto != "http/1.1" || c.Conn != "wifi" {
		t.Fatalf("浏览器的数字不对：%+v", c)
	}
	if rep.PingMS != 31.2 || rep.DownBps != 100e6 || rep.UpBps != 10e6 {
		t.Fatalf("汇总不对：%v %v %v", rep.PingMS, rep.DownBps, rep.UpBps)
	}
	if c.LoadedDown != 55.5 || c.LoadedUp != 40.25 || c.Segs != 20000 || c.Retrans != 150 || c.TCPRtt != 30.12 {
		t.Fatalf("加载时延迟和重传不对：%+v", c)
	}
	if len(rep.Detail.Routes) != 1 || len(rep.Detail.Routes[0].Hops) != 3 || rep.Detail.Back == nil {
		t.Fatalf("路由不对：%+v", rep.Detail)
	}
	// 每跳探五次：数出发了几次、丢了几次，延迟取回应了的那几次的中位数。
	h := rep.Detail.Routes[0].Hops
	if h[0].Sent != 5 || h[0].Lost != 1 || h[0].MS != 0.55 || h[1].Sent != 5 || h[1].Lost != 5 || h[1].IP != "" ||
		h[2].IP != "203.0.113.24" || h[2].Sent != 5 || h[2].Lost != 2 || h[2].MS != 32 {
		t.Fatalf("每跳的丢包不对：%+v", h)
	}
	// 重发的比发的还多：不收。
	if rep := Parse(strings.Replace(out, "retrans=150", "retrans=99999", 1)); rep.Detail.Client == nil || rep.Detail.Client.Segs != 0 {
		t.Fatalf("离谱的重传数应该丢掉：%+v", rep.Detail.Client)
	}
	for in, want := range map[string]string{
		"speedtest direct port=28080 size=25\nspeedtest error=no-python3":                              "python3",
		"speedtest direct port=28080 size=25\nspeedtest error=port-in-use":                             "占用",
		"speedtest direct port=28080 size=25\nspeedtest listen port=28080\nspeedtest error=no-visitor": "没有等到",
		"speedtest direct port=28080 size=25\nspeedtest listen port=28080":                             "没有拿到",
		// 数字离谱的不收。
		"speedtest direct port=28080 size=25\nspeedtest client ip=203.0.113.24\nspeedtest direct pings=99999999 lost=0 proto=- conn=-": "没有拿到",
		// 地址不是 IP 的不收。
		"speedtest direct port=28080 size=25\nspeedtest client ip=$(id)\nspeedtest direct pings=1.00 lost=0 proto=- conn=-": "没有拿到",
	} {
		if rep := Parse(in); !strings.Contains(rep.Error, want) {
			t.Fatalf("应该报 %q，现在是 %q\n%s", want, rep.Error, in)
		}
	}
	// 别的测速输出里夹带 direct 行不算数。
	if rep := Parse("speedtest client ip=203.0.113.24\nspeedtest direct pings=1.00 lost=0 proto=- conn=-"); rep.Detail.Client != nil {
		t.Fatal("不是直连测速的输出不应该带出浏览器的数字")
	}
}

// 把主机上那段临时测速服务真的跑起来，照着浏览器的做法走一遍。没有 python3 的环境跳过。
func TestDirectScript(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("没有 python3")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	script := strings.NewReplacer("__WAIT__", "20", "__RUN__", "20").Replace(directScript)
	cmd := exec.Command(py, "-c", script, fmt.Sprint(port), testToken, "25")
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	next := func() string {
		select {
		case s, ok := <-lines:
			if !ok {
				t.Fatalf("脚本提前结束了：%s", stderr.String())
			}
			return s
		case <-time.After(15 * time.Second):
			t.Fatalf("等不到输出：%s", stderr.String())
		}
		return ""
	}
	if s := next(); s != fmt.Sprintf("speedtest listen port=%d", port) {
		t.Fatalf("第一行应该是 listen，现在是 %q", s)
	}
	base := fmt.Sprintf("http://127.0.0.1:%d/", port)
	get := func(path string) (int, []byte) {
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, b
	}
	post := func(path string, body []byte) int {
		resp, err := http.Post(base+path, "application/octet-stream", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	// 口令不对：什么都不给。
	for _, p := range []string{"", "ping", strings.Repeat("0", 32) + "/", testToken} {
		if code, body := get(p); code != 404 || len(body) != 0 {
			t.Fatalf("%q 应该是 404，现在是 %d", p, code)
		}
	}
	tok := testToken + "/"
	if code, body := get(tok); code != 200 || !bytes.Contains(body, []byte("const MB=25,")) {
		t.Fatalf("测速页不对：%d", code)
	}
	if s := next(); s != "speedtest client ip=127.0.0.1" {
		t.Fatalf("应该报来源地址，现在是 %q", s)
	}
	if code, _ := get(tok + "ping?1"); code != 200 {
		t.Fatalf("ping：%d", code)
	}
	// 下载：四条连接各要 7 MB，合起来不超过 25 MB。
	total := 0
	for i := 0; i < 4; i++ {
		code, body := get(tok + "down?mb=7")
		if code != 200 {
			t.Fatalf("下载：%d", code)
		}
		total += len(body)
	}
	if total != 25e6 {
		t.Fatalf("下载合起来应该正好 25 MB，现在是 %d", total)
	}
	if code, _ := get(tok + "down?mb=7"); code != 429 {
		t.Fatalf("超过上限应该是 429，现在是 %d", code)
	}
	// 上传：上限是四分之一。
	if code := post(tok+"up", make([]byte, 4<<20)); code != 200 {
		t.Fatalf("上传：%d", code)
	}
	if code := post(tok+"up", make([]byte, 4<<20)); code != 429 {
		t.Fatalf("上传超过上限应该是 429，现在是 %d", code)
	}
	if code := post(tok+"done", []byte(`{"pings":[1e99]}`)); code != 400 {
		t.Fatalf("离谱的数字应该是 400，现在是 %d", code)
	}
	done := `{"pings":[12.345,11.2],"lost":0,"proto":"http/1.1","conn":"<b>","down":{"bytes":25000000,"ms":2000,"conns":4},"up":{"bytes":4194304,"ms":1000,"conns":2},"ld":20.5,"lu":18}`
	if code := post(tok+"done", []byte(done)); code != 200 {
		t.Fatalf("交结果：%d", code)
	}
	// 五行：延迟、下载、上传、加载时延迟、TCP 连接上读的（Linux 上才有，测试只在有 python3 的 Linux 环境里跑）。
	out := "speedtest direct port=1 size=25\nspeedtest client ip=127.0.0.1\n" + next() + "\n" + next() + "\n" + next() + "\n" + next() + "\n" + next()
	rep := Parse(out)
	c := rep.Detail.Client
	if rep.Error != "" || c == nil || len(c.Pings) != 2 || c.Conn != "" || c.Proto != "http/1.1" || rep.DownBps != 100e6 || c.Up == nil || c.Up.Bytes != 4194304 ||
		c.LoadedDown != 20.5 || c.LoadedUp != 18 || c.Segs < 100 || c.Retrans > c.Segs {
		t.Fatalf("脚本的输出解析不对：%+v\n%s", rep, out)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("脚本应该正常退出：%v %s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("交完结果后脚本应该退出")
	}
}
