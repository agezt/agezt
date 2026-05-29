// SPDX-License-Identifier: MIT

package vertex

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Vertex auth: service-account JWT-bearer grant exchanged for an OAuth
// access token. We deliberately implement a minimal slice of
// golang.org/x/oauth2/google in-package to avoid pulling in the
// full oauth2 + cloud.google.com transitive deps.
//
// Flow (RFC 7523):
//
//  1. Load service-account JSON key file (path from
//     GOOGLE_APPLICATION_CREDENTIALS).
//  2. Build a JWT with iss=client_email, scope=cloud-platform,
//     aud=token_uri, iat=now, exp=now+1h.
//  3. Sign the JWT with the account's RSA private key (RS256).
//  4. POST the signed JWT to token_uri with
//     grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer.
//  5. Receive {access_token, expires_in, token_type:"Bearer"}.
//  6. Cache the access token; refresh before expiry.
//
// Application Default Credentials, workload-identity-federation, and
// the GCE metadata server are not implemented in M1.n — service-account
// JSON is the desktop/CI use case. ADC lands in M1.n.x.

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
// JSON key file we need. The full schema has more fields (client_id,
// auth_uri, etc.) we don't use.
type ServiceAccountKey struct {
	Type         string `json:"type"`           // "service_account"
	ProjectID    string `json:"project_id"`
	PrivateKey   string `json:"private_key"`    // PEM-encoded RSA
	PrivateKeyID string `json:"private_key_id"` // kid header
	ClientEmail  string `json:"client_email"`
	TokenURI     string `json:"token_uri"`      // typically https://oauth2.googleapis.com/token
}

// LoadServiceAccountFile reads + parses a service-account JSON key
// from the given filesystem path.
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

// ParseServiceAccountJSON parses raw JSON bytes into a ServiceAccountKey,
// validating that required fields are present and the key type is right.
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

// parsePrivateKey decodes the PEM-armoured RSA private key from the
// service account file. Google issues these as PKCS#8 keys.
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

// b64url is the JWT-style base64url-without-padding encoder.
func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// signJWT builds and signs a JWT-bearer assertion for the given
// service account, scope, and audience. now is injectable for tests.
func signJWT(sa *ServiceAccountKey, key *rsa.PrivateKey, scope, aud string, now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if sa.PrivateKeyID != "" {
		header["kid"] = sa.PrivateKeyID
	}
	hdrJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claims := map[string]any{
		"iss":   sa.ClientEmail,
		"scope": scope,
		"aud":   aud,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64url(hdrJSON) + "." + b64url(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("vertex: sign JWT: %w", err)
	}
	return signingInput + "." + b64url(sig), nil
}

// TokenSource mints (and caches) OAuth2 access tokens for a service
// account. Safe for concurrent use.
type TokenSource struct {
	sa     *ServiceAccountKey
	key    *rsa.PrivateKey
	scope  string
	http   *http.Client
	now    func() time.Time

	mu        sync.Mutex
	cached    string
	expiresAt time.Time
}

// NewTokenSource builds a TokenSource for the given service account.
// scope defaults to CloudPlatformScope when empty. httpClient may be
// nil (defaults to http.DefaultClient).
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

// Token returns a valid access token, minting a fresh one if the
// cached token is missing or near expiry.
func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.cached != "" && ts.now().Add(TokenSkew).Before(ts.expiresAt) {
		return ts.cached, nil
	}
	tok, expiresIn, err := ts.exchange(ctx)
	if err != nil {
		return "", err
	}
	ts.cached = tok
	ts.expiresAt = ts.now().Add(time.Duration(expiresIn) * time.Second)
	return tok, nil
}

// exchange performs the JWT-bearer → access-token roundtrip.
func (ts *TokenSource) exchange(ctx context.Context) (token string, expiresIn int, err error) {
	jwt, err := signJWT(ts.sa, ts.key, ts.scope, ts.sa.TokenURI, ts.now())
	if err != nil {
		return "", 0, err
	}
	body := url.Values{
		"grant_type": {JWTBearerGrantType},
		"assertion":  {jwt},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.sa.TokenURI, strings.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("vertex: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := ts.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("vertex: token exchange: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("vertex: read token response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", 0, fmt.Errorf("vertex: token exchange status %d: %s", resp.StatusCode, string(raw))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", 0, fmt.Errorf("vertex: parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, errors.New("vertex: token response missing access_token")
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600 // default 1h per Google's spec
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}
