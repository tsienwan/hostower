//go:build linux

package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/tsienwan/hostower/internal/inventory"
)

const localShell = "/bin/sh"

// Local 直接在 Hostower 所在的这台机器上执行，不经过网络。
// 它和别的执行器一样只是通道：判级、审批、审计仍然全部在内核里。
// 命令以运行 Hostower 的那个账号的身份执行，没有 SSH 那一层账号隔离。
//
// 宿主机模式（NewHostLocal）用在容器部署里：Hostower 在容器里，但命令进入宿主机的命名空间执行，
// 文件经 /proc/1/root 读写，所以看到和操作的是宿主机而不是容器自己。它需要特权容器和宿主机的进程命名空间，
// 在宿主机上的身份是 root。
type Local struct {
	// enter 是进入宿主机命名空间的命令前缀；为空表示就在当前环境里执行。
	enter []string
	// root 是宿主机根文件系统在本进程里的路径；为空表示就是 /。
	root string
	// env 是宿主机模式下命令的环境。
	env []string
}

const hostRoot = "/proc/1/root"

var hostEnter = []string{"nsenter", "-t", "1", "-m", "-u", "-i", "-n", "--"}

// bundledBin 是镜像里自带的工具目录（一份静态的 docker 命令）。
const bundledBin = "/opt/hostower/bin"

// hostEnv 是宿主机模式下命令的环境：不把容器自己的环境变量带到宿主机上。
// 有些宿主机上只有 Docker 服务、没有 docker 命令（比如 Docker Desktop 的虚拟机）：
// 这时把镜像里自带的那份接在 PATH 的最后，宿主机自己有的永远优先。
// 进了宿主机的挂载命名空间之后，容器里的文件要经 /proc/<本进程>/root 才看得到。
func hostEnv() []string {
	search := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	if st, err := os.Stat(bundledBin); err == nil && st.IsDir() {
		search += fmt.Sprintf(":/proc/%d/root%s", os.Getpid(), bundledBin)
	}
	return []string{"PATH=" + search, "HOME=/root", "USER=root", "LOGNAME=root", utf8Locale}
}

// utf8Locale：没有设置语言环境时，ls 这类程序会把中文、表情符号的文件名打印成 \360\237 这样的转义。
const utf8Locale = "LANG=C.UTF-8"

// withLocale 在环境里没有任何语言设置时补上 UTF-8。
func withLocale(env []string) []string {
	for _, e := range env {
		if (strings.HasPrefix(e, "LANG=") || strings.HasPrefix(e, "LC_ALL=") || strings.HasPrefix(e, "LC_CTYPE=")) && !strings.HasSuffix(e, "=") {
			return env
		}
	}
	return append(env, utf8Locale)
}

// NewLocal 返回本机执行器；这台机器上用不了（比如没有 shell 的容器镜像）时返回 nil。
func NewLocal() *Local {
	if st, err := os.Stat(localShell); err != nil || st.IsDir() {
		return nil
	}
	return &Local{}
}

