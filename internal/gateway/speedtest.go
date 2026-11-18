package gateway

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/i18n"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/speedtest"
)

// 测速。发起和其他命令一样交给内核：判级（L1）、等待批准、执行、审计都照常；
// 这里只负责生成那条固定的测速命令（需要时前面带上安装 curl 的那段），以及列出解析好的结果。
// 默认用 Speedtest by Ookla；它要求先由所有者接受 Ookla 的许可协议和隐私政策。

func (s *Server) speedtestList(w http.ResponseWriter, r *http.Request) {
	list, err := s.Speedtest.Recent(r.Context(), 300)
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	writeJSON(w, 200, map[string]any{"results": list, "sizes": speedtest.Sizes, "server": speedtest.Server, "retention": s.Speedtest.Retention(),
		"targets": s.Speedtest.Targets(), "maxTargets": speedtest.MaxRoutes, "presets": speedtest.Presets, "geo": s.Speedtest.GeoEnabled(), "geoService": speedtest.GeoService,
		"ookla": s.Speedtest.OoklaLicense(), "searchService": speedtest.ServerSearchService})
}

// speedtestReport 返回一条测速结果和它的明细（延迟各段、传输、网络信息、路由）。
func (s *Server) speedtestReport(w http.ResponseWriter, r *http.Request) {
	k := speedtest.Key{RequestID: r.URL.Query().Get("request"), Host: r.URL.Query().Get("host")}
	res, err := s.Speedtest.Get(r.Context(), k)
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "没有这条测速记录")
		return
	}
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	writeJSON(w, 200, res)
}

// speedtestGeo 查一条测速记录里各跳公网地址的地区和运营商，并用自治系统号重新推测线路。
// 只查这条记录里的地址，不接受任意地址；所有者关掉了这项查询时不发出去。
func (s *Server) speedtestGeo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	res, err := s.Speedtest.Get(r.Context(), speedtest.Key{RequestID: q.Get("request"), Host: q.Get("host")})
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, 404, "没有这条测速记录")
		return
	}
	if err != nil {
		writeErr(w, 500, "读取失败")
		return
	}
	if !s.Speedtest.GeoEnabled() {
		writeJSON(w, 200, map[string]any{"enabled": false, "service": speedtest.GeoService})
		return
	}
	var ips []string
	if res.Detail != nil {
		for _, rt := range res.Detail.Routes {
			for _, h := range rt.Hops {
				if h.IP != "" {
					ips = append(ips, h.IP)
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	geo, err := s.Speedtest.Geo(ctx, ips, i18n.Get(q.Get("lang")).Geo)
	out := map[string]any{"enabled": true, "service": speedtest.GeoService, "hops": geo}
	if err != nil {
		out["error"] = err.Error()
	}
	if res.Detail != nil {
		out["routes"] = speedtest.Relabel(res.Detail.Routes, geo)
	}
	writeJSON(w, 200, out)
}

// speedtestGeoSetting 打开或关掉节点地区查询。只有所有者能改，先写审计、写不进去就不改。
func (s *Server) speedtestGeoSetting(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if old := s.Speedtest.GeoEnabled(); old != in.Enabled {
		if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "speedtest.geo", Actor: sessionOf(r).User.Name, Entry: "web",
			Message: "修改测速路由的节点地区查询",
			Detail:  map[string]any{"ip": s.clientIP(r), "before": old, "after": in.Enabled, "service": speedtest.GeoService}}); err != nil {
			writeErr(w, 500, "写审计失败，没有修改")
			return
		}
		if err := s.Speedtest.SetGeoEnabled(r.Context(), in.Enabled); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"enabled": s.Speedtest.GeoEnabled(), "service": speedtest.GeoService})
}

// speedtestServers 按名字、城市或运营商搜索 Ookla 的测速服务器（面板向 speedtest.net 查询，只发搜索词）。
func (s *Server) speedtestServers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	list, err := s.Speedtest.SearchServers(ctx, r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"servers": list, "service": speedtest.ServerSearchService})
}

