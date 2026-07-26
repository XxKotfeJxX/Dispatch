package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"dispatch/internal/config"
)

func TestGitHubAppJWTUsesConfiguredRSAKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	api := API{Config: config.Config{Connectors: config.ConnectorConfig{
		GitHubAppID:         42,
		GitHubPrivateKeyB64: base64.StdEncoding.EncodeToString(pemBytes),
	}}}
	token, err := api.githubAppJWT(time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("unexpected JWT: %q", token)
	}
}

func TestDecodeGitHubPrivateKeyRejectsInvalidValue(t *testing.T) {
	if _, err := decodeGitHubPrivateKey("not-base64"); err == nil {
		t.Fatal("invalid private key was accepted")
	}
}
