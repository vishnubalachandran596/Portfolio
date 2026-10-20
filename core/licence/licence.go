// Package licence signs and verifies Temple Accounts licence keys.
//
// The website holds the Ed25519 PRIVATE key (an environment secret) and
// signs keys; the desktop app ships only the matching PUBLIC key and
// verifies offline. Format (must match the app exactly):
//
//	payload   = compact JSON, keys sorted, UTF-8:
//	            {"dev","iat","kid","lic","prod","v":1,"ver"}
//	signature = Ed25519(b"vb-licence/v1\n" + payload)
//	key       = "VBL1-" + b64url(payload) + "." + b64url(signature)   (no padding)
package licence

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// SigningPrefix is prepended to the payload before signing. Never change it.
	SigningPrefix = "vb-licence/v1\n"
	KeyPrefix     = "VBL1-"
	payloadV      = 1
)

var b64 = base64.RawURLEncoding

// Signer signs licence keys with one Ed25519 private key.
type Signer struct {
	priv ed25519.PrivateKey
	kid  string
}

// NewSigner builds a Signer from a base64 (standard or URL-safe, padded or
// not) 32-byte seed or 64-byte private key, and a key id.
func NewSigner(privB64, kid string) (*Signer, error) {
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return nil, errors.New("licence: key id is empty")
	}
	raw, err := decodeB64(privB64)
	if err != nil {
		return nil, errors.New("licence: signing key is not valid base64")
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return &Signer{priv: ed25519.NewKeyFromSeed(raw), kid: kid}, nil
	case ed25519.PrivateKeySize:
		return &Signer{priv: ed25519.PrivateKey(raw), kid: kid}, nil
	}
	return nil, fmt.Errorf("licence: signing key must be 32 or 64 bytes, got %d", len(raw))
}

// KID is the key id written into every payload.
func (s *Signer) KID() string { return s.kid }

// PublicKeyB64 is the base64 of the raw 32-byte public key -- the value the
// app owner puts in config/licence_keys.json. It is not secret.
func (s *Signer) PublicKeyB64() string {
	return base64.StdEncoding.EncodeToString(s.priv.Public().(ed25519.PublicKey))
}

// Issue builds and signs a key for one licence, device and app major version.
func (s *Signer) Issue(product, lic, device, ver string, iat time.Time) (string, error) {
	dev, err := NormaliseDevice(device)
	if err != nil {
		return "", err
	}
	payload, err := Payload(dev, iat, s.kid, lic, product, ver)
	if err != nil {
		return "", err
	}
	return s.SignPayload(payload), nil
}

// SignPayload signs exact payload bytes and returns the key string.
func (s *Signer) SignPayload(payload []byte) string {
	msg := append([]byte(SigningPrefix), payload...)
	sig := ed25519.Sign(s.priv, msg)
	return KeyPrefix + b64.EncodeToString(payload) + "." + b64.EncodeToString(sig)
}

// Payload serialises the payload: compact, keys sorted, no HTML escaping.
func Payload(dev string, iat time.Time, kid, lic, product, ver string) ([]byte, error) {
	if lic == "" || product == "" || ver == "" || kid == "" {
		return nil, errors.New("licence: payload field missing")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	// json.Marshal sorts map keys, which gives the required order.
	err := enc.Encode(map[string]any{
		"dev":  dev,
		"iat":  iat.UTC().Format("2006-01-02T15:04:05Z"),
		"kid":  kid,
		"lic":  lic,
		"prod": product,
		"v":    payloadV,
		"ver":  ver,
	})
	return bytes.TrimRight(buf.Bytes(), "\n"), err
}

// NormaliseDevice does exactly what the Temple app does to the signed dev
// value before comparing it with its own device hash (app.py verify_licence:
// strip, lower-case): it trims surrounding whitespace and lower-cases, and
// requires a non-empty value. There is no format or length rule.
func NormaliseDevice(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", errors.New("Enter the Device ID shown in Temple Accounts.")
	}
	return s, nil
}

// GenerateKeyPair makes a fresh Ed25519 pair. The private value is the
// base64 of the 32-byte seed (accepted by NewSigner); the public value is
// the base64 of the raw 32-byte public key. Nothing is stored or logged.
func GenerateKeyPair() (privB64, pubB64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(pub), nil
}

// Verify checks a key string against a public key (used by tests and the
// admin "does my server key match" check).
func Verify(key string, pub ed25519.PublicKey) (payload []byte, ok bool) {
	body, found := strings.CutPrefix(strings.TrimSpace(key), KeyPrefix)
	if !found {
		return nil, false
	}
	p, s, found := strings.Cut(body, ".")
	if !found {
		return nil, false
	}
	payload, err := b64.DecodeString(p)
	if err != nil {
		return nil, false
	}
	sig, err := b64.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return payload, ed25519.Verify(pub, append([]byte(SigningPrefix), payload...), sig)
}

func decodeB64(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("-", "+", "_", "/").Replace(strings.TrimRight(s, "="))
	return base64.RawStdEncoding.DecodeString(s)
}

// GenerateKeyPairFromSeed returns the private (seed) and public base64
// values for a given 32-byte seed. Used by tests.
func GenerateKeyPairFromSeed(seed []byte) (privB64, pubB64 string, err error) {
	if len(seed) != ed25519.SeedSize {
		return "", "", errors.New("licence: seed must be 32 bytes")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	return base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey)), nil
}
