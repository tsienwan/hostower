package notify

import (
	"fmt"
	"github.com/tsienwan/hostower/internal/i18n"
	"strings"
	"time"
)

// Event 是一类可以订阅的通知。
type Event struct {
	Key string `json:"key"`
	// Group 用来在页面上分组显示：approval | run | host | security | system
	Group string `json:"group"`
	// Default 表示新建通知渠道时默认勾选。
	Default bool `json:"default"`
}

// Events 是全部可以订阅的通知。页面上的名字和说明在前端词典里，这里只有键。
var Events = []Event{
	{"approval.pending", "approval", true},
	{"request.failed", "run", true},
	{"request.done", "run", false},
	{"request.rejected", "security", true},
	{"host.offline", "host", true},
	{"host.online", "host", true},
	{"auth.failed", "security", true},
	{"terminal.opened", "security", false},
	{"system.paused", "security", true},
	{"secret.changed", "security", false},
	{"system.update", "system", true},
}

func knownEvent(key string) bool {
	for _, e := range Events {
		if e.Key == key {
			return true
		}
	}
	return false
}

// DefaultEvents 是新建渠道时默认订阅的通知。
func DefaultEvents() []string {
	var out []string
	for _, e := range Events {
		if e.Default {
			out = append(out, e.Key)
		}
	}
	return out
}

// Severity 决定通知的颜色和图标。
type Severity string

const (
	Info Severity = "info"
	Warn Severity = "warn"
	Crit Severity = "crit"
	OK   Severity = "ok"
)

// Message 是一条要发出去的通知。文字按渠道的语言选取；Fields 是“名称：值”的明细行。
type Message struct {
	Event    string
	Severity Severity
	Title    i18n.Text
	Fields   []Field
	Link     string // 页面上对应的地址（相对路径），发送时拼上访问地址
	Time     time.Time
}

// Field 是明细里的一行。名称按渠道语言显示，值原样显示（主机名、命令等不翻译）。
type Field struct {
	Name  i18n.Text
	Value string
}

// Text 把一条通知排成纯文本：标题一行，明细每行一条，最后是链接。
func (m Message) Text(lang, base string) (title, body string) {
	title = m.Title.In(lang)
	var b strings.Builder
	for _, f := range m.Fields {
		if f.Value == "" {
			continue
		}
		fmt.Fprintf(&b, "%s%s%s\n", f.Name.In(lang), i18n.Get(lang).Colon, f.Value)
	}
	if u := m.URL(base); u != "" {
		b.WriteString(u + "\n")
	}
	return title, strings.TrimRight(b.String(), "\n")
}

// URL 是通知对应的页面地址。
func (m Message) URL(base string) string {
	if m.Link == "" || base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + m.Link
}

func f(name i18n.Text, v string) Field { return Field{Name: name, Value: v} }

// clip 截断过长的内容（命令、错误信息），通知里只放个大概，细节回页面上看。
func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
