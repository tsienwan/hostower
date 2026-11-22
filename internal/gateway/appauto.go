package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
)

// 应用自动发起更新：定时检查已安装的实例，发现新版本就以“系统”的身份提交更新请求，等人批准。
// 它不会自己批准任何东西——L1 以上的请求总要有人批准，这条由内核保证，这里也没有办法绕开。
// 同一个实例的同一个新版本只发起一次：请求还在等批准或执行中就不再提交；被人拒绝过就不再提交；
// 过期、失败、被取消的，隔一天再提交一次。

const (
	appAutoKey      = "apps.autoupdate"
	appAutoStateKey = "apps.autoupdate.state"
	appAutoEvery    = 6 * time.Hour
	appAutoFirst    = 2 * time.Minute
	appAutoRetry    = 24 * time.Hour
	appAutoForget   = 30 * 24 * time.Hour
)

// appAutoActor 是自动发起更新时的发起人。它只能发起，不能批准。
var appAutoActor = kernel.Actor{ID: "system:app-updates", Name: "应用自动更新", Kind: kernel.System, Entry: "schedule"}

type appAutoSent struct {
	Req string    `json:"req"`
	At  time.Time `json:"at"`
	// Denied 记下“人拒绝过这个版本”：内核只在内存里留最近的几百个请求，旧请求查不到以后也要记得。
	Denied bool `json:"denied,omitempty"`
}

type appAuto struct {
	mu        sync.Mutex
	enabled   bool
	kick      chan struct{}
	running   bool
	lastRun   time.Time
	submitted int // 上一次检查提交了几个请求
	lastErr   string
	sent      map[string]appAutoSent // 主机/实例@版本 → 发起的请求
}

func (s *Server) loadAppAuto() {
	s.appAuto = &appAuto{kick: make(chan struct{}, 1), sent: map[string]appAutoSent{}}
	if s.DB == nil {
		return
	}
	var v string
	if s.DB.QueryRow("SELECT value FROM settings WHERE key = ?", appAutoKey).Scan(&v) == nil {
		s.appAuto.enabled = v == "1"
	}
	if s.DB.QueryRow("SELECT value FROM settings WHERE key = ?", appAutoStateKey).Scan(&v) == nil {
		json.Unmarshal([]byte(v), &s.appAuto.sent)
	}
}

func (s *Server) saveSetting(key, value string) error {
	if s.DB == nil {
		return errors.New("设置存储不可用")
	}
	_, err := s.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// RunAppUpdates 是自动发起更新的后台循环：启动后两分钟查一次，之后每 6 小时查一次；页面上点“立即检查”会提前查。
func (s *Server) RunAppUpdates(ctx context.Context) {
	t := time.NewTimer(appAutoFirst)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.appAuto.kick:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
		}
		s.appAuto.mu.Lock()
		on := s.appAuto.enabled
		s.appAuto.mu.Unlock()
		if on {
			s.autoUpdateOnce(ctx)
		}
		t.Reset(appAutoEvery)
	}
}

func (s *Server) autoUpdateOnce(ctx context.Context) {
	aa := s.appAuto
	aa.mu.Lock()
	if aa.running {
		aa.mu.Unlock()
		return
	}
	aa.running = true
	aa.mu.Unlock()
	submitted, err := s.autoUpdateCheck(ctx)
	aa.mu.Lock()
	defer aa.mu.Unlock()
	aa.running, aa.lastRun, aa.submitted, aa.lastErr = false, time.Now(), submitted, ""
	if err != nil {
		aa.lastErr = err.Error()
		s.Log.Warn("应用自动更新检查失败", "err", err)
	}
	if raw, err := json.Marshal(aa.sent); err == nil {
		s.saveSetting(appAutoStateKey, string(raw))
	}
}

// shouldSend 判断一个“实例的新版本”要不要（再）发起更新请求。调用时持有锁。
func (aa *appAuto) shouldSend(k *kernel.Kernel, key string, now time.Time) bool {
	prev, ok := aa.sent[key]
	if !ok {
		return true
	}
	if prev.Denied {
		return false // 人拒绝过这个版本：不再打扰
	}
	r, err := k.Get(prev.Req)
	if err == nil {
		switch r.State {
		case kernel.StatePending, kernel.StateRunning:
			return false // 还在等批准或执行中
		case kernel.StateDenied:
			prev.Denied = true
			aa.sent[key] = prev // 存下来：这个请求以后从内核里掉出去了也记得
			return false
		}
	}
	return now.Sub(prev.At) >= appAutoRetry
}

