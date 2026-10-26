// Package notify sends best-effort owner alerts (Telegram) and
// transactional email (Resend for orders and contributions, including a
// zero amount; Brevo for bookings, consultancy connect requests, and
// newsletter confirmations).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/resend/resend-go/v4"

	"github.com/vishnubalachandran596/portfolio/core/models"
)

const (
	DisplayName = "Vishnu Balachandran"
	FromPaid    = DisplayName + " <orders@vishnubalachandran.com>"
	FromBooking = DisplayName + " <bookings@vishnubalachandran.com>"
	FromNews    = DisplayName + " <news@vishnubalachandran.com>"
	ReplyTo     = "admin@vishnubalachandran.com"

	brevoSendURL = "https://api.brevo.com/v3/smtp/email"
)

// FromFor is the From header for a kind. News stays on the main domain;
// moving promos later is a change to FromNews only.
func FromFor(kind string) string {
	switch kind {
	case models.EmailBookingNew, models.EmailBookingRescheduled, models.EmailBookingCancelled, models.EmailConnectRequest:
		return FromBooking
	case models.EmailNewsletterConfirm:
		return FromNews
	default:
		return FromPaid
	}
}

// Message is one outbound email. BCC is applied only by WithAdminBCC.
type Message struct {
	From           string
	To             string
	Subject        string
	HTML           string
	Text           string
	ReplyTo        string
	Bcc            []string
	IdempotencyKey string
}

// Mailer sends one message. Implementations must return errors and must
// not exit the process.
type Mailer interface {
	Send(ctx context.Context, m Message) (providerID string, err error)
	Name() string
}

// ErrDisabled is returned by the log-only mailer used when that
// provider's API key is empty.
var ErrDisabled = errors.New("email disabled")

// Disabled does not contact a provider. Local Docker uses it whenever
// the matching API key is empty.
type Disabled struct{}

func (Disabled) Name() string { return "disabled" }

func (Disabled) Send(context.Context, Message) (string, error) {
	return "", ErrDisabled
}

// WithAdminBCC appends the admin address unless it is already the
// recipient or already on BCC. This is the only place BCC is applied.
func WithAdminBCC(m Message, adminEmail string) Message {
	adminEmail = strings.ToLower(strings.TrimSpace(adminEmail))
	bcc := append([]string{}, m.Bcc...)
	if adminEmail != "" && !strings.EqualFold(strings.TrimSpace(m.To), adminEmail) {
		bcc = append(bcc, adminEmail)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(bcc))
	for _, addr := range bcc {
		addr = strings.TrimSpace(addr)
		key := strings.ToLower(addr)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, addr)
	}
	m.Bcc = out
	return m
}

// ResendMailer sends order and contribution mail, including a zero amount.
// The key is the client constructor
// argument, never a source literal. Method names match resend-go v4:
// Client.Emails.SendWithOptions and SendEmailOptions.IdempotencyKey.
type ResendMailer struct {
	client *resend.Client
}

func NewResend(apiKey string) *ResendMailer {
	return &ResendMailer{client: resend.NewClient(apiKey)}
}

func (m *ResendMailer) Name() string { return "resend" }

func (m *ResendMailer) Send(ctx context.Context, msg Message) (string, error) {
	if m == nil || m.client == nil {
		return "", ErrDisabled
	}
	req := &resend.SendEmailRequest{
		From:    msg.From,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Html:    msg.HTML,
		Text:    msg.Text,
		ReplyTo: msg.ReplyTo,
	}
	if len(msg.Bcc) > 0 {
		req.Bcc = append([]string{}, msg.Bcc...)
	}
	opts := &resend.SendEmailOptions{IdempotencyKey: msg.IdempotencyKey}
	sent, err := m.client.Emails.SendWithOptions(ctx, req, opts)
	if err != nil {
		return "", err
	}
	if sent == nil || sent.Id == "" {
		return "", errors.New("resend: empty response")
	}
	return sent.Id, nil
}

// BrevoMailer sends booking and newsletter confirmation mail with a
// plain POST. There is no Brevo SDK in this module.
type BrevoMailer struct {
	apiKey string
	client *http.Client
}

func NewBrevo(apiKey string) *BrevoMailer {
	return &BrevoMailer{
		apiKey: apiKey,
		client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (m *BrevoMailer) Name() string { return "brevo" }

func (m *BrevoMailer) Send(ctx context.Context, msg Message) (string, error) {
	if m == nil || m.apiKey == "" {
		return "", ErrDisabled
	}
	name, email := parseFrom(msg.From)
	body := map[string]any{
		"sender":      map[string]string{"name": name, "email": email},
		"to":          []map[string]string{{"email": msg.To}},
		"subject":     msg.Subject,
		"htmlContent": msg.HTML,
		"textContent": msg.Text,
	}
	if msg.ReplyTo != "" {
		body["replyTo"] = map[string]string{"email": msg.ReplyTo}
	}
	if len(msg.Bcc) > 0 {
		bcc := make([]map[string]string, 0, len(msg.Bcc))
		for _, addr := range msg.Bcc {
			bcc = append(bcc, map[string]string{"email": addr})
		}
		body["bcc"] = bcc
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, brevoSendURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("api-key", m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("brevo: " + strings.TrimSpace(string(payload)))
	}
	var parsed struct {
		MessageID string `json:"messageId"`
	}
	_ = json.Unmarshal(payload, &parsed)
	if parsed.MessageID == "" {
		return "accepted", nil
	}
	return parsed.MessageID, nil
}

func parseFrom(from string) (name, email string) {
	addr, err := mail.ParseAddress(from)
	if err != nil {
		return DisplayName, strings.Trim(from, "<> ")
	}
	name = addr.Name
	if name == "" {
		name = DisplayName
	}
	return name, addr.Address
}
