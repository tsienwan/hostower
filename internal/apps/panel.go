package apps

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// 面板地址：“已安装”列表上的“打开面板”按钮默认按清单里标了 panel 的访问入口、
// 用这个实例在主机上记的参数（监听地址、端口）现拼出来，改过设置以后自然跟着变。
// 有三种情况拼不对，由所有者给这个实例设一个面板地址（存在塔台的设置里，改动写审计）：
//   - 端口或路径是在应用自己的页面里改的（比如 3x-ui 的面板路径），Hostower 看不到：填端口、路径覆盖；
//   - 前面套了反向代理或只绑在 127.0.0.1 上：直接填完整的地址（https://panel.example.com/xui/）；
//   - 经由 SSH 隧道访问：页面给出一条 ssh -L 命令在自己电脑上运行，按钮打开 http://localhost:<本地端口>。
// 地址里不允许出现账号、密码或查询参数：按钮链接里绝不能带凭据。

// 面板地址的访问方式。
const (
	PanelDirect = "direct" // 直接访问主机地址（可以覆盖端口、路径）
	PanelURL    = "url"    // 用填好的完整地址（反向代理、域名）
	PanelSSH    = "ssh"    // 经由 SSH 隧道，打开本机端口
)

// PanelSetting 是一个实例的面板地址设置。持久化为 JSON，带版本号 V。
type PanelSetting struct {
	V    int    `json:"v"`
	Mode string `json:"mode"`
	// URL 是 url 方式的完整地址。
	URL string `json:"url,omitempty"`
	// Port、Path 覆盖清单里的端口和路径（direct、ssh 方式）；空表示照清单和参数。
	Port string `json:"port,omitempty"`
	Path string `json:"path,omitempty"`
	// LocalPort 是 ssh 方式在自己电脑上用的端口；空表示和应用端口相同。
	LocalPort string `json:"localPort,omitempty"`
}

// PanelSettingVersion 是当前的设置格式版本。
const PanelSettingVersion = 1

var panelHostRe = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)

// Normalize 校验并整理设置；返回 false 表示整理后就是默认值（不需要存）。
func (p *PanelSetting) Normalize() (bool, error) {
	p.V = PanelSettingVersion
	p.URL, p.Port, p.Path, p.LocalPort = strings.TrimSpace(p.URL), strings.TrimSpace(p.Port), strings.TrimSpace(p.Path), strings.TrimSpace(p.LocalPort)
	if p.Mode == "" {
		p.Mode = PanelDirect
	}
	switch p.Mode {
	case PanelURL:
		if err := checkPanelURL(p.URL); err != nil {
			return false, err
		}
		p.Port, p.Path, p.LocalPort = "", "", ""
		return true, nil
	case PanelDirect, PanelSSH:
		p.URL = ""
	default:
		return false, errors.New("面板地址的访问方式不认识")
	}
	if p.Port != "" && !validPort(p.Port) {
		return false, errors.New("面板端口不合法（1–65535）")
	}
	if p.Path != "" {
		if !strings.HasPrefix(p.Path, "/") {
			p.Path = "/" + p.Path
		}
		if !guidePathRe.MatchString(p.Path) {
			return false, errors.New("面板路径只能用字母、数字和 . _ ~ / -，以 / 开头")
		}
	}
	if p.Mode == PanelDirect {
		p.LocalPort = ""
		return p.Port != "" || p.Path != "", nil
	}
	if p.LocalPort != "" && !validPort(p.LocalPort) {
		return false, errors.New("本地端口不合法（1–65535）")
	}
	return true, nil
}

// checkPanelURL 只接受 http(s) 地址，不带账号密码、查询参数和片段：链接里不能夹带凭据。
func checkPanelURL(s string) error {
	if s == "" {
		return errors.New("请填写面板地址")
	}
	if len(s) > 512 || strings.ContainsAny(s, " \t\r\n\\\"'<>`") {
		return errors.New("面板地址不合法")
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Opaque != "" {
		return errors.New("面板地址要以 http:// 或 https:// 开头")
	}
	if u.User != nil {
		return errors.New("面板地址里不能带账号或密码")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(s, "?#") {
		return errors.New("面板地址里不能带查询参数（? 和 # 后面的部分），以免把令牌放进链接")
	}
	host := u.Hostname()
	if net.ParseIP(host) == nil && !panelHostRe.MatchString(host) {
		return errors.New("面板地址里的主机名不合法")
	}
	if p := u.Port(); p != "" && !validPort(p) {
		return errors.New("面板地址里的端口不合法")
	}
	if u.EscapedPath() != "" && !guidePathRe.MatchString(u.EscapedPath()) {
		return errors.New("面板地址里的路径只能用字母、数字和 . _ ~ / -")
	}
	return nil
}

// ParsePanelSetting 读出存着的设置；格式不对时返回 nil（按默认地址）。
func ParsePanelSetting(s string) *PanelSetting {
	var p PanelSetting
	if json.Unmarshal([]byte(s), &p) != nil || p.V != PanelSettingVersion {
		return nil
	}
	if keep, err := p.Normalize(); err != nil || !keep {
		return nil
	}
	return &p
}
