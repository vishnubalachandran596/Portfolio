package licence

import (
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// Interoperability vector from the Temple Accounts app owner. The seed is a
// throwaway TEST key; it must never be used in production.
const (
	vecSeedHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	vecPubB64  = "ebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ="
	vecDevice  = "9f7828e963f3f765e509de5cddb6c3aee32482b4aec979ca60b8e578f51ca241"
	vecPayload = `{"dev":"9f7828e963f3f765e509de5cddb6c3aee32482b4aec979ca60b8e578f51ca241","iat":"2026-10-19T12:00:00Z","kid":"test-1","lic":"TAB-TEST-0001","prod":"temple-accounts","v":1,"ver":"1"}`
	vecKey     = "VBL1-eyJkZXYiOiI5Zjc4MjhlOTYzZjNmNzY1ZTUwOWRlNWNkZGI2YzNhZWUzMjQ4MmI0YWVjOTc5Y2E2MGI4ZTU3OGY1MWNhMjQxIiwiaWF0IjoiMjAyNi0xMC0xOVQxMjowMDowMFoiLCJraWQiOiJ0ZXN0LTEiLCJsaWMiOiJUQUItVEVTVC0wMDAxIiwicHJvZCI6InRlbXBsZS1hY2NvdW50cyIsInYiOjEsInZlciI6IjEifQ.Paxkt7qmZhvHiBnZnYzafxxPBFF-kAkN2SE8KVyPEy9400hHhj7j76eThXALxqXNIq6KUHCsd0w87ikmpBLTDQ"
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
	p, err := Payload(vecDevice, iat, "test-1", "TAB-TEST-0001", "temple-accounts", "1")
	if err != nil {
		t.Fatal(err)
	}
	if string(p) != vecPayload {
		t.Fatalf("payload\n got %s\nwant %s", p, vecPayload)
	}
	key, err := s.Issue("temple-accounts", "TAB-TEST-0001", vecDevice, "1", iat)
	if err != nil {
		t.Fatal(err)
	}
	if key != vecKey {
		t.Fatalf("key\n got %s\nwant %s", key, vecKey)
	}
}

func TestNormaliseDevice(t *testing.T) {
	got, err := NormaliseDevice("  9f7828e963f3\n")
	if err != nil || got != "9f7828e963f3" {
		t.Fatalf("got %q %v", got, err)
	}
	// No format or length rule: anything non-empty is kept as typed.
	for _, ok := range []string{"xyz", "9F78-28E9 63F3", strings.Repeat("a", 500)} {
		if got, err := NormaliseDevice(ok); err != nil || got != ok {
			t.Errorf("%q should pass unchanged, got %q %v", ok, got, err)
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
	key, err := s.Issue("temple-accounts", "TAB-X", vecDevice, "1", time.Now())
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
