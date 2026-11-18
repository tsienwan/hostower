// Command hostower 是塔台的单一可执行文件：服务端和命令行工具都在这里。
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/tsienwan/hostower/internal/app"
	"github.com/tsienwan/hostower/internal/auth"
	"github.com/tsienwan/hostower/internal/executor"
	"github.com/tsienwan/hostower/internal/gateway"
	"github.com/tsienwan/hostower/internal/kernel"
	"github.com/tsienwan/hostower/internal/kernel/audit"
	"github.com/tsienwan/hostower/internal/kernel/risk"
	"github.com/tsienwan/hostower/internal/metrics"
	"github.com/tsienwan/hostower/internal/oidc"
	"github.com/tsienwan/hostower/internal/secrets"
	"github.com/tsienwan/hostower/internal/selfmon"
	"github.com/tsienwan/hostower/internal/speedtest"
	"github.com/tsienwan/hostower/internal/store"
	"github.com/tsienwan/hostower/internal/update"
	"github.com/tsienwan/hostower/web"
)

// version 在发布构建时由 -ldflags 注入。
var version = "dev"

// updateRepo（owner/name）和 updateKey（发布公钥）也在发布构建时注入；
// 没有它们的构建不会自我更新。
var (
	updateRepo string
	updateKey  string
)

