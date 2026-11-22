// Command hygiene 是仓库卫生检查：仓库里只能有代码、通用示例和文档。
// 它拦截四类东西：密钥、非保留网段的 IP 地址、非示例域名、不该进仓库的文件。
// 提交前钩子和 CI 都跑它。确实需要例外的行，在行尾加注释 hygiene:allow。
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	ipRe = regexp.MustCompile(`(^|[^0-9.])((?:\d{1,3}\.){3}\d{1,3})($|[^0-9.])`)
	// 域名只在配置和文档类文件里检查；代码里的导入路径不算。
	domainRe = regexp.MustCompile(`(?i)\b((?:[a-z0-9-]+\.)+(?:com|net|org|io|dev|cn|me|xyz|top|cc|co|app|cloud|info|tech|site|ai))\b`)

	secretRes = []*regexp.Regexp{
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
		regexp.MustCompile(`github_pat_[A-Za-z0-9_]{40,}`),
		regexp.MustCompile(`sk-[A-Za-z0-9_-]{32,}`),
		regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{20,}`),
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{20,}\.eyJ[A-Za-z0-9_-]{20,}\.`),
	}
	pemHeadRe  = regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----`)
	pemBodyRe  = regexp.MustCompile(`^[A-Za-z0-9+/=]{40,}$`)
	assignedRe = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|private[_-]?key)["']?\s*[:=]\s*["']?([^\s"'#]{12,})`)

	forbiddenNames = regexp.MustCompile(`(?i)(^|/)(master_key|\.env|id_(rsa|ed25519|ecdsa)|known_hosts|audit\.jsonl)$|\.(pem|key|p12|pfx|db|sqlite)$`)
	forbiddenDirs  = []string{"config/", "data/", "backups/"}
)

// 允许出现的域名：示例域名，以及项目本身引用的公开站点。
var allowedDomains = []string{
	"example.com", "example.org", "example.net",
	"github.com", "ghcr.io", "githubusercontent.com", "golang.org", "go.dev", "pkg.go.dev",
	"mvdan.cc", "modernc.org", "gopkg.in", "npmjs.com", "nodejs.org", "docker.com", "docker.io", "gcr.io",
	"sigstore.dev", "contributor-covenant.org", "semver.org", "w3.org", "claude.com", "anthropic.com",
	"vite.dev", "react.dev", "lucide.dev",
	"speed.cloudflare.com",  // 测速服务（internal/speedtest），只放这一个主机名，不放整个 cloudflare.com
	"containerd.io",         // 不是网站：Docker 官方软件源里的包名（internal/apps/docker.go）
	"install.speedtest.net", // Ookla 测速程序的官方下载地址（internal/speedtest/ookla.go），只放这一个主机名
	"proxycheck.io",         // 环境检测查出口 IP 归属用的公开信息库（internal/hostenv），文档里要写明用的是哪一家
}

var domainFileExt = map[string]bool{".yaml": true, ".yml": true, ".md": true, ".txt": true, ".toml": true, ".json": true, ".conf": true, ".env": true}

var skipFiles = map[string]bool{"go.sum": true, "go.mod": true, "web/package-lock.json": true}

func reservedIP(ip string) bool {
	parts := strings.Split(ip, ".")
	var o [4]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n > 255 {
			return true // 不是合法地址（多半是版本号之类），不管
		}
		o[i] = n
	}
	switch {
	case o[0] == 127, o[0] == 0, o[0] == 255:
		return true
	case o[0] == 192 && o[1] == 0 && o[2] == 2: // TEST-NET-1
		return true
	case o[0] == 198 && o[1] == 51 && o[2] == 100: // TEST-NET-2
		return true
	case o[0] == 203 && o[1] == 0 && o[2] == 113: // TEST-NET-3
		return true
	}
	return false
}

func allowedDomain(d string) bool {
	d = strings.ToLower(d)
	for _, a := range allowedDomains {
		if d == a || strings.HasSuffix(d, "."+a) {
			return true
		}
	}
	return false
}

