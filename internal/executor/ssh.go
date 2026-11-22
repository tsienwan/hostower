package executor

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/tsienwan/hostower/internal/inventory"
	"github.com/tsienwan/hostower/internal/secrets"
)

// SSHCredential 是 ssh/* 凭据的内容。
type SSHCredential struct {
	User       string `json:"user"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase,omitempty"`
}

// SSH 是内置的 SSH 执行器：塔台 → 主机，被管机只需要 sshd。
// 每台主机复用一条连接，空闲一段时间后关闭。
type SSH struct {
	secrets *secrets.Store
	db      *sql.DB

	mu    sync.Mutex
	conns map[string]*sshConn
}

type sshConn struct {
	client *ssh.Client
	last   time.Time
	// busy 是这条连接上正在跑的会话数：持续采样的会话会一直开着，这时连接不算空闲。
	busy int
}

const sshIdle = 2 * time.Minute

// NewSSH 创建 SSH 执行器。主机公钥按“首次信任、之后固定”保存在数据库里。
func NewSSH(sec *secrets.Store, db *sql.DB) *SSH {
	s := &SSH{secrets: sec, db: db, conns: map[string]*sshConn{}}
	go s.reap()
	return s
}

func (s *SSH) reap() {
	for range time.Tick(30 * time.Second) {
		s.mu.Lock()
		for name, c := range s.conns {
			if c.busy == 0 && time.Since(c.last) > sshIdle {
				c.client.Close()
				delete(s.conns, name)
			}
		}
		s.mu.Unlock()
	}
}

func (s *SSH) hostKeyCallback(hostName string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := base64.StdEncoding.EncodeToString(key.Marshal())
		var want string
		err := s.db.QueryRow("SELECT key FROM known_hosts WHERE host = ?", hostName).Scan(&want)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = s.db.Exec("INSERT INTO known_hosts (host, key, added_at) VALUES (?,?,?)",
				hostName, got, time.Now().UTC().Format(time.RFC3339))
			return err
		}
		if err != nil {
			return err
		}
		if want != got {
			return fmt.Errorf("主机 %s 的公钥与首次连接时不一致（%s）：可能是中间人，也可能是重装了系统。确认后用 hostower hostkey forget %s 清除旧记录",
				hostName, ssh.FingerprintSHA256(key), hostName)
		}
		return nil
	}
}

func (s *SSH) dial(ctx context.Context, h inventory.Host) (*ssh.Client, error) {
	name, err := secrets.RefName(h.Credential)
	if err != nil {
		return nil, fmt.Errorf("主机 %s：%w", h.Name, err)
	}
	raw, err := s.secrets.Get(name)
	if err != nil {
		return nil, err
	}
	var cred SSHCredential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return nil, fmt.Errorf("凭据 %s 的格式不是 SSH 凭据", name)
	}
	var signer ssh.Signer
	if cred.Passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cred.PrivateKey), []byte(cred.Passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(cred.PrivateKey))
	}
	if err != nil {
		return nil, fmt.Errorf("凭据 %s 里的私钥无法解析", name)
	}
	addr := h.Addr
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "22")
	}
	cfg := &ssh.ClientConfig{
		User:            cred.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: s.hostKeyCallback(h.Name),
		Timeout:         10 * time.Second,
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	// ClientConfig.Timeout 只管 ssh.Dial；这里是自己拨的连接，握手要自己设期限，也要跟着 ctx 取消：
	// 否则遇到接了 TCP 却一直不完成握手的主机（sshd 过载、MaxStartups 满了），请求会一直卡在“执行中”。
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	stop()
	if err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

const (
	handshakeTimeout  = 15 * time.Second
	keepaliveInterval = 30 * time.Second
	keepaliveTimeout  = 15 * time.Second
)

// keepalive 定时向服务端发 keepalive 请求；网络悄悄断了、对面没有应答时把这条连接从缓存里拿掉，
// 下一个会话会重新连，而不是在一条死连接上等到 TCP 超时（可能十几分钟）。连接关闭后自己退出。
func (s *SSH) keepalive(h inventory.Host, client *ssh.Client) {
	done := make(chan struct{})
	go func() { client.Wait(); close(done) }()
	t := time.NewTicker(keepaliveInterval)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		res := make(chan error, 1)
		go func() { _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); res <- err }()
		select {
		case <-done:
			return
		case err := <-res:
			if err == nil {
				continue
			}
		case <-time.After(keepaliveTimeout):
		}
		s.drop(h, client)
		return
	}
}

func (s *SSH) client(ctx context.Context, h inventory.Host) (*ssh.Client, error) {
	s.mu.Lock()
	if c, ok := s.conns[h.Name]; ok {
		c.last = time.Now()
		s.mu.Unlock()
		return c.client, nil
	}
	s.mu.Unlock()
	client, err := s.dial(ctx, h)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.conns[h.Name]; ok {
		client.Close()
		return c.client, nil
	}
	s.conns[h.Name] = &sshConn{client: client, last: time.Now()}
	go s.keepalive(h, client)
	return client, nil
}

