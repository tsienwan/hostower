package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TelegramBot 是从一个 Telegram 通知渠道借来的机器人，给聊天入口收发消息用。
// 令牌只留在这个包里：调用方拿不到它，错误信息里也不会出现它。
type TelegramBot struct {
	token string
}

// Update 是机器人收到的一条消息（只取用得到的字段）。
type Update struct {
	ID      int64 `json:"update_id"`
	Message *struct {
		Date int64  `json:"date"`
		Text string `json:"text"`
		From *struct {
			ID        int64  `json:"id"`
			IsBot     bool   `json:"is_bot"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
	} `json:"message"`
}

// TelegramBot 用指定渠道的令牌构造机器人。渠道必须是 Telegram 类型。
func (h *Hub) TelegramBot(channelID string) (*TelegramBot, error) {
	h.mu.Lock()
	var kind string
	for _, c := range h.channels {
		if c.ID == channelID {
			kind = c.Kind
		}
	}
	h.mu.Unlock()
	if kind == "" {
		return nil, errors.New("选择的 Telegram 渠道不存在")
	}
	if kind != "telegram" {
		return nil, errors.New("只有 Telegram 渠道的机器人能接收命令")
	}
	raw, err := h.secrets.Get(secretName(channelID))
	if err != nil {
		return nil, errors.New("读不到这个渠道的机器人令牌")
	}
	var sec Secret
	if json.Unmarshal(raw, &sec) != nil || sec.Token == "" {
		return nil, errors.New("这个渠道保存的令牌已损坏，请重新填写")
	}
	return &TelegramBot{token: sec.Token}, nil
}

// call 调用一个 Bot API 方法。错误信息里不带地址（地址里有令牌）。
func (b *TelegramBot) call(ctx context.Context, method string, params map[string]any, out any) error {
	data, _ := json.Marshal(params)
	req, err := http.NewRequestWithContext(ctx, "POST", telegramAPI+"/bot"+b.token+"/"+method, strings.NewReader(string(data)))
	if err != nil {
		return errors.New("请求构造失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "hostower-notify")
	client := httpClient
	if t, ok := params["timeout"].(int); ok && t > 0 {
		// 长轮询：等待时间要比普通请求的超时长。
		client = &http.Client{Timeout: time.Duration(t+15) * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			return fmt.Errorf("连不上 Telegram：%v", ue.Err)
		}
		return errors.New("连不上 Telegram")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var r struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	json.Unmarshal(body, &r)
	switch {
	case resp.StatusCode == http.StatusConflict:
		return errors.New("这个机器人正被别的程序接收消息（或设置了 Webhook）：请给 Hostower 单独建一个机器人")
	case resp.StatusCode == http.StatusUnauthorized:
		return errors.New("机器人令牌无效")
	case !r.OK:
		if r.Description == "" {
			r.Description = "HTTP " + strconv.Itoa(resp.StatusCode)
		}
		return fmt.Errorf("Telegram 拒绝了：%s", clip(r.Description, 200))
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// Updates 长轮询新消息。offset 为 -1 时只取最后一条（用来跳过积压的消息）。
func (b *TelegramBot) Updates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	var out []Update
	err := b.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": timeout, "allowed_updates": []string{"message"}}, &out)
	return out, err
}

// Send 发一条纯文本消息：不解析任何格式，不展开链接预览。
func (b *TelegramBot) Send(ctx context.Context, chatID int64, text string) error {
	return b.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": clip4096(text), "disable_web_page_preview": true}, nil)
}

// Me 返回机器人的用户名（不带 @）。
func (b *TelegramBot) Me(ctx context.Context) (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	err := b.call(ctx, "getMe", map[string]any{}, &me)
	return me.Username, err
}

// BotCommand 是机器人菜单里的一项。
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// SetCommands 设置机器人的命令菜单（只在私聊里显示）。
func (b *TelegramBot) SetCommands(ctx context.Context, cmds []BotCommand, lang string) error {
	p := map[string]any{"commands": cmds, "scope": map[string]string{"type": "all_private_chats"}}
	if lang != "" {
		p["language_code"] = lang
	}
	return b.call(ctx, "setMyCommands", p, nil)
}
