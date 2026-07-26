package connectors

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

func OAuthValues(provider OAuthProvider, redirectURI, state, verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	values := url.Values{
		"client_id":             {provider.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(provider.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
	}
	for key, value := range provider.ExtraAuthorize {
		values.Set(key, value)
	}
	return provider.AuthorizeURL + "?" + values.Encode()
}

func RandomURLToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func StateHash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
