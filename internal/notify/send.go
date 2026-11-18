package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tsienwan/hostower/internal/i18n"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 四种渠道的发送。所有渠道都只发纯文本（或者把文本放进对方要求的结构里），不发 HTML，
// 也不让对方解析其中的提及（@everyone 之类）。地址只能是各家官方的接口地址，不能随便填一个网址，
// 免得通知功能被拿来让 Hostower 去访问内网里的东西。

// 这几个地址在测试里会被换成本地的假服务。
var (
	telegramAPI  = "https://api.telegram.org"
	httpClient   = &http.Client{Timeout: 15 * time.Second}
	allowTestURL func(string) bool
)

var (
	telegramTokenRe = regexp.MustCompile(`^[0-9]{5,16}:[A-Za-z0-9_-]{30,64}$`)
	telegramChatRe  = regexp.MustCompile(`^(-?[0-9]{1,20}|@[A-Za-z][A-Za-z0-9_]{4,31})$`)
	feishuHookRe    = regexp.MustCompile(`^https://open\.(feishu\.cn|larksuite\.com)/open-apis/bot/v2/hook/[A-Za-z0-9-]{8,64}$`)
	discordHookRe   = regexp.MustCompile(`^https://(discord\.com|discordapp\.com|ptb\.discord\.com|canary\.discord\.com)/api/webhooks/[0-9]{5,25}/[A-Za-z0-9_-]{20,120}$`)
	emailRe         = regexp.MustCompile(`^[^@\s<>,;"]{1,64}@[A-Za-z0-9.-]{1,253}\.[A-Za-z]{2,24}$`)
	smtpHostRe      = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}$`)
)

func okURL(u string, re *regexp.Regexp) bool {
	return re.MatchString(u) || (allowTestURL != nil && allowTestURL(u))
}

// colors 是 Discord 卡片左边那条竖线的颜色。
var colors = map[Severity]int{Info: 0x0a84ff, Warn: 0xff9f0a, Crit: 0xff453a, OK: 0x30d158}

var icons = map[Severity]string{Info: "🔔", Warn: "⚠️", Crit: "🚨", OK: "✅"}

func postJSON(ctx context.Context, url string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hostower-notify")
	resp, err := httpClient.Do(req)
	if err != nil {
		// 错误信息里可能带着完整地址（里面有令牌），只留下原因。
		var ue interface{ Unwrap() error }
		if errors.As(err, &ue) && ue.Unwrap() != nil {
			return nil, fmt.Errorf("连接失败：%v", ue.Unwrap())
		}
		return nil, errors.New("连接失败")
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode/100 != 2 {
		return out, fmt.Errorf("对方返回 %d：%s", resp.StatusCode, clip(string(out), 200))
	}
	return out, nil
}

func sendTelegram(ctx context.Context, sec Secret, ch *Channel, m Message, base string) error {
	title, body := m.Text(ch.Lang, base)
	text := icons[m.Severity] + " " + title
	if body != "" {
		text += "\n\n" + body
	}
	out, err := postJSON(ctx, telegramAPI+"/bot"+sec.Token+"/sendMessage", map[string]any{
		"chat_id": ch.Telegram.ChatID, "text": clip4096(text), "disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}
	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if json.Unmarshal(out, &r) == nil && !r.OK {
		return fmt.Errorf("Telegram 拒绝了：%s", r.Description)
	}
	return nil
}

func clip4096(s string) string {
	if r := []rune(s); len(r) > 4000 {
		return string(r[:4000]) + "…"
	}
	return s
}

func sendFeishu(ctx context.Context, sec Secret, ch *Channel, m Message, base string) error {
	title, _ := m.Text(ch.Lang, base)
	lines := [][]map[string]string{}
	for _, fl := range m.Fields {
		if fl.Value != "" {
			lines = append(lines, []map[string]string{{"tag": "text", "text": fl.Name.In(ch.Lang) + i18n.Get(ch.Lang).Colon + fl.Value}})
		}
	}
	if u := m.URL(base); u != "" {
		label := i18n.T(ch.Lang, "打开 Hostower 查看")
		lines = append(lines, []map[string]string{{"tag": "a", "text": label, "href": u}})
	}
	// 飞书的富文本只认这几种语言键；内容本身已经是渠道的语言。
	lang := i18n.Get(ch.Lang).Feishu
	payload := map[string]any{
		"msg_type": "post",
		"content":  map[string]any{"post": map[string]any{lang: map[string]any{"title": icons[m.Severity] + " " + title, "content": lines}}},
	}
	// 签名校验（机器人设置里打开“签名校验”时才需要）。
	if sec.Sign != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(ts+"\n"+sec.Sign))
		payload["timestamp"] = ts
		payload["sign"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	out, err := postJSON(ctx, sec.Webhook, payload)
	if err != nil {
		return err
	}
	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if json.Unmarshal(out, &r) == nil && r.Code != 0 {
		return fmt.Errorf("飞书拒绝了：%s（%d）", r.Msg, r.Code)
	}
	return nil
}

func sendDiscord(ctx context.Context, sec Secret, ch *Channel, m Message, base string) error {
	title, _ := m.Text(ch.Lang, base)
	var fields []map[string]any
	for _, fl := range m.Fields {
		if fl.Value != "" {
			fields = append(fields, map[string]any{"name": fl.Name.In(ch.Lang), "value": clip(fl.Value, 1000), "inline": len(fl.Value) < 40})
		}
	}
	embed := map[string]any{"title": clip(title, 250), "color": colors[m.Severity], "fields": fields, "timestamp": m.Time.UTC().Format(time.RFC3339)}
	if u := m.URL(base); u != "" {
		embed["url"] = u
	}
	_, err := postJSON(ctx, sec.Webhook, map[string]any{
		"username": "Hostower", "embeds": []any{embed},
		// 不让通知内容里的 @everyone、@某人 真的去提醒人。
		"allowed_mentions": map[string]any{"parse": []string{}},
	})
	return err
}

func sendEmail(ctx context.Context, sec Secret, ch *Channel, m Message, base string) error {
	e := ch.Email
	title, body := m.Text(ch.Lang, base)
	addr := net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if e.Security == "tls" {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: e.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("连不上邮件服务器：%v", err)
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, e.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("邮件服务器握手失败：%v", err)
	}
	defer c.Close()
	if e.Security == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: e.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("STARTTLS 失败：%v", err)
		}
	}
	if e.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", e.Username, sec.Password, e.Host)); err != nil {
			return fmt.Errorf("邮件服务器登录失败：%v", err)
		}
	}
	if err := c.Mail(e.From); err != nil {
		return fmt.Errorf("发件人被拒绝：%v", err)
	}
	for _, to := range e.To {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("收件人 %s 被拒绝：%v", to, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	var msg strings.Builder
	msg.WriteString("From: Hostower <" + e.From + ">\r\n")
	msg.WriteString("To: " + strings.Join(e.To, ", ") + "\r\n")
	msg.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", "[Hostower] "+title) + "\r\n")
	msg.WriteString("Date: " + m.Time.Format(time.RFC1123Z) + "\r\n")
	msg.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(title + "\n\n" + body + "\n"))
	for len(enc) > 76 {
		msg.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	msg.WriteString(enc + "\r\n")
	if _, err := io.WriteString(w, msg.String()); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("邮件服务器没有接收：%v", err)
	}
	return c.Quit()
}
