// Package i18n 是服务端自己发出去的文字（通知、聊天机器人的回复）的多语言。
//
// 支持哪些语言只看一份清单：web/src/locales/languages.json（界面也读它）。
// 代码里直接写中文原文，占位符写成 {名字}；其他语言的译文在 text/<语言>.json 里，按中文原文查。
// 缺译文时按清单里的 fallback 退（通常退到英文），再退回中文原文。
// 有没有漏译由 go run ./tools/i18n 检查：它从代码里找出 i18n.T / i18n.Tx 的中文原文，逐个语言核对。
//
// 页面上显示的服务端文字（错误、判级理由、审计说明）不走这里：服务端只说中文，界面显示时翻译。
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tsienwan/hostower/web"
)

// Language 是清单里的一种语言。
type Language struct {
	Code     string   `json:"code"`
	Label    string   `json:"label"`
	Fallback string   `json:"fallback"`
	Browser  []string `json:"browser"`
	Telegram string   `json:"telegram"`
	Feishu   string   `json:"feishu"`
	Colon    string   `json:"colon"`
	Geo      string   `json:"geo"`
}

// Registry 是整份语言清单。
type Registry struct {
	Source         string     `json:"source"`
	BrowserDefault string     `json:"browserDefault"`
	Languages      []Language `json:"languages"`
}

//go:embed text/*.json
var textFS embed.FS

var (
	reg    Registry
	byCode = map[string]*Language{}
	dicts  = map[string]map[string]string{}
)

func init() {
	if err := json.Unmarshal(web.Languages, &reg); err != nil {
		panic("语言清单 languages.json 格式不对：" + err.Error())
	}
	for i := range reg.Languages {
		l := &reg.Languages[i]
		byCode[l.Code] = l
		if l.Code == reg.Source {
			continue
		}
		b, err := textFS.ReadFile("text/" + l.Code + ".json")
		if err != nil {
			continue // 没有词典时全部按 fallback 退；tools/i18n 会报出来
		}
		d := map[string]string{}
		if err := json.Unmarshal(b, &d); err != nil {
			panic("词典 text/" + l.Code + ".json 格式不对：" + err.Error())
		}
		dicts[l.Code] = d
	}
	if byCode[reg.Source] == nil {
		panic("语言清单里没有原文语言 " + reg.Source)
	}
}

// Source 是原文语言（代码里直接写的那种）。
func Source() string { return reg.Source }

// Languages 返回清单里的语言，按清单的顺序。
func Languages() []Language { return append([]Language(nil), reg.Languages...) }

// Known 报告是不是清单里的语言。
func Known(code string) bool { return byCode[code] != nil }

// Get 返回一种语言的设置；不认识的按原文语言。
func Get(code string) Language {
	if l := byCode[code]; l != nil {
		return *l
	}
	return *byCode[reg.Source]
}

// Dict 返回一种语言的词典（只读，给检查工具用）。
func Dict(code string) map[string]string { return dicts[code] }

// T 把中文原文 key 翻成 lang，并填上占位符。args 是成对的 名字, 值：T(lang, "主机离线：{host}", "host", name)。
func T(lang, key string, args ...any) string {
	s := lookup(lang, key)
	for i := 0; i+1 < len(args); i += 2 {
		s = strings.ReplaceAll(s, "{"+fmt.Sprint(args[i])+"}", fmt.Sprint(args[i+1]))
	}
	return s
}

func lookup(lang, key string) string {
	seen := map[string]bool{}
	for l := lang; l != "" && !seen[l]; l = Get(l).Fallback {
		seen[l] = true
		if !Known(l) || l == reg.Source {
			break
		}
		if v, ok := dicts[l][key]; ok && v != "" {
			return v
		}
	}
	return key
}

// Text 是一段要等知道语言之后才翻译的文字（比如一条要发给多个渠道的通知）。
type Text struct {
	Key  string
	Args []any
}

// Tx 记下一段文字和它的占位符，见 T。
func Tx(key string, args ...any) Text { return Text{Key: key, Args: args} }

// In 返回这段文字在 lang 下的样子。
func (t Text) In(lang string) string { return T(lang, t.Key, t.Args...) }
