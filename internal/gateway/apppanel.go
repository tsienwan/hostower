package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tsienwan/hostower/internal/apps"
	"github.com/tsienwan/hostower/internal/kernel/audit"
)

// 应用的面板地址设置（见 apps.PanelSetting）：存在塔台的 settings 表里，键是 apps.panel:<主机>/<实例>。
// 只是“按钮打开哪个地址”，不碰主机，所以不走审批；但只有所有者能改，每次改动先写审计，写不进去就不改。

const appPanelPrefix = "apps.panel:"

func appPanelKey(host, inst string) string { return appPanelPrefix + host + "/" + inst }

// appPanels 读出全部实例的面板地址设置。读不到时返回空表（按钮照默认地址拼）。
func (s *Server) appPanels() map[string]*apps.PanelSetting {
	out := map[string]*apps.PanelSetting{}
	if s.DB == nil {
		return out
	}
	rows, err := s.DB.Query(`SELECT key, value FROM settings WHERE substr(key, 1, ?) = ?`, len(appPanelPrefix), appPanelPrefix)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) != nil {
			continue
		}
		if p := apps.ParsePanelSetting(v); p != nil {
			out[strings.TrimPrefix(k, appPanelPrefix)] = p
		}
	}
	return out
}

func (s *Server) appPanel(host, inst string) *apps.PanelSetting {
	if s.DB == nil {
		return nil
	}
	var v string
	if err := s.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, appPanelKey(host, inst)).Scan(&v); err != nil {
		return nil
	}
	return apps.ParsePanelSetting(v)
}

type appPanelBody struct {
	Host     string            `json:"host"`
	Instance string            `json:"instance"`
	Panel    apps.PanelSetting `json:"panel"`
}

// appPanelSet 设置（或恢复默认）一个实例的面板地址。实例必须真的装在这台主机上、清单里有网页面板。
func (s *Server) appPanelSet(w http.ResponseWriter, r *http.Request) {
	if !ownerOnly(w, r) {
		return
	}
	var in appPanelBody
	if !readJSON(w, r, &in) {
		return
	}
	if !apps.InstanceRe.MatchString(in.Instance) {
		writeErr(w, 400, "实例名不合法")
		return
	}
	if _, ok := s.Kernel.Inventory().Host(in.Host); !ok {
		writeErr(w, 404, "主机不存在")
		return
	}
	if s.DB == nil {
		writeErr(w, 500, "设置存储不可用")
		return
	}
	keep, err := in.Panel.Normalize()
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ha := s.installedRecords(r.Context(), actor(sessionOf(r)), []string{in.Host})[0]
	if ha.Error != "" {
		writeErr(w, 400, ha.Error)
		return
	}
	var found *installedView
	for i := range ha.Installed {
		if ha.Installed[i].Instance == in.Instance {
			found = &ha.Installed[i]
		}
	}
	if found == nil || found.Guide == nil || !hasPanel(found.Guide) {
		writeErr(w, 404, "这台主机上没有这个实例，或者它没有网页面板")
		return
	}
	old := s.appPanel(in.Host, in.Instance)
	if (old == nil && !keep) || (old != nil && keep && *old == in.Panel) {
		writeJSON(w, 200, map[string]any{"panel": old})
		return
	}
	detail := map[string]any{"ip": s.clientIP(r), "instance": in.Instance, "app": found.App}
	if old != nil {
		detail["before"] = old
	}
	msg := "恢复应用面板的默认地址"
	if keep {
		detail["after"] = in.Panel
		msg = "设置应用的面板地址"
	}
	if _, err := s.Kernel.Audit().Append(r.Context(), audit.Entry{Type: "app.panel", Actor: sessionOf(r).User.Name, Entry: "web", Host: in.Host,
		Message: msg, Detail: detail}); err != nil {
		writeErr(w, 500, "写审计失败，没有修改")
		return
	}
	if keep {
		b, _ := json.Marshal(in.Panel)
		err = s.saveSetting(appPanelKey(in.Host, in.Instance), string(b))
	} else {
		_, err = s.DB.Exec(`DELETE FROM settings WHERE key = ?`, appPanelKey(in.Host, in.Instance))
	}
	if err != nil {
		writeErr(w, 500, "保存失败")
		return
	}
	if !keep {
		writeJSON(w, 200, map[string]any{"panel": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"panel": in.Panel})
}

func hasPanel(g *apps.GuideView) bool {
	for _, o := range g.Open {
		if o.Panel {
			return true
		}
	}
	return false
}
