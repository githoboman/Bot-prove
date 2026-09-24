package hitl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TelegramSink delivers an escalation to a Telegram chat via the Bot API
// sendMessage method.
//
// The sink assumes a bot has been created via BotFather and added to the
// target chat, and that the caller knows the numeric ChatID (obtainable from
// getUpdates or @userinfobot). Message text is Markdown-formatted via
// Telegram's "MarkdownV2" parse mode; special characters are escaped
// defensively so the message never fails to render.
type TelegramSink struct {
	// BotToken is the token from BotFather, e.g. "123456:ABC-DEF...". Required.
	BotToken string
	// ChatID is the numeric identifier of the target chat. Required.
	// Negative values are legal (they identify group chats).
	ChatID int64
	// APIBase overrides the Telegram Bot API base URL. Defaults to
	// "https://api.telegram.org". Tests set this to a local httptest server.
	APIBase string
	// HTTPClient is used for delivery. If nil, a sensible default with a
	// short timeout is created.
	HTTPClient *http.Client
}

// NewTelegramSink returns a TelegramSink with a preconfigured HTTP client. It
// validates that token and chatID are set.
func NewTelegramSink(botToken string, chatID int64) (*TelegramSink, error) {
	if botToken == "" {
		return nil, errors.New("hitl: telegram bot token is empty")
	}
	if chatID == 0 {
		return nil, errors.New("hitl: telegram chat id is zero")
	}
	return &TelegramSink{
		BotToken:   botToken,
		ChatID:     chatID,
		APIBase:    "https://api.telegram.org",
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// telegramSendMessageRequest is the subset of Bot API sendMessage we use.
type telegramSendMessageRequest struct {
	ChatID              int64  `json:"chat_id"`
	Text                string `json:"text"`
	ParseMode           string `json:"parse_mode,omitempty"`
	DisableNotification bool   `json:"disable_notification,omitempty"`
}

// telegramResponse is the standard Bot API envelope.
type telegramResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
}

// Deliver posts a compact summary of the event to Telegram.
func (t *TelegramSink) Deliver(ctx context.Context, ev EscalationEvent) error {
	if t.BotToken == "" || t.ChatID == 0 {
		return errors.New("hitl: telegram sink not configured")
	}
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	payload := telegramSendMessageRequest{
		ChatID:    t.ChatID,
		Text:      formatTelegramText(ev),
		ParseMode: "MarkdownV2",
	}
	// low severity does not ring the phone
	if ev.Severity == SeverityLow {
		payload.DisableNotification = true
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("hitl: telegram marshal: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", base, t.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("hitl: telegram build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("hitl: telegram post: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hitl: telegram status %d: %s", resp.StatusCode, string(raw))
	}
	var out telegramResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("hitl: telegram parse response: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("hitl: telegram api error %d: %s", out.ErrorCode, out.Description)
	}
	return nil
}

// escapeMarkdownV2 escapes the reserved characters described in the Telegram
// Bot API MarkdownV2 spec so arbitrary text can be safely embedded.
// See https://core.telegram.org/bots/api#markdownv2-style
func escapeMarkdownV2(s string) string {
	// Reserved: _ * [ ] ( ) ~ ` > # + - = | { } . !
	const special = "_*[]()~`>#+-=|{}.!"
	var buf bytes.Buffer
	for _, r := range s {
		if bytes.ContainsRune([]byte(special), r) {
			buf.WriteByte('\\')
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

// formatTelegramText produces a compact MarkdownV2 summary of the event.
func formatTelegramText(ev EscalationEvent) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*HITL escalation* — severity `%s`\n", escapeMarkdownV2(string(ev.Severity)))
	fmt.Fprintf(&buf, "*task*: `%s`  *overall*: `%s`\n",
		escapeMarkdownV2(ev.TaskID), escapeMarkdownV2(string(ev.Overall)))
	fmt.Fprintf(&buf, "*reason*: %s\n", escapeMarkdownV2(ev.Reason))
	fmt.Fprintf(&buf, "*digest*: `%s`\n", escapeMarkdownV2(ev.Digest))
	if len(ev.Facets) > 0 {
		buf.WriteString("*facets*:\n")
		for _, f := range ev.Facets {
			fmt.Fprintf(&buf, "• `%s` → `%s`",
				escapeMarkdownV2(f.FacetID), escapeMarkdownV2(string(f.Verdict)))
			if f.Winner != "" {
				fmt.Fprintf(&buf, " \\(winner=`%s`, agree=%.2f, live=%d\\)",
					escapeMarkdownV2(f.Winner), f.AgreementFraction, f.LiveCount)
			}
			buf.WriteString("\n")
		}
	}
	// Backslashes for percent-formatted floats (agree=0.50 contains a dot).
	// escapeMarkdownV2 escapes strings, so we escape dots in the final buffer
	// only when they came from Sprintf floats:
	return replaceUnescapedDots(buf.String())
}

// replaceUnescapedDots walks the string and escapes any '.' that is not
// already preceded by a backslash. MarkdownV2 requires every '.' to be
// escaped; Sprintf-produced floats emit unescaped ones.
func replaceUnescapedDots(s string) string {
	var buf bytes.Buffer
	buf.Grow(len(s) + 8)
	var prev rune
	for _, r := range s {
		if r == '.' && prev != '\\' {
			buf.WriteByte('\\')
		}
		buf.WriteRune(r)
		prev = r
	}
	return buf.String()
}
