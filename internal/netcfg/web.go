package netcfg

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/tsienwan/hostower/internal/snippet"
)

// ---------- 站点：Caddy 或 Nginx ----------
//
// 每个站点一个文件，第一行是标记（# hostower-site <地址> <目标> 选项…），检测脚本只读这一行，
// 所以页面上只列出 Hostower 添加的站点，主机上原有的配置不受影响。
// 写文件用带引号的 heredoc：内容原样写进去，不做任何展开；它是命令的一部分，审批摘要覆盖了写入的全部内容。
// 写完先校验（caddy validate / nginx -t），通过才重载；校验不过就把这个文件改名撤下，配置回到原样。

// NginxDir 是 Hostower 写 Nginx 站点的目录（各发行版的 nginx.conf 都 include 它）。
const NginxDir = "/etc/nginx/conf.d"

// ACMERoot 是 Nginx 站点给 Let's Encrypt 验证用的目录（certbot --webroot）。
const ACMERoot = "/var/www/hostower-acme"

const siteMarker = "# hostower-site "

// hdocEnd 是写文件用的 heredoc 结束标记。写进去的每一行都经过校验，不可能出现它。
const hdocEnd = "HOSTOWER_EOF"

// Site 是一个 Hostower 管理的站点。
type Site struct {
	Server  string `json:"server"`  // caddy | nginx
	Address string `json:"address"` // 域名（可带端口）或 IP:端口
	Kind    string `json:"kind"`    // proxy（反向代理）| static（静态目录）| redirect（跳转）
	// Target：反向代理是 127.0.0.1:端口，静态目录是目录，跳转是网址。
	Target string `json:"target"`
	// TLS：auto（Caddy 自动申请）| internal（Caddy 自签）| le（Nginx + Let's Encrypt）| custom（已有证书）| http（不用 HTTPS）
	TLS      string `json:"tls"`
	MaxBody  int    `json:"maxBody,omitempty"` // 请求体上限（MB），0 表示用服务器的默认值
	HSTS     bool   `json:"hsts,omitempty"`
	KeepHTTP bool   `json:"keepHttp,omitempty"` // 有 HTTPS 时仍然允许 HTTP 访问（不跳转）
	Perm     bool   `json:"perm,omitempty"`     // 跳转用 301（永久），否则 302
}

// SiteRequest 是“添加站点”的表单。
type SiteRequest struct {
	Server   string `json:"server"`
	Address  string `json:"address"`
	Kind     string `json:"kind"`
	Port     int    `json:"port"`  // 反向代理：转发到 127.0.0.1 的端口
	Dir      string `json:"dir"`   // 静态目录
	URL      string `json:"url"`   // 跳转到的网址
	TLS      string `json:"tls"`   // 空表示按服务器选默认值
	Email    string `json:"email"` // Let's Encrypt 的联系邮箱（可空）
	Agree    bool   `json:"agree"` // 同意 Let's Encrypt 的订户协议
	Cert     string `json:"cert"`  // 已有证书：证书文件
	Key      string `json:"key"`   // 已有证书：私钥文件
	MaxBody  int    `json:"maxBody"`
	HSTS     bool   `json:"hsts"`
	KeepHTTP bool   `json:"keepHttp"`
	Perm     bool   `json:"perm"`
}