// Reconnect 关掉这台主机的连接，下一个会话会重新登录。
// 登录账号的组变了（比如刚加进 docker 组）时要这样做：已经登录的连接拿不到新的组。
// 这条连接上正在跑的会话会断开。
func (s *SSH) Reconnect(host string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.conns[host]; ok {
		c.client.Close()
		delete(s.conns, host)
	}
}

func (s *SSH) drop(h inventory.Host, client *ssh.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.conns[h.Name]; ok && c.client == client {
		c.client.Close()
		delete(s.conns, h.Name)
	}
}

// session 取一个新会话；复用的连接已经断开时重连一次。
func (s *SSH) session(ctx context.Context, h inventory.Host) (*ssh.Session, *ssh.Client, error) {
	for attempt := 0; ; attempt++ {
		client, err := s.client(ctx, h)
		if err != nil {
			return nil, nil, err
		}
		sess, err := client.NewSession()
		if err == nil {
			return sess, client, nil
		}
		s.drop(h, client)
		if attempt > 0 {
			return nil, nil, err
		}
	}
}

func (s *SSH) Exec(ctx context.Context, h inventory.Host, command string, out io.Writer) (int, error) {
	return s.ExecStdin(ctx, h, command, nil, out)
}

// ExecStdin 执行命令，并把 stdin 的内容流式送进它的标准输入。
func (s *SSH) ExecStdin(ctx context.Context, h inventory.Host, command string, stdin io.Reader, out io.Writer) (int, error) {
	sess, client, err := s.session(ctx, h)
	if err != nil {
		return -1, err
	}
	defer sess.Close()
	s.hold(h, client, 1)
	defer s.hold(h, client, -1)
	if stdin != nil {
		sess.Stdin = stdin
	}
	w := &lockedWriter{w: out}
	sess.Stdout, sess.Stderr = w, w
	if err := sess.Start(command); err != nil {
		return -1, err
	}
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	select {
	case <-ctx.Done():
		sess.Signal(ssh.SIGKILL)
		sess.Close()
		return -1, ctx.Err()
	case err := <-done:
		if err == nil {
			return 0, nil
		}
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			return exit.ExitStatus(), nil
		}
		return -1, err
	}
}

// hold 记下（或撤销）“这台主机的连接上有一个会话在跑”。只算在会话所在的那条连接上（重连之后旧连接已经不在表里了）。
func (s *SSH) hold(h inventory.Host, client *ssh.Client, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.conns[h.Name]; ok && c.client == client {
		c.busy += n
		c.last = time.Now()
	}
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// sftp 开一个 SFTP 会话。返回的 done 用完要调用：它关掉会话；ctx 取消（超时、浏览器断开）时也会提前关掉，
// 正在读写的操作随之返回，不会卡在一个慢文件（NFS、管道）上。
func (s *SSH) sftp(ctx context.Context, h inventory.Host) (*sftp.Client, func(), error) {
	client, err := s.client(ctx, h)
	if err != nil {
		return nil, nil, err
	}
	fc, err := sftp.NewClient(client)
	if err != nil {
		s.drop(h, client)
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { fc.Close() })
	return fc, func() { stop(); fc.Close() }, nil
}

func (s *SSH) ReadFile(ctx context.Context, h inventory.Host, p string, limit int64) ([]byte, error) {
	fc, done, err := s.sftp(ctx, h)
	if err != nil {
		return nil, err
	}
	defer done()
	f, err := fc.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotExist
		}
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if limit > 0 {
		r = io.LimitReader(f, limit)
	}
	return io.ReadAll(r)
}

func (s *SSH) WriteFile(ctx context.Context, h inventory.Host, p string, data []byte) error {
	fc, done, err := s.sftp(ctx, h)
	if err != nil {
		return err
	}
	defer done()
	tmp := path.Join(path.Dir(p), fmt.Sprintf(".%s.hostower-%d", path.Base(p), time.Now().UnixNano()))
	f, err := fc.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, bytes.NewReader(data)); err != nil {
		f.Close()
		fc.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		fc.Remove(tmp)
		return err
	}
	if st, err := fc.Stat(p); err == nil {
		fc.Chmod(tmp, st.Mode().Perm())
	}
	if err := fc.PosixRename(tmp, p); err != nil {
		fc.Remove(tmp)
		return err
	}
	return nil
}

func (s *SSH) ListDir(ctx context.Context, h inventory.Host, dir string) ([]FileInfo, error) {
	fc, done, err := s.sftp(ctx, h)
	if err != nil {
		return nil, err
	}
	defer done()
	entries, err := fc.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotExist
		}
		return nil, err
	}
	out := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		fi := FileInfo{
			Name:    e.Name(),
			Size:    e.Size(),
			Mode:    e.Mode().String(),
			ModTime: e.ModTime(),
			Dir:     e.IsDir(),
			Link:    e.Mode()&os.ModeSymlink != 0,
		}
		// 指向目录的符号链接按目录显示，点了才进得去。
		if fi.Link {
			if st, err := fc.Stat(path.Join(dir, e.Name())); err == nil && st.IsDir() {
				fi.Dir = true
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

// ForgetHostKey 清除一台主机固定的公钥（重装系统后使用）。
func ForgetHostKey(db *sql.DB, host string) error {
	_, err := db.Exec("DELETE FROM known_hosts WHERE host = ?", host)
	return err
}