// NewHostLocal 返回宿主机模式的本机执行器，并当场确认真的进得去宿主机。
func NewHostLocal() (*Local, error) {
	l := &Local{enter: hostEnter, root: hostRoot, env: hostEnv()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	code, err := l.Exec(ctx, inventory.Host{}, "true", &out)
	if err != nil || code != 0 {
		detail := strings.TrimSpace(out.String())
		if err != nil {
			detail = strings.TrimSpace(detail + " " + err.Error())
		}
		return nil, fmt.Errorf("进不了宿主机：这个镜像要以特权方式运行。docker run 加上 --privileged --pid=host，compose 里写 privileged: true 和 pid: host（示例见 deploy/compose.host.yaml）；不想给特权就改用默认镜像。原始错误：%s", detail)
	}
	return l, nil
}

// build 组装一条要执行的脚本：普通模式是 sh -c；宿主机模式先进入宿主机的命名空间。
func (l *Local) build(ctx context.Context, script string) *exec.Cmd {
	if l.enter == nil {
		cmd := exec.CommandContext(ctx, localShell, "-c", script)
		cmd.Dir = localHome()
		cmd.Env = withLocale(os.Environ())
		return cmd
	}
	args := append(append([]string{}, l.enter[1:]...), localShell, "-c", `cd "$HOME" 2>/dev/null || cd /; exec /bin/sh -c "$0"`, script)
	cmd := exec.CommandContext(ctx, l.enter[0], args...)
	cmd.Dir = "/"
	cmd.Env = append([]string{}, l.env...)
	return cmd
}

// path 把主机上的路径换成本进程能打开的路径。宿主机模式下逐级解析符号链接：
// 链接指向的绝对路径是相对宿主机根目录的，交给内核去解析会落到容器自己的文件系统里。
func (l *Local) path(p string) (string, error) {
	if l.root == "" {
		return p, nil
	}
	if !path.IsAbs(p) {
		return "", fmt.Errorf("路径 %q 必须是绝对路径", p)
	}
	todo := strings.Split(path.Clean(p), "/")
	cur := "/"
	for links := 0; len(todo) > 0; {
		name := todo[0]
		todo = todo[1:]
		if name == "" {
			continue
		}
		next := path.Join(cur, name)
		st, err := os.Lstat(l.root + next)
		if err != nil || st.Mode()&os.ModeSymlink == 0 {
			cur = next // 不存在的部分原样保留，由后面的读写去报错
			continue
		}
		if links++; links > 40 {
			return "", fmt.Errorf("路径 %q 的符号链接层数太多", p)
		}
		target, err := os.Readlink(l.root + next)
		if err != nil {
			return "", err
		}
		if path.IsAbs(target) {
			cur = "/"
		}
		todo = append(strings.Split(path.Clean(target), "/"), todo...)
	}
	return l.root + cur, nil
}

func (l *Local) Exec(ctx context.Context, h inventory.Host, command string, out io.Writer) (int, error) {
	return l.ExecStdin(ctx, h, command, nil, out)
}

// ExecStdin 用 sh -c 执行命令。命令放在自己的进程组里，取消时整组结束。
func (l *Local) ExecStdin(ctx context.Context, h inventory.Host, command string, stdin io.Reader, out io.Writer) (int, error) {
	cmd := l.build(ctx, command)
	w := &lockedWriter{w: out}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, w, w
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() >= 0 {
		return exit.ExitCode(), nil
	}
	return -1, err
}

func (l *Local) ReadFile(ctx context.Context, h inventory.Host, name string, limit int64) ([]byte, error) {
	p, err := l.path(name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err == nil {
		if st, serr := f.Stat(); serr == nil && st.IsDir() {
			f.Close()
			return nil, fmt.Errorf("%s 是一个目录", name)
		}
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotExist
		}
		return nil, l.plain(err, name)
	}
	defer f.Close()
	var r io.Reader = f
	if limit > 0 {
		r = io.LimitReader(f, limit)
	}
	return io.ReadAll(r)
}

// WriteFile 先写同目录的临时文件再改名，沿用原文件的权限位。
func (l *Local) WriteFile(ctx context.Context, h inventory.Host, p string, data []byte) error {
	p, err := l.path(p)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(p), fmt.Sprintf(".%s.hostower-%d", filepath.Base(p), time.Now().UnixNano()))
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if st, err := os.Stat(p); err == nil {
		os.Chmod(tmp, st.Mode().Perm())
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func (l *Local) ListDir(ctx context.Context, h inventory.Host, shown string) ([]FileInfo, error) {
	dir, err := l.path(shown)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotExist
		}
		return nil, l.plain(err, shown)
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // 列目录的过程中被删掉了
		}
		fi := FileInfo{
			Name:    e.Name(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime(),
			Dir:     e.IsDir(),
			Link:    info.Mode()&os.ModeSymlink != 0,
		}
		// 指向目录的符号链接（比如 /bin → usr/bin）按目录显示，点了才进得去。
		if fi.Link {
			if target, err := l.path(path.Join(shown, e.Name())); err == nil {
				if st, err := os.Stat(target); err == nil && st.IsDir() {
					fi.Dir = true
				}
			}
		}
		out = append(out, fi)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// plain 把错误里的内部路径（宿主机模式下带着 /proc/1/root 前缀）换成用户看到的那个路径。
func (l *Local) plain(err error, shown string) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return fmt.Errorf("%s：%w", shown, pe.Err)
	}
	return err
}

func localHome() string {
	if home, err := os.UserHomeDir(); err == nil {
		if st, err := os.Stat(home); err == nil && st.IsDir() {
			return home
		}
	}
	return "/"
}

// ---------- 终端 ----------

type localSession struct {
	pty  *os.File
	fd   int
	cmd  *exec.Cmd
	once sync.Once
}

// Shell 在本机开一个带伪终端的登录 shell。
func (l *Local) Shell(ctx context.Context, h inventory.Host, cols, rows int) (ShellSession, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("打开伪终端失败：%w", err)
	}
	fd := int(master.Fd())
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err == nil {
		err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)
	}
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("打开伪终端失败：%w", err)
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, fmt.Errorf("打开伪终端失败：%w", err)
	}
	defer slave.Close()
	unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)})

	// 有 bash 用 bash，没有就用 sh；都按登录 shell 启动。
	cmd := l.build(context.Background(), `if [ -x /bin/bash ]; then exec /bin/bash -l; fi; exec /bin/sh -l`)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		master.Close()
		return nil, err
	}
	return &localSession{pty: master, fd: fd, cmd: cmd}, nil
}

func (s *localSession) Read(p []byte) (int, error) {
	n, err := s.pty.Read(p)
	// shell 退出后，读伪终端的主端会得到 EIO，这就是正常结束。
	if err != nil && (errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed)) {
		return n, io.EOF
	}
	return n, err
}

func (s *localSession) Write(p []byte) (int, error) { return s.pty.Write(p) }

func (s *localSession) Resize(cols, rows int) error {
	return unix.IoctlSetWinsize(s.fd, unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)})
}

// Close 结束整个会话（shell 和它启动的前台进程），再回收资源。
func (s *localSession) Close() error {
	s.once.Do(func() {
		syscall.Kill(-s.cmd.Process.Pid, syscall.SIGHUP)
		done := make(chan struct{})
		go func() { s.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		s.pty.Close()
	})
	return nil
}
