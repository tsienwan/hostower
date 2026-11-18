package speedtest

// Preset 是一个常用的路由追踪目标：国内各地三家运营商的公共 DNS 服务器地址。
// 从境外主机追踪到这些地址，看的是回到国内各运营商的线路（CN2 GIA、9929、CMI 等）。
// City 和 ISP 是中文原文，界面按语言翻译后拼成名字。
type Preset struct {
	City   string `json:"city"`
	ISP    string `json:"isp"`
	Target string `json:"target"`
}

// Presets 来自开源的回程测试工具（zhanghanyun/backtrace、oneclickvirt/backtrace）用的同一组目标，
// 都是运营商公开的 DNS 服务器地址，不是任何部署的地址。有的不回应最后一跳，但沿路的骨干网照样能看到。
var Presets = []Preset{
	{"北京", "电信", "219.141.140.10"},  // hygiene:allow
	{"北京", "联通", "202.106.195.68"},  // hygiene:allow
	{"北京", "移动", "221.179.155.161"}, // hygiene:allow
	{"上海", "电信", "202.96.209.133"},  // hygiene:allow
	{"上海", "联通", "210.22.97.1"},     // hygiene:allow
	{"上海", "移动", "211.136.112.200"}, // hygiene:allow
	{"广州", "电信", "58.60.188.222"},   // hygiene:allow
	{"广州", "联通", "210.21.196.6"},    // hygiene:allow
	{"广州", "移动", "120.196.165.24"},  // hygiene:allow
	{"成都", "电信", "61.139.2.69"},     // hygiene:allow
	{"成都", "联通", "119.6.6.6"},       // hygiene:allow
	{"成都", "移动", "211.137.96.205"},  // hygiene:allow
}