const usage = `Hostower —— 自托管的多主机运维灯塔

用法：
  hostower serve [--dev]                启动服务
  hostower classify <命令>              给一条命令判级（不执行）
  hostower exec --host <主机> <命令>    通过内核在主机上执行（L1 会当场询问；L2 只能在网页上批准）
  hostower audit verify                 离线校验审计哈希链
  hostower keygen                       生成主密钥
  hostower secret set-ssh <名称> --user <用户> --key-file <私钥文件>
  hostower hostkey forget <主机>        清除固定的主机公钥
  hostower auth reset-password          重新生成所有者的密码（写进数据目录的 initial-password，并打开密码登录、吊销所有会话）
  hostower healthcheck                  容器健康检查
  hostower version                      版本、构建封签和发布公钥指纹
  hostower verify <公钥> [文件]          用给定的发布公钥校验可执行文件的构建封签（默认校验自己）

环境变量（监听、访问地址、TLS、会话时长也可以在“设置”页里改；环境变量优先）：
  HOSTOWER_DATA             数据目录（默认 ./data）
  HOSTOWER_CONFIG           配置目录：inventory.yaml、snippets/、rules/（默认 ./config）
  HOSTOWER_LISTEN           监听地址（默认 127.0.0.1:18443）
  HOSTOWER_PUBLIC_URL       浏览器访问塔台用的地址（Passkey 绑定到它）
  HOSTOWER_TLS_CERT / _KEY  证书与私钥文件：塔台自己终结 TLS
  HOSTOWER_TLS=proxy        声明前面有一层终结 TLS 的反向代理
  HOSTOWER_SESSION_MINUTES  登录会话时长（分钟，默认 720）
  HOSTOWER_ADMIN_USER       首次启动时创建的所有者用户名（默认 admin；登录后可以在“设置 → 安全”里改）
  HOSTOWER_MASTER_KEY_FILE  主密钥文件（凭据加密用，不进数据库）；不设置时用数据目录里自动生成的 master_key
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	var err error
	switch cmd, args := os.Args[1], os.Args[2:]; cmd {
	case "serve":
		err = serve(args)
	case "classify":
		err = classify(args)
	case "exec":
		err = execCmd(args)
	case "audit":
		err = auditCmd(args)
	case "keygen":
		fmt.Println(secrets.GenerateMasterKey())
	case "secret":
		err = secretCmd(args)
	case "hostkey":
		err = hostkeyCmd(args)
	case "auth":
		err = authCmd(args)
	case "healthcheck":
		err = healthcheck()
	case "version":
		fmt.Println("hostower", version, "schema", store.SchemaVersion())
		b := buildInfo()
		fmt.Println("构建：", buildText[b.Status])
		if b.Fingerprint != "" {
			fmt.Println("发布公钥指纹：", b.Fingerprint)
		}
		fmt.Println("许可证：AGPL-3.0-only")
	case "verify":
		err = verifyCmd(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dev := fs.Bool("dev", os.Getenv("HOSTOWER_DEV") == "1", "开发模式：只监听本机，注册模拟主机执行器，允许用启动令牌登录")
	fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// 数据目录里如果有一份验过签、比自己新的版本，就接力过去：容器重启后跑的仍然是更新过的版本。
	dataDir := app.ConfigFromEnv(*dev).DataDir
	if os.Getenv(update.HandoffEnv) == "" {
		if path, v, ok := update.Staged(dataDir, version, updateKey); ok {
			log.Info("切换到数据目录里的新版本", "from", version, "to", v)
			if err := update.MarkPending(dataDir); err == nil {
				err = update.Exec(path)
				log.Error("切换失败，继续运行当前版本", "err", err)
			}
			update.Discard(dataDir)
		}
	}
	a, err := app.Open(app.ConfigFromEnv(*dev), log)
	if err != nil {
		return err
	}
	defer a.Close()
	// 正在生效的配置 = 环境变量 + 面板设置（环境变量优先）。
	cfg := a.Config
	// 启动自检：不安全的暴露方式直接拒绝启动，而不是给一条警告。
	if err := cfg.SelfCheck(); err != nil {
		return fmt.Errorf("%w\n（如果是面板设置写错了：设置 HOSTOWER_IGNORE_PANEL=1 可以跳过它启动）", err)
	}

	authSvc, err := auth.New(a.DB, cfg.PublicURL, cfg.Dev)
	if err != nil {
		return err
	}
	authSvc.SetSessionTTL(time.Duration(cfg.SessionMinutes) * time.Minute)
	authSvc.SetSecrets(a.Secrets)
	authSvc.SetDataDir(cfg.DataDir)
	// 首次启动：创建所有者，生成初始密码写进数据目录里的文件。密码不进日志（日志常被收集到别处）。
	initialPassword, err := authSvc.EnsureOwner(os.Getenv("HOSTOWER_ADMIN_USER"))
	if err != nil {
		return fmt.Errorf("创建所有者失败：%w", err)
	}
	if initialPassword != "" {
		a.Kernel.Audit().Append(context.Background(), audit.Entry{Type: "auth.setup", Entry: "system", Message: "首次启动：创建所有者并生成初始密码"})
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 每台主机一条长连接持续采样；这里的间隔是向页面推送和落盘的节奏。
	interval := executor.StreamProbeInterval
	sampler := metrics.NewSampler(a.Kernel, interval, a.DB)
	a.Kernel.Audit().Append(ctx, audit.Entry{Type: "system.started", Entry: "system",
		Message: "塔台启动", Detail: map[string]any{"version": version, "dev": cfg.Dev, "probe": executor.StreamProbeCommand, "probeInterval": interval.String()}})
	go sampler.Run(ctx)
	self := selfmon.New(cfg.DataDir)
	go self.Run(ctx, 5*time.Second)
	go a.Notify.Run(ctx, a.Kernel)
	speed := speedtest.New(a.DB)
	// 审计归档：启动一分钟后看一次，之后每 6 小时看一次；条件由所有者在设置里定，默认不归档。
	archiveDir := filepath.Join(cfg.DataDir, "audit-archive")
	go func() {
		wait := time.Minute
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			wait = 6 * time.Hour
			if cp, err := a.Kernel.Audit().Maintain(ctx, archiveDir); err != nil {
				log.Error("审计归档失败", "err", err)
			} else if cp != nil {
				log.Info("已归档审计记录", "first", cp.First, "upto", cp.Upto, "file", cp.File)
			}
		}
	}()
	// 回调地址跟着访问地址走：身份提供方那边要填同一个地址。
	oidcSvc, err := oidc.New(a.DB, a.Secrets, func() string { return strings.TrimRight(cfg.PublicURL, "/") + "/api/v1/auth/oidc/callback" })
	if err != nil {
		return err
	}
	go speed.Run(ctx, a.Kernel)

	// 自我更新：新版本就位后，优雅停下服务，再原地换成新的可执行文件。
	restartTo := make(chan string, 1)
	updater := update.New(update.Config{
		Version: version, Repo: updateRepo, PubKey: updateKey, API: os.Getenv("HOSTOWER_UPDATE_API"),
		DataDir: cfg.DataDir, DB: a.DB, Log: log,
		Audit: func(typ, msg string, detail map[string]any) {
			a.Kernel.Audit().Append(context.Background(), audit.Entry{Type: typ, Entry: "system", Message: msg, Detail: detail})
		},
		Restart: func(path string) {
			select {
			case restartTo <- path:
			default:
			}
			stop()
		},
	})
	go updater.Run(ctx)

	build := buildInfo()
	if build.Status == update.BuildInvalid {
		log.Warn("构建封签对不上：这个可执行文件被改过，或者不是用编译进来的发布公钥签的", "fingerprint", build.Fingerprint)
	}
	// 从页面重启：优雅停下，再用磁盘上同一个位置的可执行文件原地启动（换过程序文件的话就是新程序）。
	relaunch := make(chan string, 1)
	var restartFn func() error
	if runtime.GOOS == "linux" {
		restartFn = func() error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			select {
			case relaunch <- exe:
			default:
			}
			stop()
			return nil
		}
	}
	gw, err := gateway.New(gateway.Options{
		Build: build, SourceRepo: updateRepo, Restart: restartFn,
		Version: version, Dev: cfg.Dev, PublicURL: cfg.PublicURL, TrustProxy: cfg.TLSMode == "proxy",
		Kernel: a.Kernel, Auth: authSvc, Sampler: sampler, Secrets: a.Secrets,
		Snippets: a.Snippets, Reload: a.Reload, SaveHost: a.SaveHost, DeleteHost: a.DeleteHost, SaveSnippet: a.SaveSnippet, DeleteSnippet: a.DeleteSnippet, Executors: a.Executors,
		Panel: a.Panel, SavePanel: a.SavePanel, Started: a.Started, Self: self.Status, Update: updater, Notify: a.Notify, Speedtest: speed, OIDC: oidcSvc, AuditArchiveDir: archiveDir, DB: a.DB, Web: web.FS(), Log: log,
	})
	if err != nil {
		return err
	}
	go gw.RunChatBot(ctx)
	go gw.RunAppUpdates(ctx)
	// 先占住端口，再往下走：端口被占用时在这里就失败，不会打印出一个没人认的令牌。
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("无法监听 %s（是不是已经有一个 Hostower 在运行？）：%w", cfg.Listen, err)
	}
	// 能走到这里说明这个版本起得来：上一次切换算成功。
	update.MarkHealthy(cfg.DataDir)
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           gw.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS13},
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()

	log.Info("Hostower 已启动", "version", version, "listen", cfg.Listen, "url", cfg.PublicURL, "dev", cfg.Dev,
		"hosts", len(a.Kernel.Inventory().Hosts()))
	if cfg.Dev {
		log.Warn("开发模式：主机是模拟的，可以用下面的令牌直接登录。不要在真实环境使用 --dev")
	}
	if !a.Secrets.Available() {
		log.Warn("没有配置主密钥，SSH 凭据不可用（hostower keygen 生成，HOSTOWER_MASTER_KEY_FILE 指向它）")
	}
	if tok := authSvc.SetupToken(); tok != "" {
		// 开发模式的启动令牌只写进启动日志。
		fmt.Fprintf(os.Stderr, "\n  初始化 / 登录令牌：%s\n  打开 %s 并输入它。\n\n", tok, cfg.PublicURL)
	}
	if initialPassword != "" {
		owner := os.Getenv("HOSTOWER_ADMIN_USER")
		if owner == "" {
			owner = auth.DefaultOwnerName
		}
		fmt.Fprintf(os.Stderr, "\n  已创建所有者 %s，初始密码在 %s（权限 0600）。\n  用它登录 %s，登录后请修改密码，并添加 Passkey 或动态码。\n  用 Docker 时：docker exec <容器> cat /data/%s\n\n",
			owner, initialPassword, cfg.PublicURL, auth.InitialPasswordFile)
	}
	if cfg.TLSCert != "" {
		err = srv.ServeTLS(ln, cfg.TLSCert, cfg.TLSKey)
	} else {
		err = srv.Serve(ln)
	}
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	select {
	case path := <-restartTo:
		log.Info("更新就位，切换到新版本")
		a.Close()
		if perr := update.MarkPending(cfg.DataDir); perr != nil {
			return perr
		}
		// 成功时不返回：进程原地换成新版本。
		err = update.Exec(path)
		update.Discard(cfg.DataDir)
		return fmt.Errorf("切换到新版本失败：%w", err)
	case exe := <-relaunch:
		log.Info("按页面上的请求重启")
		a.Close()
		err = update.Relaunch(exe)
		return fmt.Errorf("重启失败：%w", err)
	default:
	}
	return err
}

func classify(args []string) error {
	if len(args) == 0 {
		return errors.New("用法：hostower classify <命令>")
	}
	cfg := app.ConfigFromEnv(false)
	packs, err := app.LoadPacks(cfg.ConfigDir)
	if err != nil {
		return err
	}
	v := risk.New(packs...).Classify(strings.Join(args, " "))
	fmt.Println(v.Level)
	for _, f := range v.Findings {
		fmt.Printf("  %s  %-22s %s", f.Level, f.Rule, f.Reason)
		if f.Command != "" {
			fmt.Printf("  ← %s", f.Command)
		}
		fmt.Println()
	}
	return nil
}

func execCmd(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ExitOnError)
	host := fs.String("host", "", "主机名或选择器")
	dev := fs.Bool("dev", os.Getenv("HOSTOWER_DEV") == "1", "开发模式（模拟主机）")
	yes := fs.Bool("yes", false, "L1 不再询问，直接批准")
	fs.Parse(args)
	command := strings.Join(fs.Args(), " ")
	if *host == "" || command == "" {
		return errors.New("用法：hostower exec --host <主机或选择器> <命令>")
	}
	a, err := app.Open(app.ConfigFromEnv(*dev), slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
	if err != nil {
		return err
	}
	defer a.Close()
	hosts, err := a.Kernel.Inventory().Select(*host)
	if err != nil {
		return err
	}
	if len(hosts) == 0 {
		return fmt.Errorf("%q 没有匹配到主机", *host)
	}
	spec := kernel.CommandSpec{Commands: map[string]string{}, Intent: "命令行执行"}
	for _, h := range hosts {
		spec.Hosts = append(spec.Hosts, h.Name)
		spec.Commands[h.Name] = command
	}
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME")
	}
	// 命令行调用者是这台机器上能读数据目录的人，等同所有者；但 L2 只能在网页上批准。
	me := kernel.Actor{ID: "cli:" + user, Name: user + "（命令行）", Kind: kernel.Human, Entry: "cli", MaxLevel: risk.L2, CanApprove: true}
	ctx := context.Background()
	events, unsubscribe := a.Kernel.Subscribe()
	defer unsubscribe()
	req, err := a.Kernel.SubmitCommand(ctx, me, spec)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "请求 %s  等级 %s  主机 %d 台\n", req.ID, req.Level, len(req.Targets))
	for _, t := range req.Targets {
		for _, f := range t.Findings {
			if f.Level > risk.L0 {
				fmt.Fprintf(os.Stderr, "  %s %s：%s\n", f.Level, t.Host, f.Reason)
			}
		}
	}
	switch req.State {
	case kernel.StateRejected:
		return kernel.ErrForbidden
	case kernel.StatePending:
		if req.Level >= risk.L2 {
			a.Kernel.Cancel(ctx, req.ID, me)
			return kernel.ErrL2WebOnly
		}
		if !*yes {
			fmt.Fprintf(os.Stderr, "将在 %d 台主机上执行：%s\n批准？[y/N] ", len(req.Targets), command)
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if s := strings.ToLower(strings.TrimSpace(line)); s != "y" && s != "yes" {
				a.Kernel.Deny(ctx, req.ID, me, "命令行拒绝")
				return errors.New("已拒绝")
			}
		}
		if _, err := a.Kernel.Approve(ctx, req.ID, me, kernel.Proof{Digest: req.Digest}); err != nil {
			return err
		}
	}
	multi := len(req.Targets) > 1
	for ev := range events {
		switch d := ev.Data.(type) {
		case kernel.OutputChunk:
			if d.ID != req.ID {
				continue
			}
			if multi {
				for _, line := range strings.SplitAfter(d.Chunk, "\n") {
					if line != "" {
						fmt.Printf("[%s] %s", d.Host, line)
					}
				}
			} else {
				fmt.Print(d.Chunk)
			}
		case *kernel.Request:
			if d.ID != req.ID || d.State == kernel.StateRunning || d.State == kernel.StatePending {
				continue
			}
			failed := 0
			for _, t := range d.Targets {
				if t.State != "ok" {
					failed++
					fmt.Fprintf(os.Stderr, "[%s] %s %s\n", t.Host, t.State, t.Error)
				}
			}
			fmt.Fprintf(os.Stderr, "结束：%s（审计 #%d）\n", d.State, d.AuditSeq)
			if failed > 0 {
				os.Exit(1)
			}
			return nil
		}
	}
	return nil
}

func auditCmd(args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return errors.New("用法：hostower audit verify")
	}
	cfg := app.ConfigFromEnv(false)
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	res, err := audit.Verify(context.Background(), db)
	if err != nil {
		return err
	}
	// 归档过的记录在数据目录的 audit-archive/ 里：逐个校验文件摘要和首尾相接。
	arch, err := audit.VerifyArchives(context.Background(), db, filepath.Join(cfg.DataDir, "audit-archive"))
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(map[string]any{"database": res, "archives": arch}, "", "  ")
	fmt.Println(string(out))
	if !res.OK || !arch.OK {
		os.Exit(1)
	}
	return nil
}

func secretCmd(args []string) error {
	if len(args) < 2 || args[0] != "set-ssh" {
		return errors.New("用法：hostower secret set-ssh <名称> --user <用户> --key-file <私钥文件>")
	}
	name := args[1]
	fs := flag.NewFlagSet("secret", flag.ExitOnError)
	user := fs.String("user", "", "登录用户名（不要用 root）")
	keyFile := fs.String("key-file", "", "私钥文件")
	fs.Parse(args[2:])
	if *user == "" || *keyFile == "" {
		return errors.New("需要 --user 和 --key-file")
	}
	key, err := os.ReadFile(*keyFile)
	if err != nil {
		return err
	}
	master, err := secrets.LoadMasterKey()
	if err != nil {
		return err
	}
	cfg := app.ConfigFromEnv(false)
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	if secrets.Reserved(name) {
		return errors.New("这个名字是系统自己用的，换一个")
	}
	body, _ := json.Marshal(executor.SSHCredential{User: *user, PrivateKey: string(key)})
	if err := secrets.New(db, master).Put(name, body); err != nil {
		return err
	}
	audit.Open(db, nil).Append(context.Background(), audit.Entry{Type: "secret.updated", Entry: "cli", Message: "写入凭据 " + name})
	fmt.Println("已写入", name, "（在清单里用 credential: ref:"+name+" 引用）")
	return nil
}

func hostkeyCmd(args []string) error {
	if len(args) != 2 || args[0] != "forget" {
		return errors.New("用法：hostower hostkey forget <主机>")
	}
	db, err := store.Open(app.ConfigFromEnv(false).DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	audit.Open(db, nil).Append(context.Background(), audit.Entry{Type: "hostkey.forgotten", Entry: "cli", Host: args[1]})
	return executor.ForgetHostKey(db, args[1])
}

// authCmd 是登录方式全丢了时的兜底：只有能进到服务器（或容器）里的人才能执行。
func authCmd(args []string) error {
	if len(args) != 1 || args[0] != "reset-password" {
		return errors.New("用法：hostower auth reset-password")
	}
	cfg := app.ConfigFromEnv(false)
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	svc, err := auth.New(db, "http://localhost", false)
	if err != nil {
		return err
	}
	svc.SetDataDir(cfg.DataDir)
	if _, err := audit.Open(db, nil).Append(context.Background(), audit.Entry{Type: "auth.password.reset", Entry: "cli",
		Message: "在命令行重置所有者的密码，打开密码登录并吊销所有会话"}); err != nil {
		return fmt.Errorf("写审计失败，没有重置：%w", err)
	}
	path, err := svc.ResetPassword()
	if err != nil {
		return err
	}
	fmt.Println("已重置所有者的密码，新密码在", path)
	fmt.Println("所有会话已失效；用新密码登录后请修改密码。")
	return nil
}

func healthcheck() error {
	cfg := app.ConfigFromEnv(false)
	if db, err := store.Open(cfg.DataDir); err == nil {
		if stored, err := app.LoadPanel(db); err == nil && os.Getenv("HOSTOWER_IGNORE_PANEL") != "1" {
			cfg = cfg.WithPanel(stored)
		}
		db.Close()
	}
	scheme := "http"
	if cfg.TLSCert != "" {
		scheme = "https"
	}
	addr := cfg.Listen
	if host, port, ok := strings.Cut(addr, ":"); ok && (host == "" || host == "0.0.0.0" || host == "[::]") {
		addr = "127.0.0.1:" + port
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		// 只对本机回环地址做探活，证书的主机名对不上是预期的。
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get(scheme + "://" + addr + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("状态码 %d", resp.StatusCode)
	}
	return nil
}

var buildText = map[string]string{
	update.BuildOfficial: "发布签名校验通过（对照下面的公钥指纹和官方公布的是否一致）",
	update.BuildUnsealed: "没有构建封签（自己编译的或开发版）",
	update.BuildInvalid:  "封签对不上：文件被改过，或者不是用编译进来的发布公钥签的",
	update.BuildNoKey:    "没有编译进发布公钥（自己编译的或开发版）",
}

// buildInfo 校验正在运行的这个可执行文件的构建封签。
func buildInfo() update.BuildInfo {
	exe, err := os.Executable()
	if err != nil {
		return update.BuildInfo{Status: update.BuildInvalid}
	}
	return update.CheckSeal(exe, updateKey)
}

// verifyCmd 用给定的公钥离线校验一个可执行文件（默认是自己）：不依赖文件里编译进去的任何东西。
func verifyCmd(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("用法：hostower verify <发布公钥（base64）> [文件]")
	}
	path := ""
	if len(args) == 2 {
		path = args[1]
	} else {
		p, err := os.Executable()
		if err != nil {
			return err
		}
		path = p
	}
	info, err := update.VerifyFile(path, args[0])
	if err != nil {
		return err
	}
	fmt.Println("封签正确：这个文件由这把公钥对应的私钥签出，没有被改过。公钥指纹：", info.Fingerprint)
	return nil
}
