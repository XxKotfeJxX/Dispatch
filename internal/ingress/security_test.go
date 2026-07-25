package ingress

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func TestSecretHashAndEncryption(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil || len(secret) < 32 {
		t.Fatalf("secret=%q err=%v", secret, err)
	}
	if !VerifyStoredHash(HashSecret(secret), secret) || VerifyStoredHash(HashSecret(secret), "wrong") {
		t.Fatal("hash verification failed")
	}
	ciphertext, err := EncryptSecret("encryption-key", secret)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := DecryptSecret("encryption-key", ciphertext)
	if err != nil || plain != secret {
		t.Fatalf("plain=%q err=%v", plain, err)
	}
}

func TestProviderSignatures(t *testing.T) {
	secret, body := "signing-secret", []byte(`{"id":"evt_1"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	if !VerifyHMAC(secret, body, "sha256="+hex.EncodeToString(mac.Sum(nil))) {
		t.Fatal("generic HMAC rejected")
	}
	now := time.Unix(1_800_000_000, 0)
	timestamp := "1800000000"
	mac = hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + timestamp + ":" + string(body)))
	if !VerifySlack(secret, body, timestamp, "v0="+hex.EncodeToString(mac.Sum(nil)), now) {
		t.Fatal("Slack signature rejected")
	}
	mac = hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	if !VerifyStripe(secret, body, "t="+timestamp+",v1="+hex.EncodeToString(mac.Sum(nil)), now) {
		t.Fatal("Stripe signature rejected")
	}
}

func TestSignatureTimestampWindow(t *testing.T) {
	if VerifySlack("secret", []byte("{}"), "1", "v0=bad", time.Now()) {
		t.Fatal("stale Slack request accepted")
	}
	if VerifyStripe("secret", []byte("{}"), "t=1,v1=bad", time.Now()) {
		t.Fatal("stale Stripe request accepted")
	}
}
