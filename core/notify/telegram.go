// Package notify sends a best-effort Telegram message to the site owner
// for events worth knowing about immediately (a paid booking or course
// purchase) — Telegram's Bot API is free and needs no business
// verification, unlike WhatsApp's Cloud API.
package notify

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

type Telegram struct {
	BotToken string
	ChatID   string
	client   *http.Client
}

func NewTelegram(botToken, chatID string) *Telegram {
	return &Telegram{
		BotToken: botToken,
		ChatID:   chatID,
		client:   &http.Client{Timeout: 6 * time.Second},
	}
}

func (t *Telegram) Enabled() bool { return t.BotToken != "" && t.ChatID != "" }

// Send posts a plain-text message to the configured chat. Best-effort:
// failures are logged, never returned to the caller — a Telegram outage
// should never fail a booking or a purchase.
func (t *Telegram) Send(message string) {
	if !t.Enabled() {
		return
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	resp, err := t.client.PostForm(apiURL, url.Values{
		"chat_id": {t.ChatID},
		"text":    {message},
	})
	if err != nil {
		log.Printf("telegram notify: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("telegram notify: status %d", resp.StatusCode)
	}
}
