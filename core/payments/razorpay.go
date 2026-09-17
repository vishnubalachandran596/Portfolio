// Package payments wraps the Razorpay Orders API and the HMAC signature
// verification Razorpay's documented checkout flow requires — see
// https://razorpay.com/docs/payments/payment-gateway/web-integration/standard/build-integration/
package payments

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Razorpay struct {
	KeyID     string
	KeySecret string
	client    *http.Client
}

func NewRazorpay(keyID, keySecret string) *Razorpay {
	return &Razorpay{
		KeyID:     keyID,
		KeySecret: keySecret,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether Razorpay credentials are configured. Checkout
// stays disabled (and paid courses show as "coming soon") until they are.
func (r *Razorpay) Enabled() bool { return r.KeyID != "" && r.KeySecret != "" }

type orderRequest struct {
	Amount   int    `json:"amount"` // smallest currency unit (cents/paise)
	Currency string `json:"currency"`
	Receipt  string `json:"receipt"`
}

type orderResponse struct {
	ID    string `json:"id"`
	Error *struct {
		Description string `json:"description"`
	} `json:"error"`
}

// CreateOrder creates a Razorpay order for the given amount and returns
// its order ID, which the client-side Checkout.js widget needs to open
// the payment modal.
func (r *Razorpay) CreateOrder(amountCents int, currency, receipt string) (string, error) {
	body, err := json.Marshal(orderRequest{Amount: amountCents, Currency: currency, Receipt: receipt})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.razorpay.com/v1/orders", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(r.KeyID, r.KeySecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("razorpay create order: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var parsed orderResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("razorpay create order: invalid response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || parsed.Error != nil {
		if parsed.Error != nil {
			return "", fmt.Errorf("razorpay create order: %s", parsed.Error.Description)
		}
		return "", fmt.Errorf("razorpay create order: status %d", resp.StatusCode)
	}
	return parsed.ID, nil
}

// VerifySignature checks the HMAC-SHA256 signature Razorpay's Checkout.js
// returns after a successful payment, per Razorpay's documented formula:
// hex(HMAC-SHA256(key_secret, order_id + "|" + payment_id)) == signature.
// This is the step that actually proves the payment happened — never grant
// access based on a client-side "success" callback alone.
func (r *Razorpay) VerifySignature(orderID, paymentID, signature string) bool {
	mac := hmac.New(sha256.New, []byte(r.KeySecret))
	mac.Write([]byte(orderID + "|" + paymentID))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
