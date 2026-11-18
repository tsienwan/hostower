// Command i18n 检查多语言有没有漏：go run ./tools/i18n [-v]
//
// 支持的语言只登记在 web/src/locales/languages.json。这个工具按清单逐个语言核对：
//
//   - 清单本身：原文语言在、代码不重复、fallback 指向清单里的语言且不绕圈；
//     每种非原文语言都有界面词典目录 web/src/locales/<code>/ 和服务端词表 internal/i18n/text/<code>.json，没有清单外的目录或词表。
//   - 界面词典：代码里每个 t("中文") 在每种语言里都有译文；各语言的词条一致（某种语言有、另一种没有，就是漏了）。
//   - 服务端文字词典（server-*.ts）：各语言的词条一致。
//   - Go 发出去的文字（通知、聊天机器人）：代码里每个 i18n.T / i18n.Tx 的中文原文在每个词表里都有，词表里没有用不到的条目。
//   - 占位符：译文里的 {名字} 和原文一致，不能丢也不能多。
//
// 以上任何一条不满足都算错误，退出码 1。另外会提醒（不算错误）：
//   - Go 代码里 errors.New / fmt.Errorf / writeErr 的中文在英文的 server-*.ts 里找不到对应的整句或模板
//     （这些文字由界面翻译；拼了变量的句子只能尽量匹配，所以只提醒）；
//   - 界面词典里的词条在代码里找不到（可能没用了，也可能是通过变量传给 t 的）。
//
// 加 -v 列出提醒的明细。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type language struct {
	Code     string `json:"code"`
	Fallback string `json:"fallback"`
}

type registry struct {
	Source    string     `json:"source"`
	Languages []language `json:"languages"`
}

const (
	registryPath = "web/src/locales/languages.json"
	localesDir   = "web/src/locales"
	textDir      = "internal/i18n/text"
)

var (
	errs, warns []string
	verbose     = flag.Bool("v", false, "列出提醒的明细")
)

func errorf(f string, a ...any) { errs = append(errs, fmt.Sprintf(f, a...)) }

func main() {
	flag.Parse()
	var reg registry
	b, err := os.ReadFile(registryPath)
	if err != nil {
		fail("读不到语言清单：" + err.Error())
	}
	if err := json.Unmarshal(b, &reg); err != nil {
		fail("语言清单格式不对：" + err.Error())
	}
	langs := checkRegistry(reg)
	ui, server := loadWebDicts(langs)
	checkUI(ui)
	checkConsistent("服务端文字词典（server-*.ts）", server)
	checkGoText(langs)
	checkServerCoverage(server["en"])

	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "错误："+e)
	}
	if len(warns) > 0 {
		if *verbose {
			for _, w := range warns {
				fmt.Fprintln(os.Stderr, "提醒："+w)
			}
		} else {
			fmt.Fprintf(os.Stderr, "提醒：%d 条（加 -v 查看）\n", len(warns))
		}
	}
	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\ni18n：发现 %d 个问题。\n", len(errs))
		os.Exit(1)
	}
	fmt.Printf("i18n：%d 种语言，检查通过\n", len(langs)+1)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "错误："+msg)
	os.Exit(1)
}

// checkRegistry 检查清单，返回除原文语言以外的语言代码。
func checkRegistry(reg registry) []string {
	codes := map[string]language{}
	var out []string
	for _, l := range reg.Languages {
		if l.Code == "" {
			errorf("语言清单里有一行没有 code")
			continue
		}
		if _, dup := codes[l.Code]; dup {
			errorf("语言清单里 %s 重复了", l.Code)
		}
		codes[l.Code] = l
		if l.Code != reg.Source {
			out = append(out, l.Code)
		}
	}
	if _, ok := codes[reg.Source]; !ok {
		fail("语言清单里没有原文语言 " + reg.Source)
	}
	for _, l := range reg.Languages {
		seen := map[string]bool{l.Code: true}
		for f := l.Fallback; f != ""; f = codes[f].Fallback {
			if _, ok := codes[f]; !ok {
				errorf("%s 的 fallback %s 不在清单里", l.Code, f)
				break
			}
			if seen[f] {
				errorf("%s 的 fallback 绕圈了", l.Code)
				break
			}
			seen[f] = true
		}
	}
	// 每种语言都要有界面词典目录和服务端词表；清单外的不许有。
	for _, c := range out {
		if st, err := os.Stat(filepath.Join(localesDir, c)); err != nil || !st.IsDir() {
			errorf("缺界面词典目录 %s/%s/", localesDir, c)
		}
		if _, err := os.Stat(filepath.Join(textDir, c+".json")); err != nil {
			errorf("缺服务端词表 %s/%s.json", textDir, c)
		}
	}
	entries, _ := os.ReadDir(localesDir)
	for _, e := range entries {
		if e.IsDir() {
			if _, ok := codes[e.Name()]; !ok || e.Name() == reg.Source {
				errorf("%s/%s/ 不是清单里的语言（或者是原文语言，不需要词典）", localesDir, e.Name())
			}
		}
	}
	texts, _ := filepath.Glob(filepath.Join(textDir, "*.json"))
	for _, p := range texts {
		c := strings.TrimSuffix(filepath.Base(p), ".json")
		if _, ok := codes[c]; !ok || c == reg.Source {
			errorf("%s 不是清单里的语言（或者是原文语言，不需要词表）", filepath.ToSlash(p))
		}
	}
	return out
}