var (
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}(:[0-9]{1,5})?$`)
	ipPortRe = regexp.MustCompile(`^[0-9]{1,3}(\.[0-9]{1,3}){3}:[0-9]{1,5}$`)
	// 静态目录只能在这几个放网站的地方，而且不能是隐藏目录：别把 /etc、家目录里的 .ssh 之类的发布到网上。
	staticDirRe = regexp.MustCompile(`^/(var/www|srv|opt|home)(/[A-Za-z0-9_@+-][A-Za-z0-9._@+-]*)+$`)
	redirectRe  = regexp.MustCompile(`^https?://[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:[0-9]{1,5})?(/[A-Za-z0-9._~%/?&=+-]*)?$`)
	certPathRe  = regexp.MustCompile(`^/(etc/(letsencrypt|ssl|pki|nginx|caddy)|opt|srv|var/lib)(/[A-Za-z0-9_@+-][A-Za-z0-9._@+-]*)+\.(pem|crt|cer|key)$`)
	emailRe     = regexp.MustCompile(`^[A-Za-z0-9._%+-]{1,64}@([A-Za-z0-9-]{1,63}\.)+[A-Za-z]{2,63}$`)
	certNameRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`)
)

// ValidAddress 校验站点地址：域名（可带端口），或者 IP:端口。
func ValidAddress(addr string) (ip bool, err error) {
	addr = strings.ToLower(addr)
	switch {
	case ipPortRe.MatchString(addr):
		host, port, _ := net.SplitHostPort(addr)
		n, _ := strconv.Atoi(port)
		if net.ParseIP(host) == nil || n < 1 || n > 65535 {
			return false, errors.New("地址不合法")
		}
		return true, nil
	case domainRe.MatchString(addr):
		if _, port, err := net.SplitHostPort(addr); err == nil {
			if n, _ := strconv.Atoi(port); n < 1 || n > 65535 {
				return false, errors.New("端口要在 1 到 65535 之间")
			}
		}
		return false, nil
	}
	return false, errors.New("地址要是域名（例如 app.example.com，可以带端口），或者 IP:端口")
}

func baseName(addr string) string { return strings.ReplaceAll(strings.ToLower(addr), ":", "_") }

// SiteFile 是站点配置文件的位置。
func SiteFile(server, addr string) string {
	if server == "nginx" {
		return NginxDir + "/hostower-" + baseName(addr) + ".conf"
	}
	return SitesDir + "/" + baseName(addr) + ".caddy"
}

// LogFiles 是站点的访问日志和错误日志（Caddy 的错误日志在 journal 里，这里为空）。
func LogFiles(server, addr string) (access, errlog string) {
	if server == "nginx" {
		b := "/var/log/nginx/hostower-" + baseName(addr)
		return b + ".access.log", b + ".error.log"
	}
	return "/var/log/caddy/hostower-" + baseName(addr) + ".log", ""
}

// marker 是站点文件的第一行。旧的站点只有“地址 目标”两段，其余选项按默认值。
func (s Site) marker() string {
	b := siteMarker + s.Address + " " + s.Target + " kind=" + s.Kind + " tls=" + s.TLS
	if s.MaxBody > 0 {
		b += " body=" + strconv.Itoa(s.MaxBody)
	}
	for _, f := range []struct {
		on   bool
		name string
	}{{s.HSTS, "hsts"}, {s.KeepHTTP, "http"}, {s.Perm, "perm"}} {
		if f.on {
			b += " " + f.name + "=1"
		}
	}
	return b
}

func parseSite(server, line string) (Site, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), siteMarker)
	if !ok {
		return Site{}, false
	}
	f := strings.Fields(rest)
	if len(f) < 2 {
		return Site{}, false
	}
	s := Site{Server: server, Address: f[0], Target: f[1], Kind: "proxy"}
	for _, kv := range f[2:] {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "kind":
			s.Kind = v
		case "tls":
			s.TLS = v
		case "body":
			s.MaxBody, _ = strconv.Atoi(v)
		case "hsts":
			s.HSTS = v == "1"
		case "http":
			s.KeepHTTP = v == "1"
		case "perm":
			s.Perm = v == "1"
		}
	}
	if s.TLS == "" {
		switch {
		case server == "nginx":
			s.TLS = "http"
		case ipPortRe.MatchString(s.Address):
			s.TLS = "internal"
		default:
			s.TLS = "auto"
		}
	}
	return s, true
}

// writeRoot 以 root 身份把 content 原样写进 file（带引号的 heredoc，不做任何展开）。
func writeRoot(file, content, fail string) string {
	return "{ " + asRoot("tee "+file+" >/dev/null") + "; } <<'" + hdocEnd + "' || { echo " + snippet.Quote("hostower: "+fail) + "; exit 1; }\n" +
		content + hdocEnd + "\n"
}

// installPkgs 渲染“用系统软件源装这些包”：apt 先直接装，找不到包再 update 一次重试。
func installPkgs(st Status, pkgs, fail string) (string, error) {
	switch st.Family {
	case "debian":
		in := "apt-get install -y -q " + pkgs
		return "{ { " + asRoot(in) + "; } || { { " + asRoot("apt-get update -q") + "; } && { " + asRoot(in) + "; }; }; } || { echo " +
			snippet.Quote("hostower: "+fail) + "; exit 1; }; ", nil
	case "rhel":
		return step("dnf install -y -q "+pkgs, fail+"（RHEL 系通常要先启用 EPEL 源）"), nil
	}
	return "", errors.New("这台主机的系统不支持一键安装，请在主机上自己安装")
}

// normalize 校验表单并补上默认值，返回要写的站点。
func (st Status) normalize(req SiteRequest) (Site, error) {
	addr := strings.ToLower(strings.TrimSpace(req.Address))
	ip, err := ValidAddress(addr)
	if err != nil {
		return Site{}, err
	}
	s := Site{Server: req.Server, Address: addr, Kind: req.Kind, TLS: req.TLS, MaxBody: req.MaxBody, HSTS: req.HSTS, KeepHTTP: req.KeepHTTP, Perm: req.Perm}
	if s.Server == "" {
		if st.Nginx.Installed && !st.Caddy.Installed {
			s.Server = "nginx"
		} else {
			s.Server = "caddy"
		}
	}
	switch s.Server {
	case "caddy":
		if !st.Caddy.Installed {
			return Site{}, errors.New("这台主机还没有装 Caddy")
		}
	case "nginx":
		if !st.Nginx.Installed {
			return Site{}, errors.New("这台主机还没有装 Nginx")
		}
	default:
		return Site{}, errors.New("服务器只能是 Caddy 或 Nginx")
	}
	for _, o := range st.Sites {
		if o.Address == addr {
			return Site{}, errors.New("这个地址已经有站点了，先撤下它再添加")
		}
	}
	if s.MaxBody < 0 || s.MaxBody > 10240 {
		return Site{}, errors.New("请求体上限要在 0 到 10240 MB 之间")
	}
	if s.Kind == "" {
		s.Kind = "proxy"
	}
	switch s.Kind {
	case "proxy":
		if req.Port < 1 || req.Port > 65535 {
			return Site{}, errors.New("转发到的端口要在 1 到 65535 之间")
		}
		s.Target = "127.0.0.1:" + strconv.Itoa(req.Port)
	case "static":
		dir := strings.TrimRight(strings.TrimSpace(req.Dir), "/")
		if !staticDirRe.MatchString(dir) {
			return Site{}, errors.New("静态目录要在 /var/www、/srv、/opt 或 /home 下面，不能是隐藏目录，只能用字母、数字和 . _ - @ +")
		}
		s.Target = dir
	case "redirect":
		u := strings.TrimSpace(req.URL)
		if !redirectRe.MatchString(u) {
			return Site{}, errors.New("跳转地址要是 http:// 或 https:// 开头的网址（域名用小写），不能有空格、引号或 # $ ; { } 这些字符")
		}
		s.Target = u
	default:
		return Site{}, errors.New("站点类型只能是反向代理、静态目录或跳转")
	}
	_, port, _ := net.SplitHostPort(addr)
	if s.TLS == "" {
		switch {
		case s.Server == "nginx":
			s.TLS = "http"
		case ip:
			s.TLS = "internal"
		default:
			s.TLS = "auto"
		}
	}
	switch s.Server + "/" + s.TLS {
	case "caddy/auto":
		if ip {
			return Site{}, errors.New("IP 地址申请不了公开证书：用 Caddy 自签（internal）、已有证书或不用 HTTPS")
		}
	case "caddy/internal", "caddy/http", "caddy/custom", "nginx/http", "nginx/custom":
	case "nginx/le":
		if ip || port != "" {
			return Site{}, errors.New("Let's Encrypt 只能给不带端口的域名申请证书（要用 80 端口验证）")
		}
		if !req.Agree {
			return Site{}, errors.New("申请 Let's Encrypt 证书要先同意它的订户协议")
		}
		if req.Email != "" && !emailRe.MatchString(req.Email) {
			return Site{}, errors.New("邮箱格式不对")
		}
	default:
		return Site{}, errors.New("不支持这种 HTTPS 方式")
	}
	if s.TLS == "custom" {
		for _, p := range []string{req.Cert, req.Key} {
			if !certPathRe.MatchString(p) || strings.Contains(p, "..") {
				return Site{}, errors.New("证书和私钥要是 /etc/letsencrypt、/etc/ssl、/etc/pki、/etc/nginx、/etc/caddy、/opt、/srv 或 /var/lib 下面的 .pem / .crt / .cer / .key 文件")
			}
		}
	}
	if s.TLS == "http" {
		s.HSTS, s.KeepHTTP = false, false
	}
	if s.Kind != "redirect" {
		s.Perm = false
	}
	return s, nil
}

// ---------- Caddy ----------

func (s Site) caddyConfig(cert, key string) string {
	var b strings.Builder
	b.WriteString(s.marker() + "\n")
	addr := s.Address
	switch {
	case s.TLS == "http":
		addr = "http://" + s.Address
	case s.KeepHTTP:
		addr = s.Address + ", http://" + s.Address
	}
	b.WriteString(addr + " {\n")
	switch s.TLS {
	case "internal":
		b.WriteString("\ttls internal\n")
	case "custom":
		b.WriteString("\ttls " + cert + " " + key + "\n")
	}
	access, _ := LogFiles("caddy", s.Address)
	b.WriteString("\tlog {\n\t\toutput file " + access + "\n\t}\n")
	if s.MaxBody > 0 && s.Kind != "redirect" {
		b.WriteString("\trequest_body {\n\t\tmax_size " + strconv.Itoa(s.MaxBody) + "MB\n\t}\n")
	}
	if s.HSTS {
		b.WriteString("\theader Strict-Transport-Security \"max-age=31536000\"\n")
	}
	switch s.Kind {
	case "proxy":
		b.WriteString("\treverse_proxy " + s.Target + "\n")
	case "static":
		b.WriteString("\troot * " + s.Target + "\n\tfile_server\n")
	case "redirect":
		code := "temporary"
		if s.Perm {
			code = "permanent"
		}
		b.WriteString("\tredir " + s.Target + " " + code + "\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func caddySiteAdd(s Site, req SiteRequest) string {
	file := SiteFile("caddy", s.Address)
	var b strings.Builder
	b.WriteString(step("mkdir -p "+SitesDir, "创建 "+SitesDir+" 失败"))
	b.WriteString(step("install -d -o caddy -g caddy /var/log/caddy", "创建日志目录 /var/log/caddy 失败"))
	b.WriteString(`grep -qs '^import ` + SitesDir + `/' ` + Caddyfile + ` || { ` +
		pipeRoot(`printf '\n%s\n' 'import `+SitesDir+`/*.caddy'`, "tee -a "+Caddyfile+" >/dev/null") +
		`; } || { echo 'hostower: 改不了 ` + Caddyfile + `'; exit 1; }; `)
	b.WriteString(writeRoot(file, s.caddyConfig(req.Cert, req.Key), "写站点配置失败"))
	b.WriteString("{ " + asRoot("caddy validate --config "+Caddyfile+" --adapter caddyfile") + "; } || { " +
		asRoot("mv -f "+file+" "+file+".rejected") + "; echo 'hostower: Caddy 配置校验没通过，已撤下这个站点（地址重复或者写法不对）'; exit 1; }; ")
	b.WriteString(step("systemctl reload caddy", "重载 Caddy 失败"))
	return b.String()
}

// ---------- Nginx ----------

func (s Site) nginxBody(b *strings.Builder) {
	switch s.Kind {
	case "proxy":
		b.WriteString("    location / {\n" +
			"        proxy_pass http://" + s.Target + ";\n" +
			"        proxy_http_version 1.1;\n" +
			"        proxy_set_header Host $host;\n" +
			"        proxy_set_header X-Real-IP $remote_addr;\n" +
			"        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n" +
			"        proxy_set_header X-Forwarded-Proto $scheme;\n" +
			"        proxy_set_header Upgrade $http_upgrade;\n" +
			"        proxy_set_header Connection $http_connection;\n" +
			"        proxy_read_timeout 3600s;\n" +
			"    }\n")
	case "static":
		b.WriteString("    root " + s.Target + ";\n    index index.html index.htm;\n    location / {\n        try_files $uri $uri/ =404;\n    }\n")
	case "redirect":
		code := "302"
		if s.Perm {
			code = "301"
		}
		b.WriteString("    location / {\n        return " + code + " " + s.Target + ";\n    }\n")
	}
}

// nginxConfig 渲染站点文件。tls 是这一次实际要写的方式：申请 Let's Encrypt 证书之前先写一份 http 的。
func (s Site) nginxConfig(tls, cert, key string) string {
	host, port, err := net.SplitHostPort(s.Address)
	if err != nil {
		host, port = s.Address, ""
	}
	if tls == "le" {
		cert, key = "/etc/letsencrypt/live/"+host+"/fullchain.pem", "/etc/letsencrypt/live/"+host+"/privkey.pem"
	}
	shown := s
	shown.TLS = tls
	access, errlog := LogFiles("nginx", s.Address)
	common := func(b *strings.Builder) {
		b.WriteString("    server_name " + host + ";\n")
		b.WriteString("    access_log " + access + ";\n    error_log " + errlog + ";\n")
		if s.MaxBody > 0 {
			b.WriteString("    client_max_body_size " + strconv.Itoa(s.MaxBody) + "m;\n")
		}
	}
	acme := "    location ^~ /.well-known/acme-challenge/ {\n        root " + ACMERoot + ";\n        default_type text/plain;\n    }\n"
	listen := func(b *strings.Builder, p string, ssl bool) {
		x := ""
		if ssl {
			x = " ssl http2"
		}
		b.WriteString("    listen " + p + x + ";\n    listen [::]:" + p + x + ";\n")
	}
	var b strings.Builder
	b.WriteString(shown.marker() + "\n")
	if tls == "http" {
		p := port
		if p == "" {
			p = "80"
		}
		b.WriteString("server {\n")
		listen(&b, p, false)
		common(&b)
		if port == "" {
			b.WriteString(acme)
		}
		s.nginxBody(&b)
		b.WriteString("}\n")
		return b.String()
	}
	// HTTPS：不带端口时 80 端口要么跳转、要么同样提供服务（keepHttp），并且留着证书验证用的目录。
	if port == "" {
		b.WriteString("server {\n")
		listen(&b, "80", false)
		common(&b)
		b.WriteString(acme)
		if s.KeepHTTP {
			s.nginxBody(&b)
		} else {
			b.WriteString("    location / {\n        return 301 https://$host$request_uri;\n    }\n")
		}
		b.WriteString("}\n")
		port = "443"
	}
	b.WriteString("server {\n")
	listen(&b, port, true)
	common(&b)
	b.WriteString("    ssl_certificate " + cert + ";\n    ssl_certificate_key " + key + ";\n")
	if s.HSTS {
		b.WriteString("    add_header Strict-Transport-Security \"max-age=31536000\" always;\n")
	}
	s.nginxBody(&b)
	b.WriteString("}\n")
	return b.String()
}

func nginxTest(file, fail string) string {
	return "{ " + asRoot("nginx -t") + "; } || { " + asRoot("mv -f "+file+" "+file+".rejected") + "; echo " + snippet.Quote("hostower: "+fail) + "; exit 1; }; "
}

func nginxSiteAdd(st Status, s Site, req SiteRequest) (string, error) {
	file := SiteFile("nginx", s.Address)
	var b strings.Builder
	if s.TLS == "le" {
		if !st.Certbot.Installed {
			in, err := installPkgs(st, "certbot", "安装 certbot 失败")
			if err != nil {
				return "", err
			}
			b.WriteString(in)
		}
		b.WriteString(step("mkdir -p "+ACMERoot, "创建 "+ACMERoot+" 失败"))
		// 第一步：先用 HTTP 把站点开起来，Let's Encrypt 要从 80 端口访问验证文件。
		b.WriteString(writeRoot(file, s.nginxConfig("http", "", ""), "写站点配置失败"))
		b.WriteString(nginxTest(file, "Nginx 配置校验没通过，已撤下这个站点（地址重复或者写法不对）"))
		b.WriteString(step("systemctl reload nginx", "重载 Nginx 失败"))
		mail := "--register-unsafely-without-email"
		if req.Email != "" {
			mail = "-m " + snippet.Quote(req.Email)
		}
		host := s.Address
		b.WriteString(step("certbot certonly --webroot -w "+ACMERoot+" -d "+host+" --cert-name "+host+
			" --non-interactive --agree-tos "+mail+" --keep-until-expiring --deploy-hook 'systemctl reload nginx'",
			"申请证书失败：站点先以 HTTP 提供。请确认域名已经解析到这台主机、80 端口能从外面访问，再撤下站点重新添加"))
		// 第二步：证书到手，换成 HTTPS 的配置。
		b.WriteString(writeRoot(file, s.nginxConfig("le", "", ""), "写站点配置失败"))
		b.WriteString(nginxTest(file, "证书已经申请到，但 HTTPS 配置没通过校验，已撤下这个站点"))
	} else {
		b.WriteString(writeRoot(file, s.nginxConfig(s.TLS, req.Cert, req.Key), "写站点配置失败"))
		b.WriteString(nginxTest(file, "Nginx 配置校验没通过，已撤下这个站点（地址重复、证书路径不对或者写法不对）"))
	}
	b.WriteString(step("systemctl reload nginx", "重载 Nginx 失败"))
	return b.String(), nil
}

// SiteAddCommand 渲染“添加站点”。
func SiteAddCommand(st Status, req SiteRequest) (string, Site, error) {
	if !st.CanRoot() {
		return "", Site{}, st.rootErr("改不了站点配置")
	}
	s, err := st.normalize(req)
	if err != nil {
		return "", Site{}, err
	}
	var cmd string
	if s.Server == "caddy" {
		cmd = caddySiteAdd(s, req)
	} else if cmd, err = nginxSiteAdd(st, s, req); err != nil {
		return "", Site{}, err
	}
	what := s.Target
	if s.Kind == "redirect" {
		what = "→ " + s.Target
	}
	return cmd + `echo ` + snippet.Quote("hostower: 站点 "+s.Address+" "+what+" 已生效"), s, nil
}

// pipeRoot 把标准输入交给一条以 root 身份运行的命令（同 asRoot）。
func pipeRoot(input, cmd string) string {
	return input + " | " + asRoot(cmd)
}

func (st Status) site(addr string) (Site, error) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if _, err := ValidAddress(addr); err != nil {
		return Site{}, err
	}
	for _, s := range st.Sites {
		if s.Address == addr {
			return s, nil
		}
	}
	return Site{}, errors.New("没有这个站点（只能管 Hostower 添加的站点）")
}

// SiteRemoveCommand 渲染“撤下站点”：把站点文件挪成 .removed（不删，想恢复改回名字就行），再重载服务。证书留着不动。
func SiteRemoveCommand(st Status, addr string) (string, error) {
	s, err := st.site(addr)
	if err != nil {
		return "", err
	}
	if !st.CanRoot() {
		return "", st.rootErr("改不了站点配置")
	}
	file := SiteFile(s.Server, s.Address)
	return step("mv -f "+file+" "+file+".removed", "撤下站点失败") + step("systemctl reload "+s.Server, "重载 "+serverName(s.Server)+" 失败") +
		`echo ` + snippet.Quote("hostower: 站点 "+s.Address+" 已撤下"), nil
}

func serverName(server string) string {
	if server == "nginx" {
		return "Nginx"
	}
	return "Caddy"
}

// SiteLogCommand 渲染“看站点日志”：只读，最后 200 行。kind 是 access 或 error。
// 日志的位置完全由校验过的地址决定，不用先检测主机：站点不存在时只是读不到。
func SiteLogCommand(server, addr, kind string) (string, error) {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if _, err := ValidAddress(addr); err != nil {
		return "", err
	}
	if server != "caddy" && server != "nginx" {
		return "", errors.New("服务器只能是 Caddy 或 Nginx")
	}
	s := Site{Server: server, Address: addr}
	access, errlog := LogFiles(s.Server, s.Address)
	file := access
	switch {
	case kind == "error" && s.Server == "caddy":
		return "journalctl -u caddy -n 200 --no-pager 2>/dev/null || sudo -n journalctl -u caddy -n 200 --no-pager 2>&1", nil
	case kind == "error":
		file = errlog
	case kind != "access":
		return "", errors.New("日志只有访问日志和错误日志两种")
	}
	return "tail -n 200 " + file + " 2>/dev/null || sudo -n tail -n 200 " + file + " 2>&1", nil
}

// CaddyInstallCommand 渲染“安装 Caddy”：用系统软件源（Debian/Ubuntu、Fedora 等有现成的包），装好后开机自启。
func CaddyInstallCommand(st Status) (string, error) {
	if st.Caddy.Installed {
		return "", errors.New("Caddy 已经装好了")
	}
	if st.Nginx.Installed {
		return "", errors.New("这台主机已经装了 Nginx：两个都要用 80 和 443 端口，Hostower 每台主机只管一个反向代理")
	}
	if !st.CanRoot() {
		return "", st.rootErr("装不了 Caddy")
	}
	var b strings.Builder
	switch st.Family {
	case "debian":
		b.WriteString(step("apt-get install -y -q caddy", "安装 Caddy 失败：系统软件源里可能没有它，请按 Caddy 官网的说明添加它的软件源后再装"))
	case "rhel":
		b.WriteString(step("dnf install -y -q caddy", "安装 Caddy 失败：RHEL 系通常要先启用 EPEL 或 Caddy 的 COPR 源"))
	default:
		return "", errors.New("这台主机的系统不支持一键安装 Caddy，请按 Caddy 官网的说明安装")
	}
	b.WriteString(step("systemctl enable --now caddy", "启动 Caddy 失败"))
	b.WriteString(`echo 'hostower: Caddy 已安装并启动'`)
	return b.String(), nil
}

// NginxInstallCommand 渲染“安装 Nginx”：系统软件源，开机自启；RHEL 系还要允许它连本机端口（SELinux），否则反向代理会被挡住。
func NginxInstallCommand(st Status) (string, error) {
	if st.Nginx.Installed {
		return "", errors.New("Nginx 已经装好了")
	}
	if st.Caddy.Installed {
		return "", errors.New("这台主机已经装了 Caddy：两个都要用 80 和 443 端口，Hostower 每台主机只管一个反向代理")
	}
	if !st.CanRoot() {
		return "", st.rootErr("装不了 Nginx")
	}
	if st.Family != "debian" && st.Family != "rhel" {
		return "", errors.New("这台主机的系统不支持一键安装 Nginx，请在主机上自己安装")
	}
	in, err := installPkgs(st, "nginx", "安装 Nginx 失败")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(in)
	if st.Family == "rhel" {
		b.WriteString("if command -v setsebool >/dev/null 2>&1; then " + step("setsebool -P httpd_can_network_connect 1", "设置 SELinux 允许 Nginx 反向代理失败") + "fi; ")
	}
	b.WriteString(step("mkdir -p "+ACMERoot, "创建 "+ACMERoot+" 失败"))
	b.WriteString(step("systemctl enable --now nginx", "启动 Nginx 失败（80 端口可能被别的程序占用了）"))
	b.WriteString(`echo 'hostower: Nginx 已安装并启动'`)
	return b.String(), nil
}

// ---------- 证书（certbot） ----------

// Cert 是 certbot 管理的一张证书。
type Cert struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Expiry  string   `json:"expiry"` // certbot 原样输出的到期时间
	Days    int      `json:"days"`   // 还剩几天；已过期是负数
	Valid   bool     `json:"valid"`
}

var certDaysRe = regexp.MustCompile(`\((VALID|INVALID)[^)]*?(-?\d+) day`)

func parseCerts(lines []string) []Cert {
	out := []Cert{}
	var cur *Cert
	for _, l := range lines {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "Certificate Name:"):
			out = append(out, Cert{Name: strings.TrimSpace(strings.TrimPrefix(l, "Certificate Name:")), Domains: []string{}})
			cur = &out[len(out)-1]
		case cur == nil:
		case strings.HasPrefix(l, "Domains:"):
			cur.Domains = strings.Fields(strings.TrimPrefix(l, "Domains:"))
		case strings.HasPrefix(l, "Expiry Date:"):
			v := strings.TrimSpace(strings.TrimPrefix(l, "Expiry Date:"))
			if i := strings.Index(v, " ("); i >= 0 {
				cur.Expiry = v[:i]
			} else {
				cur.Expiry = v
			}
			if m := certDaysRe.FindStringSubmatch(v); m != nil {
				cur.Valid = m[1] == "VALID"
				cur.Days, _ = strconv.Atoi(m[2])
				if strings.Contains(v, "EXPIRED") && cur.Days > 0 {
					cur.Days = -cur.Days
				}
			}
		}
	}
	return out
}

// CertRequest 是一次证书操作。
type CertRequest struct {
	Action string `json:"action"` // dry-run（演练续期）| renew（续期快到期的）| force（强制续期一张）
	Name   string `json:"name"`
}

// CertCommand 渲染证书操作。续期之后重载用到证书的服务。
func CertCommand(st Status, req CertRequest) (string, string, error) {
	if !st.Certbot.Installed {
		return "", "", errors.New("这台主机没有装 certbot")
	}
	if !st.CanRoot() {
		return "", "", st.rootErr("续期不了证书")
	}
	hook := ""
	if st.Nginx.Installed {
		hook = " --deploy-hook 'systemctl reload nginx'"
	}
	switch req.Action {
	case "dry-run":
		return step("certbot renew --dry-run", "演练续期失败") + `echo 'hostower: 演练续期通过，到期前会自动续期'`, "", nil
	case "renew":
		return step("certbot renew"+hook, "续期失败") + `echo 'hostower: 续期完成（还没到续期时间的证书不会动）'`, "", nil
	case "force":
		found := false
		for _, c := range st.Certs {
			if c.Name == req.Name {
				found = true
			}
		}
		if !certNameRe.MatchString(req.Name) || !found {
			return "", "", errors.New("没有这张证书")
		}
		return step("certbot renew --cert-name "+req.Name+" --force-renewal"+hook, "续期失败") +
			`echo ` + snippet.Quote("hostower: 证书 "+req.Name+" 已续期"), req.Name, nil
	}
	return "", "", fmt.Errorf("未知的操作")
}