// speedtestOokla 接受或撤回 Ookla 的许可协议和隐私政策。只有所有者能改，先写审计、写不进去就不改。
func (s *Server) speedtestOokla(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Accept bool `json:"accept"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if old := s.Speedtest.OoklaLicense(); old.Accepted != in.Accept {
		msg := "接受 Speedtest by Ookla 的许可协议和隐私政策"
		if !in.Accept {
			msg = "撤回对 Speedtest by Ookla 许可协议的接受"
		}
		if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "speedtest.ookla", Actor: sessionOf(r).User.Name, Entry: "web",
			Message: msg, Detail: map[string]any{"ip": s.clientIP(r), "accept": in.Accept, "version": speedtest.OoklaVersion,
				"terms": speedtest.OoklaTerms, "privacy": speedtest.OoklaPrivacy}}); err != nil {
			writeErr(w, 500, "写审计失败，没有修改")
			return
		}
		if err := s.Speedtest.SetOoklaLicense(r.Context(), in.Accept, sessionOf(r).User.Name); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, s.Speedtest.OoklaLicense())
}

// speedtestTargets 读或改路由追踪目标。改只有所有者能做，先写审计、写不进去就不改。
func (s *Server) speedtestTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		if !ownerOnly(w, r) {
			return
		}
		var in struct {
			Targets []speedtest.Target `json:"targets"`
		}
		if !readJSON(w, r, &in) {
			return
		}
		if in.Targets == nil {
			in.Targets = []speedtest.Target{}
		}
		for i := range in.Targets {
			in.Targets[i].Name = strings.TrimSpace(in.Targets[i].Name)
			in.Targets[i].Target = strings.TrimSpace(in.Targets[i].Target)
		}
		if err := speedtest.CheckTargets(in.Targets); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "speedtest.routes", Actor: sessionOf(r).User.Name, Entry: "web",
			Message: "修改测速的路由追踪目标",
			Detail:  map[string]any{"ip": s.clientIP(r), "before": s.Speedtest.Targets(), "after": in.Targets}}); err != nil {
			writeErr(w, 500, "写审计失败，没有修改")
			return
		}
		if err := s.Speedtest.SetTargets(r.Context(), in.Targets); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"targets": s.Speedtest.Targets(), "max": speedtest.MaxRoutes})
}

// speedtestRetention 读或改测速历史的保留规则。改只有所有者能做，先写审计、写不进去就不改。
func (s *Server) speedtestRetention(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		if !ownerOnly(w, r) {
			return
		}
		var in speedtest.Retention
		if !readJSON(w, r, &in) {
			return
		}
		if !in.Valid() {
			writeErr(w, 400, "保留规则不是可选的值")
			return
		}
		if old := s.Speedtest.Retention(); old != in {
			if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "speedtest.retention", Actor: sessionOf(r).User.Name, Entry: "web",
				Message: "修改测速历史的保留规则",
				Detail:  map[string]any{"ip": s.clientIP(r), "before": old, "after": in}}); err != nil {
				writeErr(w, 500, "写审计失败，没有修改")
				return
			}
			if err := s.Speedtest.SetRetention(r.Context(), in); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	writeJSON(w, 200, map[string]any{"retention": s.Speedtest.Retention(), "days": speedtest.DayChoices, "perHost": speedtest.PerHostChoices,
		"stored": s.Speedtest.Count(r.Context())})
}

// speedtestDelete 删掉测速历史里的几条，或者全部。只删测速结果的副本：对应的请求和输出照常留在审计里。
func (s *Server) speedtestDelete(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Items []speedtest.Key `json:"items"`
		All   bool            `json:"all"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if !in.All && (len(in.Items) == 0 || len(in.Items) > 1000) {
		writeErr(w, 400, "没有选中要删的记录")
		return
	}
	msg := "删除测速记录"
	if in.All {
		msg = "清空测速历史"
	}
	if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "speedtest.deleted", Actor: sessionOf(r).User.Name, Entry: "web",
		Message: msg, Detail: map[string]any{"ip": s.clientIP(r), "all": in.All, "items": in.Items}}); err != nil {
		writeErr(w, 500, "写审计失败，没有删除")
		return
	}
	n, err := s.Speedtest.Delete(r.Context(), in.Items, in.All)
	if err != nil {
		writeErr(w, 500, "删除失败")
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": n})
}

