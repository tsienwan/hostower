package kernel

import (
	"regexp"
	"strings"
)

var (
	pemRe    = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(-----END [A-Z0-9 ]*PRIVATE KEY-----|\z)`)
	kvRe     = regexp.MustCompile(`(?i)\b([a-z0-9_.-]*(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key)[a-z0-9_.-]*)(\s*[=:]\s*)("[^"\n]*"|'[^'\n]*'|[^\s"']+)`)
	bearerRe = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[a-z0-9+/_.=-]{12,}`)
	urlRe    = regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^\s:/@]+):[^\s@/]+@`)
)

// Redact 把输出里看起来像凭据的内容遮掉。它用在两处：写审计之前，
// 以及把工具输出交给模型之前。脱敏是纵深防御，不是唯一防线。
func Redact(s string) string {
	s = pemRe.ReplaceAllString(s, "[已脱敏：私钥]")
	s = kvRe.ReplaceAllString(s, "$1$2[已脱敏]")
	s = bearerRe.ReplaceAllString(s, "$1 [已脱敏]")
	s = urlRe.ReplaceAllString(s, "$1:[已脱敏]@")
	return s
}

// Summarize 生成写进审计的输出摘要：脱敏，并只保留头尾。
func Summarize(s string) string {
	s = Redact(s)
	const head, tail = 1200, 800
	if len(s) <= head+tail {
		return s
	}
	h := strings.ToValidUTF8(s[:head], "")
	t := strings.ToValidUTF8(s[len(s)-tail:], "")
	return h + "\n…（中间省略）…\n" + t
}
