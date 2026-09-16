// SPDX-License-Identifier: MIT
//
// plugins/providers/vertex auth: ServiceAccountKey + TokenSource + TokenMinter
// types + LoadServiceAccountFile + ParseServiceAccountJSON + parsePrivateKey
// + NewTokenSource.
// Extracted from auth.go during Day 211 god-file refactor (#89).
// Public API unchanged.
package vertex

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	// CloudPlatformScope grants access to all Google Cloud APIs the
	// account is authorised for. Vertex requires this scope.
	CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
	// JWTBearerGrantType is RFC 7523's grant type for JWT-bearer flow.
	JWTBearerGrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	// TokenSkew is how much earlier than the stated expiry we
	// consider a token stale — protects against clock drift and
	// in-flight request latency.
	TokenSkew = 60 * time.Second
)

// ServiceAccountKey models the subset of the Google service-account
type ServiceAccountKey struct {
	Type         string `json:"type"` // "service_account"
	ProjectID    string `json:"project_id"`
	PrivateKey   string `json:"private_key"`    // PEM-encoded RSA
	PrivateKeyID string `json:"private_key_id"` // kid header
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"` // typically https://oauth2.googleapis.com/token
}

func LoadServiceAccountFile(path string) (*ServiceAccountKey, error) {
	if path == "" {
		return nil, errors.New("vertex: GOOGLE_APPLICATION_CREDENTIALS path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vertex: read service account file %q: %w", path, err)
	}
	return ParseServiceAccountJSON(raw)
}
func ParseServiceAccountJSON(raw []byte) (*ServiceAccountKey, error) {
	var sa ServiceAccountKey
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("vertex: parse service account JSON: %w", err)
	}
	if sa.Type != "" && sa.Type != "service_account" {
		return nil, fmt.Errorf("vertex: unsupported credential type %q (M1.n only supports service_account; ADC/workload-identity in M1.n.x)", sa.Type)
	}
	if sa.ClientEmail == "" {
		return nil, errors.New("vertex: service account JSON missing client_email")
	}
	if sa.PrivateKey == "" {
		return nil, errors.New("vertex: service account JSON missing private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &sa, nil
}
func parsePrivateKey(pemBytes string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemBytes))
	if block == nil {
		return nil, errors.New("vertex: private_key is not PEM-encoded")
	}
	// Try PKCS#8 first (Google's format), fall back to PKCS#1.
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("vertex: private key is %T, want *rsa.PrivateKey", key)
		}
		return rsaKey, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("vertex: private_key is neither PKCS#8 nor PKCS#1 RSA")
}

// Provider can hold either without caring how the token was obtained.
type TokenMinter interface {
	Token(ctx context.Context) (string, error)
}

// TokenSource mints (and caches) OAuth2 access tokens for a service
// account. Safe for concurrent use. Implements TokenMinter.
type TokenSource struct {
	sa    *ServiceAccountKey
	key   *rsa.PrivateKey
	scope string
	http  *http.Client
	now   func() time.Time

	mu        sync.Mutex
	cached    string
	expiresAt time.Time
}

func NewTokenSource(sa *ServiceAccountKey, scope string, httpClient *http.Client) (*TokenSource, error) {
	key, err := parsePrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = CloudPlatformScope
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &TokenSource{
		sa:    sa,
		key:   key,
		scope: scope,
		http:  httpClient,
		now:   time.Now,
	}, nil
}