// speedtestCheck 看选中的主机上有没有 curl，以及有没有 traceroute 或 tracepath。用的是只读查询（L0），每台最多等 15 秒。
func (s *Server) speedtestCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hosts []string `json:"hosts"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Hosts) > 200 {
		writeErr(w, 400, "一次最多检查 200 台")
		return
	}
	type result struct {
		Host  string `json:"host"`
		Curl  bool   `json:"curl"`
		Trace bool   `json:"trace"`
		Error string `json:"error,omitempty"`
	}
	out := make([]result, len(in.Hosts))
	a := actor(sessionOf(r))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, h := range in.Hosts {
		wg.Add(1)
		go func(i int, h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			output, _, err := s.Kernel.Query(ctx, a, h, speedtest.CheckCommand)
			curl, trace := speedtest.Tools(output)
			out[i] = result{Host: h, Curl: err == nil && curl, Trace: err == nil && trace}
			if err != nil {
				out[i].Error = err.Error()
			}
		}(i, h)
	}
	wg.Wait()
	writeJSON(w, 200, map[string]any{"hosts": out})
}

func (s *Server) speedtestRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hosts  []string `json:"hosts"`
		SizeMB int      `json:"sizeMb"`
		// Install 是要先自动安装缺少的工具的主机（必须也在 Hosts 里）：没有 curl 就装 curl；追踪路由时没有 traceroute 和 tracepath 也装 traceroute。
		Install []string `json:"install"`
		// Routes 为真时顺便对设置里的每个目标跑一次路由追踪。
		Routes bool `json:"routes"`
		// Method 是测速方式：ookla（默认）或 cloudflare。Server 是 Ookla 的服务器编号，0 是自动选最近的。
		Method string `json:"method"`
		Server int    `json:"server"`
		// Threads（single、multi）和 IP（4、6，空是自动）：空的时候用测速方式的默认。
		Threads string `json:"threads"`
		IP      string `json:"ip"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var routes []string
	if in.Routes {
		routes = speedtest.TargetList(s.Speedtest.Targets())
	}
	if len(in.Hosts) == 0 {
		writeErr(w, 400, "没有选中任何主机")
		return
	}
	var plain, withInstall, what string
	method := in.Method
	if method == "" {
		method = speedtest.MethodOokla
	}
	opts, err := speedtest.Options{Threads: in.Threads, IP: in.IP}.Normalize(method)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	switch in.Method {
	case speedtest.MethodOokla, "":
		if !s.Speedtest.OoklaLicense().Accepted {
			writeErr(w, 400, "所有者还没有接受 Speedtest by Ookla 的许可协议和隐私政策")
			return
		}
		if plain, err = speedtest.OoklaCommandWith(in.Server, false, routes, opts); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		withInstall, _ = speedtest.OoklaCommandWith(in.Server, true, routes, opts)
		what = "用 Speedtest 测速（自动选服务器）"
		if in.Server > 0 {
			what = "用 Speedtest 测速（服务器 " + strconv.Itoa(in.Server) + "）"
		}
	case speedtest.MethodCloudflare:
		if plain, err = speedtest.CloudflareCommand(in.SizeMB, false, routes, opts); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		withInstall, _ = speedtest.CloudflareCommand(in.SizeMB, true, routes, opts)
		what = "用 Cloudflare 测速（下载 " + strconv.Itoa(in.SizeMB) + " MB、上传 " + strconv.Itoa(in.SizeMB/4) + " MB）"
	default:
		writeErr(w, 400, "测速方式不对")
		return
	}
	install := map[string]bool{}
	for _, h := range in.Install {
		install[h] = true
	}
	// 选项写进说明：“单线程”或“多线程”，指定了 IP 协议时再加“、IPv4”或“、IPv6”。
	what += map[string]string{"single": "，单线程", "multi": "，多线程"}[opts.Threads]
	if opts.IP != "" {
		what += "、IPv" + opts.IP
	}
	intent := what
	if len(in.Install) > 0 {
		intent = "安装缺少的工具并" + what
	}
	if len(routes) > 0 {
		intent += "，并追踪到 " + strconv.Itoa(len(routes)) + " 个目标的路由"
	}
	spec := kernel.CommandSpec{Hosts: in.Hosts, Commands: map[string]string{}, Snippet: speedtest.Tag, Intent: intent,
		Strategy: kernel.Strategy{Concurrency: 4}}
	for _, h := range in.Hosts {
		spec.Commands[h] = plain
		if install[h] {
			spec.Commands[h] = withInstall
		}
	}
	req, err := s.Kernel.SubmitCommand(r.Context(), actor(sessionOf(r)), spec)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, req)
}