// ---------- 界面 ----------

var (
	tsEntryRe  = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"\s*:\s*"((?:[^"\\\n]|\\.)*)"`)
	tCallRe    = regexp.MustCompile(`\bt\(\s*"((?:[^"\\\n]|\\.)*)"`)
	tsStringRe = regexp.MustCompile(`"((?:[^"\\\n]|\\.)*)"`)
	commentRe  = regexp.MustCompile(`(?m)^\s*//.*$`)
	hanRe      = regexp.MustCompile(`\p{Han}`)
)

func unquote(s string) string {
	var v string
	if json.Unmarshal([]byte(`"`+s+`"`), &v) != nil {
		return s
	}
	return v
}

// loadWebDicts 读各语言的界面词典和服务端文字词典。
func loadWebDicts(langs []string) (ui, server map[string]map[string]string) {
	ui, server = map[string]map[string]string{}, map[string]map[string]string{}
	for _, l := range langs {
		ui[l], server[l] = map[string]string{}, map[string]string{}
		files, _ := filepath.Glob(filepath.Join(localesDir, l, "*.ts"))
		for _, p := range files {
			b, _ := os.ReadFile(p)
			dst := ui[l]
			if strings.HasPrefix(filepath.Base(p), "server") {
				dst = server[l]
			}
			// 不同文件里有同一个词是允许的（各页面的词典各管各的），但译法不同时合并后只剩一个，提醒一下。
			for _, m := range tsEntryRe.FindAllStringSubmatch(commentRe.ReplaceAllString(string(b), ""), -1) {
				k, v := unquote(m[1]), unquote(m[2])
				if old, dup := dst[k]; dup && old != v {
					warns = append(warns, fmt.Sprintf("%s 的 %q 在几个文件里译法不同（%q / %q）", l, short(k), short(old), short(v)))
				}
				dst[k] = v
				checkPlaceholders(l, k, v)
			}
		}
	}
	return ui, server
}

// webSource 返回界面代码（不含词典）里所有 t("…") 的键，以及所有字符串字面量。
func webSource() (keys, literals map[string]bool) {
	keys, literals = map[string]bool{}, map[string]bool{}
	filepath.WalkDir("web/src", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "locales" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".ts") && !strings.HasSuffix(p, ".tsx") {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, m := range tCallRe.FindAllStringSubmatch(string(b), -1) {
			if k := unquote(m[1]); hanRe.MatchString(k) {
				keys[k] = true
			}
		}
		for _, m := range tsStringRe.FindAllStringSubmatch(string(b), -1) {
			literals[unquote(m[1])] = true
		}
		return nil
	})
	return keys, literals
}

