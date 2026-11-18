package executor

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/tsienwan/hostower/internal/inventory"
)

// ShellSession 是一个交互终端会话。
type ShellSession interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// ShellExecutor 是能开交互终端的执行器。
// 终端里的命令没法逐条判级，所以“开终端”这个动作本身由内核按高危处理并整段录制。
type ShellExecutor interface {
	Shell(ctx context.Context, h inventory.Host, cols, rows int) (ShellSession, error)
}

// ---------- SSH ----------

type sshShell struct {
	sess *ssh.Session
	in   io.WriteCloser
	out  io.Reader
	once sync.Once
}

// Shell 在主机上开一个带伪终端的登录 shell。
func (s *SSH) Shell(ctx context.Context, h inventory.Host, cols, rows int) (ShellSession, error) {
	sess, _, err := s.session(ctx, h)
	if err != nil {
		return nil, err
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sess.Close()
		return nil, err
	}
	in, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		sess.Close()
		return nil, err
	}
	return &sshShell{sess: sess, in: in, out: out}, nil
}

func (s *sshShell) Read(p []byte) (int, error)  { return s.out.Read(p) }
func (s *sshShell) Write(p []byte) (int, error) { return s.in.Write(p) }
func (s *sshShell) Resize(cols, rows int) error { return s.sess.WindowChange(rows, cols) }
func (s *sshShell) Close() error {
	var err error
	s.once.Do(func() { s.in.Close(); err = s.sess.Close() })
	return err
}

// ---------- 模拟主机 ----------

// simShell 是一个假的 shell：有提示符、回显和退格，命令的输出来自和“执行”相同的模拟数据。
type simShell struct {
	sim  *Sim
	h    inventory.Host
	pr   *io.PipeReader
	pw   *io.PipeWriter
	mu   sync.Mutex
	line []byte
	out  chan []byte
	done chan struct{}
	once sync.Once
}

// Shell 开一个模拟终端。
func (s *Sim) Shell(ctx context.Context, h inventory.Host, cols, rows int) (ShellSession, error) {
	if err := s.offline(h); err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	sh := &simShell{sim: s, h: h, pr: pr, pw: pw, out: make(chan []byte, 64), done: make(chan struct{})}
	// 输出走一个带缓冲的队列，写入方（处理键盘输入的那一侧）不会被读取方卡住。
	go func() {
		for {
			select {
			case b := <-sh.out:
				if _, err := pw.Write(b); err != nil {
					return
				}
			case <-sh.done:
				return
			}
		}
	}()
	sh.print("这是模拟主机 " + h.Name + " 的终端：命令不会真的执行。输入 exit 结束。\r\n" + sh.prompt())
	return sh, nil
}

func (s *simShell) prompt() string { return "\x1b[32mops@" + s.h.Name + "\x1b[0m:\x1b[34m~\x1b[0m$ " }

func (s *simShell) print(text string) {
	select {
	case s.out <- []byte(text):
	case <-s.done:
	}
}

func (s *simShell) Read(p []byte) (int, error) { return s.pr.Read(p) }

func (s *simShell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range p {
		switch {
		case b == '\r' || b == '\n':
			cmd := strings.TrimSpace(string(s.line))
			s.line = s.line[:0]
			s.print("\r\n")
			if cmd == "exit" || cmd == "logout" {
				s.print("logout\r\n")
				go s.Close()
				return len(p), nil
			}
			if cmd != "" {
				s.sim.mu.Lock()
				lines, _ := s.sim.canned(s.h, s.sim.host(s.h), cmd)
				s.sim.mu.Unlock()
				if len(lines) > 0 {
					s.print(strings.Join(lines, "\r\n") + "\r\n")
				}
			}
			s.print(s.prompt())
		case b == 0x7f || b == 0x08:
			if len(s.line) > 0 {
				s.line = s.line[:len(s.line)-1]
				s.print("\b \b")
			}
		case b == 0x03:
			s.line = s.line[:0]
			s.print("^C\r\n" + s.prompt())
		case b == 0x04:
			if len(s.line) == 0 {
				s.print("logout\r\n")
				go s.Close()
				return len(p), nil
			}
		case b >= 0x20:
			s.line = append(s.line, b)
			s.print(string([]byte{b}))
		}
	}
	return len(p), nil
}

func (s *simShell) Resize(cols, rows int) error { return nil }

func (s *simShell) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.pw.CloseWithError(io.EOF)
	})
	return nil
}

// ErrNoShell 表示执行器不支持交互终端。
var ErrNoShell = errors.New("这种连接方式不支持交互终端")