type finding struct {
	file string
	line int
	msg  string
}

func scan(path string, data []byte) []finding {
	var out []finding
	add := func(line int, msg string) { out = append(out, finding{path, line, msg}) }
	if bytes.IndexByte(data, 0) >= 0 {
		return nil // 二进制文件
	}
	ext := strings.ToLower(filepath.Ext(path))
	isConfig := domainFileExt[ext]
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.Contains(line, "hygiene:allow") {
			continue
		}
		n := i + 1
		for _, m := range ipRe.FindAllStringSubmatch(line, -1) {
			if !reservedIP(m[2]) {
				add(n, "非保留网段的 IP 地址 "+m[2]+"（示例请用 192.0.2.0/24、198.51.100.0/24、203.0.113.0/24）")
			}
		}
		for _, re := range secretRes {
			if re.MatchString(line) {
				add(n, "疑似密钥或令牌")
			}
		}
		if pemHeadRe.MatchString(line) && i+1 < len(lines) && pemBodyRe.MatchString(strings.TrimSpace(lines[i+1])) {
			add(n, "私钥")
		}
		if isConfig {
			for _, m := range domainRe.FindAllStringSubmatch(line, -1) {
				if !allowedDomain(m[1]) {
					add(n, "非示例域名 "+m[1]+"（示例请用 example.com）")
				}
			}
			if ext != ".md" {
				if m := assignedRe.FindStringSubmatch(line); m != nil && !strings.Contains(m[2], "<") && !strings.HasPrefix(m[2], "ref:") && !strings.HasPrefix(m[2], "/") && !strings.HasPrefix(m[2], "$") {
					add(n, "配置文件里疑似写了明文凭据（"+m[1]+"）")
				}
			}
		}
	}
	return out
}

func gitFiles(staged bool) ([]string, error) {
	args := []string{"ls-files", "-co", "--exclude-standard"}
	if staged {
		args = []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR"}
	}
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	var files []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if f := strings.TrimSpace(sc.Text()); f != "" {
			files = append(files, filepath.ToSlash(f))
		}
	}
	return files, nil
}

func main() {
	staged := flag.Bool("staged", false, "只检查已暂存的文件（提交前钩子用）")
	flag.Parse()
	files, err := gitFiles(*staged)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hygiene:", err)
		os.Exit(2)
	}
	var all []finding
	for _, f := range files {
		if skipFiles[f] || strings.HasPrefix(f, "web/dist/") || strings.HasPrefix(f, "web/node_modules/") {
			continue
		}
		for _, d := range forbiddenDirs {
			if strings.HasPrefix(f, d) {
				all = append(all, finding{f, 0, "真实环境的配置或数据不进仓库（" + d + "）"})
			}
		}
		if forbiddenNames.MatchString(f) {
			all = append(all, finding{f, 0, "这类文件不进仓库（密钥、数据库、环境文件）"})
			continue
		}
		info, err := os.Stat(f)
		if err != nil || info.IsDir() || info.Size() > 2<<20 {
			if err == nil && !info.IsDir() && info.Size() > 2<<20 {
				all = append(all, finding{f, 0, fmt.Sprintf("文件过大（%d KB），多半是无用内容或构建产物", info.Size()>>10)})
			}
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		all = append(all, scan(f, data)...)
	}
	if len(all) == 0 {
		fmt.Printf("hygiene: 检查了 %d 个文件，没有问题\n", len(files))
		return
	}
	for _, f := range all {
		if f.line > 0 {
			fmt.Fprintf(os.Stderr, "%s:%d: %s\n", f.file, f.line, f.msg)
		} else {
			fmt.Fprintf(os.Stderr, "%s: %s\n", f.file, f.msg)
		}
	}
	fmt.Fprintf(os.Stderr, "\nhygiene: 发现 %d 个问题。确实需要例外的行，在行尾加注释 hygiene:allow\n", len(all))
	os.Exit(1)
}
