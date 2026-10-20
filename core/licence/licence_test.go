package licence

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// Interoperability vector (payload without "ver"; fixed test seed and iat). The seed is a
// throwaway TEST key; it must never be used in production.
const (
	vecSeedHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	vecPubB64  = "ebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ="
	vecDevice  = "9f7828e963f3f765e509de5cddb6c3aee32482b4aec979ca60b8e578f51ca241"
	vecPayload = `{"dev":"9f7828e963f3f765e509de5cddb6c3aee32482b4aec979ca60b8e578f51ca241","iat":"2026-10-19T12:00:00Z","kid":"test-1","lic":"TAB-TEST-0001","prod":"temple-accounts","v":1}`
	vecKey     = "VBL1-eyJkZXYiOiI5Zjc4MjhlOTYzZjNmNzY1ZTUwOWRlNWNkZGI2YzNhZWUzMjQ4MmI0YWVjOTc5Y2E2MGI4ZTU3OGY1MWNhMjQxIiwiaWF0IjoiMjAyNi0xMC0xOVQxMjowMDowMFoiLCJraWQiOiJ0ZXN0LTEiLCJsaWMiOiJUQUItVEVTVC0wMDAxIiwicHJvZCI6InRlbXBsZS1hY2NvdW50cyIsInYiOjF9.t6-RzkzhXUzW0bbsw6v0sWGpHMePxVmTEEXB6qR1-1CLVcItqxUIYZ7goo9RbRwq6oHlR_o46Y8iqoT9Rp5pCg"
)

func testSigner(t *testing.T) *Signer {
	t.Helper()
	seed, _ := hex.DecodeString(vecSeedHex)
	priv := ed25519.NewKeyFromSeed(seed)
	s, err := NewSigner(b64Std(priv.Seed()), "test-1")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func b64Std(b []byte) string {
	s, _, _ := GenerateKeyPairFromSeed(b)
	return s
}

func TestVectorPublicKey(t *testing.T) {
	if got := testSigner(t).PublicKeyB64(); got != vecPubB64 {
		t.Fatalf("public key %q, want %q", got, vecPubB64)
	}
}

func TestVectorPayloadAndKey(t *testing.T) {
	s := testSigner(t)
	iat := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	p, err := Payload(vecDevice, iat, "test-1", "TAB-TEST-0001", "temple-accounts")
	if err != nil {
		t.Fatal(err)
	}
	if string(p) != vecPayload {
		t.Fatalf("payload\n got %s\nwant %s", p, vecPayload)
	}
	key, err := s.Issue("temple-accounts", "TAB-TEST-0001", vecDevice, iat)
	if err != nil {
		t.Fatal(err)
	}
	if key != vecKey {
		t.Fatalf("key\n got %s\nwant %s", key, vecKey)
	}
}

func TestNormaliseDevice(t *testing.T) {
	got, err := NormaliseDevice("  9F7828E963F3\n")
	if err != nil || got != "9f7828e963f3" {
		t.Fatalf("got %q %v", got, err)
	}
	// No format or length rule beyond trim + lower-case, like the app.
	for in, want := range map[string]string{"xyz": "xyz", "9F78-28E9 63F3": "9f78-28e9 63f3", strings.Repeat("A", 500): strings.Repeat("a", 500)} {
		if got, err := NormaliseDevice(in); err != nil || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "\n\t"} {
		if _, err := NormaliseDevice(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestGenerateRoundTrip(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSigner(priv, "k1")
	if err != nil {
		t.Fatal(err)
	}
	if s.PublicKeyB64() != pub {
		t.Fatal("generated public key does not match signer")
	}
	key, err := s.Issue("temple-accounts", "TAB-X", vecDevice, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := decodeB64(pub)
	if _, ok := Verify(key, ed25519.PublicKey(raw)); !ok {
		t.Fatal("signature did not verify")
	}
	if _, ok := Verify(key[:len(key)-3]+"AAA", ed25519.PublicKey(raw)); ok {
		t.Fatal("tampered signature verified")
	}
}

// A key carries no app version: the signed payload has no "ver" field, so
// one key covers every version and build on the same computer.
func TestPayloadHasNoVersion(t *testing.T) {
	if strings.Contains(vecPayload, `"ver"`) {
		t.Fatal("test vector must not contain ver")
	}
	key, err := testSigner(t).Issue("temple-accounts", "TAB-TEST-0001", vecDevice, time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := decodeB64(vecPubB64)
	payload, ok := Verify(key, ed25519.PublicKey(pub))
	if !ok || strings.Contains(string(payload), "ver") {
		t.Fatalf("ok=%v payload=%s", ok, payload)
	}
}