func checkUI(ui map[string]map[string]string) {
	keys, literals := webSource()
	for l, d := range ui {
		var miss []string
		for k := range keys {
			plain, _, _ := strings.Cut(k, "@@")
			if _, ok := d[k]; !ok {
				if _, ok := d[plain]; !ok {
					miss = append(miss, k)
				}
			}
		}
		report(fmt.Sprintf("界面词典 %s 缺代码里用到的 t(…)", l), miss)
	}
	checkConsistent("界面词典", ui)
	// 词典里有、代码里找不到的：可能没用了，也可能是通过变量传给 t 的。
	any := ui[firstKey(ui)]
	var stale []string
	for k := range any {
		if !keys[k] && !literals[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)
	for _, k := range stale {
		warns = append(warns, fmt.Sprintf("界面词典里的 %q 在代码里找不到，可能没用了", short(k)))
	}
}

// checkConsistent 要求各语言的词条一样：某种语言有、另一种没有，就是漏了。
func checkConsistent(what string, dicts map[string]map[string]string) {
	all := map[string]bool{}
	for _, d := range dicts {
		for k := range d {
			all[k] = true
		}
	}
	for l, d := range dicts {
		var miss []string
		for k := range all {
			if _, ok := d[k]; !ok {
				miss = append(miss, k)
			}
		}
		report(fmt.Sprintf("%s %s 缺别的语言里有的词条", what, l), miss)
	}
}

// ---------- Go 发出去的文字 ----------

var (
	goTRe     = regexp.MustCompile(`\bi18n\.(?:T\(\s*[^,()]+,|Tx\()\s*("(?:[^"\\\n]|\\.)*")`)
	goErrRe   = regexp.MustCompile(`(?:errors\.New|fmt\.Errorf|writeErr\([^,]+,\s*\d+,)\(?\s*("(?:[^"\\\n]|\\.)*")`)
	goVerbRe  = regexp.MustCompile(`%[-+# 0-9.]*[vsdqwxXfgt]`)
	holderRe  = regexp.MustCompile(`\{[A-Za-z0-9_]+!?\}`)
	tplHoleRe = regexp.MustCompile(`\\\{[A-Za-z0-9_]+!?\\\}`)
)

// goSource 读 Go 代码（不含测试和 internal/i18n 本身）里的 i18n.T / i18n.Tx 键，以及报错用的中文。
func goSource() (keys map[string]bool, errTexts map[string]string) {
	keys, errTexts = map[string]bool{}, map[string]string{}
	filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		p = filepath.ToSlash(p)
		if d.IsDir() {
			if p == "web/node_modules" || p == ".git" || p == "internal/i18n" || p == "tools" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, m := range goTRe.FindAllStringSubmatch(string(b), -1) {
			if k, err := strconv.Unquote(m[1]); err == nil {
				keys[k] = true
			}
		}
		// 命令行和启动时的报错只出现在终端和日志里，不经过界面，不用翻译。
		if strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "internal/app/") {
			return nil
		}
		for _, m := range goErrRe.FindAllStringSubmatch(string(b), -1) {
			if k, err := strconv.Unquote(m[1]); err == nil && hanRe.MatchString(k) {
				errTexts[k] = p
			}
		}
		return nil
	})
	return keys, errTexts
}

func checkGoText(langs []string) {
	keys, _ := goSource()
	for _, l := range langs {
		p := filepath.Join(textDir, l+".json")
		b, err := os.ReadFile(p)
		if err != nil {
			continue // checkRegistry 已经报过
		}
		d := map[string]string{}
		if err := json.Unmarshal(b, &d); err != nil {
			errorf("%s 格式不对：%v", filepath.ToSlash(p), err)
			continue
		}
		var miss, stale []string
		for k := range keys {
			if _, ok := d[k]; !ok {
				miss = append(miss, k)
			}
		}
		for k, v := range d {
			if !keys[k] {
				stale = append(stale, k)
			}
			checkPlaceholders(l, k, v)
		}
		report(fmt.Sprintf("服务端词表 %s 缺代码里用到的 i18n.T / i18n.Tx", filepath.ToSlash(p)), miss)
		report(fmt.Sprintf("服务端词表 %s 里有代码已经不用的条目", filepath.ToSlash(p)), stale)
	}
}

// checkServerCoverage 尽量核对报错用的中文在英文的服务端文字词典里有没有对应（只提醒）。
func checkServerCoverage(en map[string]string) {
	_, texts := goSource()
	var tpls []*regexp.Regexp
	for k := range en {
		if holderRe.MatchString(k) {
			tpls = append(tpls, regexp.MustCompile("^"+tplHoleRe.ReplaceAllString(regexp.QuoteMeta(k), `.+?`)+"$"))
		}
	}
	var miss []string
	for text, file := range texts {
		if _, ok := en[text]; ok {
			continue
		}
		// 把 %v 这类动词换成一段示例文字，再看有没有模板能匹配上。
		sample := goVerbRe.ReplaceAllString(text, "x")
		matched := false
		for _, re := range tpls {
			if re.MatchString(sample) {
				matched = true
				break
			}
		}
		if !matched {
			miss = append(miss, file+"："+text)
		}
	}
	sort.Strings(miss)
	for _, m := range miss {
		warns = append(warns, "服务端文字在 en 的 server-*.ts 里可能没有译文："+short(m))
	}
}

// ---------- 公用 ----------

func checkPlaceholders(lang, k, v string) {
	a, b := holderRe.FindAllString(k, -1), holderRe.FindAllString(v, -1)
	sort.Strings(a)
	sort.Strings(b)
	if strings.Join(a, ",") != strings.Join(b, ",") {
		errorf("%s 的 %q 占位符对不上：原文 %v，译文 %v", lang, short(k), a, b)
	}
}

func report(what string, items []string) {
	if len(items) == 0 {
		return
	}
	sort.Strings(items)
	var b strings.Builder
	fmt.Fprintf(&b, "%s（%d 条）", what, len(items))
	for i, it := range items {
		if i == 10 {
			fmt.Fprintf(&b, "\n    …… 还有 %d 条", len(items)-10)
			break
		}
		fmt.Fprintf(&b, "\n    %q", short(it))
	}
	errs = append(errs, b.String())
}

func short(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

func firstKey(m map[string]map[string]string) string {
	for k := range m {
		if k == "en" {
			return k
		}
	}
	for k := range m {
		return k
	}
	return ""
}
