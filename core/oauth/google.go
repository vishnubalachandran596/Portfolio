// Package oauth implements just enough of Google's OAuth2 / OpenID
// Connect "Authorization Code" flow to log a visitor in with their
// Google account — no third-party OAuth library, since the whole flow is
// two plain HTTP calls once you have the authorization code.
// See https://developers.google.com/identity/protocols/oauth2/web-server
package oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Google struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	client       *http.Client
}

func NewGoogle(clientID, clientSecret, redirectURL string) *Google {
	return &Google{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		client:       &http.Client{Timeout: 8 * time.Second},
	}
}

func (g *Google) Enabled() bool {
	return g.ClientID != "" && g.ClientSecret != "" && g.RedirectURL != ""
}

// AuthURL builds the URL that sends the visitor to Google's own consent
// screen. state is an opaque, unguessable value the caller must also
// stash (e.g. in a short-lived cookie) and compare against the value
// Google echoes back on the callback, to prevent CSRF.
func (g *Google) AuthURL(state string) string {
	v := url.Values{
		"client_id":     {g.ClientID},
		"redirect_uri":  {g.RedirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"prompt":        {"select_account"},
	}
	return "https://accounts.google.com/o/oauth2/v2/auth?" + v.Encode()
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// Exchange trades the authorization code Google's redirect handed back
// for an access token.
func (g *Google) Exchange(code string) (string, error) {
	resp, err := g.client.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"redirect_uri":  {g.RedirectURL},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return "", fmt.Errorf("google token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("google token exchange: invalid response: %w", err)
	}
	if parsed.Error != "" {
		return "", fmt.Errorf("google token exchange: %s: %s", parsed.Error, parsed.ErrorDesc)
	}
	if parsed.AccessToken == "" {
		return "", fmt.Errorf("google token exchange: empty access token")
	}
	return parsed.AccessToken, nil
}

type UserInfo struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// FetchUserInfo asks Google who the access token belongs to.
func (g *Google) FetchUserInfo(accessToken string) (UserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return UserInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.client.Do(req)
	if err != nil {
		return UserInfo{}, fmt.Errorf("google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UserInfo{}, fmt.Errorf("google userinfo: status %d", resp.StatusCode)
	}
	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return UserInfo{}, fmt.Errorf("google userinfo: invalid response: %w", err)
	}
	if info.Email == "" {
		return UserInfo{}, fmt.Errorf("google userinfo: no email returned")
	}
	return info, nil
}