func (s *Server) autoUpdateCheck(ctx context.Context) (int, error) {
	var hosts []string
	for _, h := range s.Kernel.Inventory().Hosts() {
		hosts = append(hosts, h.Name)
	}
	if len(hosts) == 0 {
		return 0, nil
	}
	now := time.Now()
	type group struct {
		version string
		targets []appTarget
		keys    []string
	}
	groups := map[string]*group{}
	var order []string
	var failed []string
	aa := s.appAuto
	for start := 0; start < len(hosts); start += maxAppTargets {
		end := min(start+maxAppTargets, len(hosts))
		for _, ha := range s.installedOn(ctx, appAutoActor, hosts[start:end]) {
			if ha.Error != "" {
				failed = append(failed, ha.Host)
				continue
			}
			for _, x := range ha.Installed {
				if (x.Update == "" && !x.Rebuilt) || !x.Known {
					continue
				}
				key := ha.Host + "/" + x.Instance + "@" + x.Update
				if x.Update == "" {
					key = ha.Host + "/" + x.Instance + "@" + x.Version + "#" + x.Remote // 同一个新构建只发起一次
				}
				aa.mu.Lock()
				send := aa.shouldSend(s.Kernel, key, now)
				aa.mu.Unlock()
				if !send {
					continue
				}
				target := x.Update
				if target == "" {
					target = x.Version
				}
				gk := x.App + "@" + target
				g := groups[gk]
				if g == nil {
					g = &group{version: target}
					groups[gk] = g
					order = append(order, gk)
				}
				g.targets = append(g.targets, appTarget{Host: ha.Host, Instance: x.Instance})
				g.keys = append(g.keys, key)
			}
		}
	}
	sort.Strings(order)
	submitted := 0
	var errs []string
	for _, gk := range order {
		g := groups[gk]
		for start := 0; start < len(g.targets); start += maxAppTargets {
			end := min(start+maxAppTargets, len(g.targets))
			p, err := s.planAppAction(ctx, appAutoActor, actUpdate, g.version, g.targets[start:end])
			if err != nil {
				errs = append(errs, gk+"："+err.Error())
				continue
			}
			reqs, err := s.submitPlanned(ctx, appAutoActor, p.plan, kernel.Strategy{Concurrency: 4, StopOnError: true}, p.intent, "app-auto:"+gk)
			submitted += len(reqs)
			if err != nil {
				errs = append(errs, gk+"："+err.Error())
			}
			if len(reqs) > 0 {
				aa.mu.Lock()
				for _, key := range g.keys[start:end] {
					aa.sent[key] = appAutoSent{Req: reqs[0].ID, At: now}
				}
				aa.mu.Unlock()
			}
		}
	}
	aa.mu.Lock()
	for k, v := range aa.sent {
		if now.Sub(v.At) > appAutoForget {
			delete(aa.sent, k)
		}
	}
	aa.mu.Unlock()
	if len(errs) > 0 {
		return submitted, errors.New(errs[0])
	}
	if len(failed) > 0 && len(failed) == len(hosts) {
		return submitted, errors.New("所有主机都查不到已安装的应用")
	}
	return submitted, nil
}

func (s *Server) appAutoStatus(w http.ResponseWriter, r *http.Request) {
	aa := s.appAuto
	aa.mu.Lock()
	defer aa.mu.Unlock()
	out := map[string]any{"enabled": aa.enabled, "running": aa.running, "intervalHours": int(appAutoEvery / time.Hour), "submitted": aa.submitted}
	if !aa.lastRun.IsZero() {
		out["lastRun"] = aa.lastRun.UnixMilli()
	}
	if aa.lastErr != "" {
		out["error"] = aa.lastErr
	}
	writeJSON(w, 200, out)
}

func (s *Server) appAutoSet(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in struct {
		Enabled bool `json:"enabled"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	v, msg := "0", "关闭应用自动发起更新"
	if in.Enabled {
		v, msg = "1", "打开应用自动发起更新：发现新版本时以系统身份提交更新请求，等人批准"
	}
	if err := s.saveSetting(appAutoKey, v); err != nil {
		writeErr(w, 500, "保存失败")
		return
	}
	s.audit(r, audit.Entry{Type: "apps.autoupdate", Message: msg, Detail: map[string]any{"enabled": in.Enabled}})
	s.appAuto.mu.Lock()
	was := s.appAuto.enabled
	s.appAuto.enabled = in.Enabled
	s.appAuto.mu.Unlock()
	if in.Enabled && !was {
		s.kickAppAuto()
	}
	s.appAutoStatus(w, r)
}

func (s *Server) appAutoCheck(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	s.appAuto.mu.Lock()
	on := s.appAuto.enabled
	s.appAuto.mu.Unlock()
	if !on {
		writeErr(w, 400, "自动发起更新没有打开")
		return
	}
	s.kickAppAuto()
	s.appAutoStatus(w, r)
}

func (s *Server) kickAppAuto() {
	select {
	case s.appAuto.kick <- struct{}{}:
	default:
	}
}
