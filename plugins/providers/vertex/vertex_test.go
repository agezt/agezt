// SPDX-License-Identifier: MIT

package vertex_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ersinkoc/agezt/kernel/agent"
	"github.com/ersinkoc/agezt/plugins/providers/vertex"
)

// generateTestSAJSON creates a fresh RSA keypair and embeds it in a
// valid-looking service account JSON. Returns the JSON bytes.
func generateTestSAJSON(t *testing.T, tokenURI string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen rsa key: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	sa := vertex.ServiceAccountKey{
		Type:         "service_account",
		ProjectID:    "test-project",
		PrivateKey:   string(pemBytes),
		PrivateKeyID: "test-kid",
		ClientEmail:  "test@test-project.iam.gserviceaccount.com",
		TokenURI:     tokenURI,
	}
	raw, err := json.Marshal(sa)
	if err != nil {
		t.Fatalf("marshal sa json: %v", err)
	}
	return raw
}

func TestParseServiceAccountJSON_RejectsNonServiceAccount(t *testing.T) {
	raw := []byte(`{"type":"authorized_user","client_email":"u@x.com","private_key":"x"}`)
	_, err := vertex.ParseServiceAccountJSON(raw)
	if err == nil {
		t.Fatal("expected error for non-service-account type")
	}
	if !strings.Contains(err.Error(), "M1.n only supports service_account") {
		t.Errorf("error should name the deferral; got %v", err)
	}
}

func TestParseServiceAccountJSON_DefaultsTokenURI(t *testing.T) {
	raw := []byte(`{"type":"service_account","client_email":"u@x.com","private_key":"-----BEGIN-----\n-----END-----"}`)
	sa, err := vertex.ParseServiceAccountJSON(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sa.TokenURI != "https://oauth2.googleapis.com/token" {
		t.Errorf("token_uri=%q want google default", sa.TokenURI)
	}
}

func TestTokenSource_MintsAndCachesToken(t *testing.T) {
	var exchanges int
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		_ = r.ParseForm()
		// Verify the JWT-bearer grant params look right.
		if r.Form.Get("grant_type") != vertex.JWTBearerGrantType {
			t.Errorf("grant_type=%q", r.Form.Get("grant_type"))
		}
		assertion := r.Form.Get("assertion")
		if !strings.Contains(assertion, ".") || strings.Count(assertion, ".") != 2 {
			t.Errorf("assertion does not look like a JWT: %s", assertion)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.test-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer tokenSrv.Close()

	saJSON := generateTestSAJSON(t, tokenSrv.URL)
	sa, err := vertex.ParseServiceAccountJSON(saJSON)
	if err != nil {
		t.Fatalf("parse sa: %v", err)
	}
	ts, err := vertex.NewTokenSource(sa, "", nil)
	if err != nil {
		t.Fatalf("new token source: %v", err)
	}

	tok1, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok1 != "ya29.test-token" {
		t.Errorf("tok1=%q", tok1)
	}
	if exchanges != 1 {
		t.Errorf("first call: exchanges=%d want 1", exchanges)
	}
	// Second call within TTL should be served from cache.
	tok2, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token (cached): %v", err)
	}
	if tok2 != tok1 {
		t.Errorf("cached token mismatch: %q vs %q", tok1, tok2)
	}
	if exchanges != 1 {
		t.Errorf("after cached call: exchanges=%d want 1 (cache miss!)", exchanges)
	}
}

func TestTokenSource_BadJSONResponse(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	_, err := ts.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error; got %v", err)
	}
}

func TestResolveEndpoint(t *testing.T) {
	cases := []struct {
		name, endpoint, baseURL, project, location, model, want string
	}{
		{
			name:     "derive from region/project",
			project:  "my-project",
			location: "us-central1",
			model:    "gemini-1.5-flash",
			want:     "https://us-central1-aiplatform.googleapis.com/v1/projects/my-project/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent",
		},
		{
			name:     "custom BaseURL override",
			baseURL:  "https://private-vertex.internal",
			project:  "p",
			location: "europe-west4",
			model:    "gemini-1.5-pro",
			want:     "https://private-vertex.internal/v1/projects/p/locations/europe-west4/publishers/google/models/gemini-1.5-pro:generateContent",
		},
		{
			name:     "explicit full Endpoint wins",
			endpoint: "https://x.example/some/full/path",
			project:  "p", location: "l", model: "m",
			want: "https://x.example/some/full/path",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &vertex.Provider{
				Endpoint: c.endpoint,
				BaseURL:  c.baseURL,
				Project:  c.project,
				Location: c.location,
			}
			if got := p.ResolveEndpoint(c.model); got != c.want {
				t.Errorf("ResolveEndpoint=%q want %q", got, c.want)
			}
		})
	}
}

func TestComplete_HappyPathWithCachedToken(t *testing.T) {
	// Two servers: one for OAuth token exchange, one for the
	// generateContent endpoint.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.real",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer tokenSrv.Close()

	var seen struct {
		path, authz string
		body        map[string]any
	}
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path = r.URL.Path
		seen.authz = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "hi from vertex"}}},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 2, "candidatesTokenCount": 3},
		})
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "test-project", "us-central1")
	// Pin the endpoint so we can verify the body/auth headers without
	// running the real URL-builder against api.googleapis.com.
	p.Endpoint = apiSrv.URL + "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent"

	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		System:   "be terse",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi from vertex" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 2 || resp.Usage.OutputTokens != 3 {
		t.Errorf("usage=%+v", resp.Usage)
	}
	if seen.authz != "Bearer ya29.real" {
		t.Errorf("authz=%q", seen.authz)
	}
	if seen.path == "" || !strings.Contains(seen.path, ":generateContent") {
		t.Errorf("path=%q", seen.path)
	}
	// systemInstruction at top-level (same as plugins/providers/google).
	if _, ok := seen.body["systemInstruction"].(map[string]any); !ok {
		t.Errorf("body missing systemInstruction: %#v", seen.body)
	}
}

func TestComplete_NoTokenSource(t *testing.T) {
	p := vertex.New(nil, "p", "us-central1")
	_, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"})
	if err != vertex.ErrNoTokenSource {
		t.Errorf("got %v want ErrNoTokenSource", err)
	}
}

func TestComplete_MissingProjectOrLocation(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()
	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)

	// Missing project.
	p := vertex.New(ts, "", "us-central1")
	if _, err := p.Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err == nil ||
		!strings.Contains(err.Error(), "Project required") {
		t.Errorf("expected Project-required error; got %v", err)
	}
	// Missing location.
	p2 := vertex.New(ts, "p", "")
	if _, err := p2.Complete(context.Background(), agent.CompletionRequest{Model: "m"}); err == nil ||
		!strings.Contains(err.Error(), "Location required") {
		t.Errorf("expected Location-required error; got %v", err)
	}
}

func TestComplete_APIError(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "x", "expires_in": 3600})
	}))
	defer tokenSrv.Close()
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"message":"permission denied"}}`))
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "p", "us-central1")
	p.Endpoint = apiSrv.URL + "/x"
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "m",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	apiErr, ok := err.(*vertex.APIError)
	if !ok {
		t.Fatalf("got %v want *vertex.APIError", err)
	}
	if apiErr.Status != 403 {
		t.Errorf("status=%d", apiErr.Status)
	}
}

// Defensive: ensure the token cache really uses the skew window — a
// token expiring within TokenSkew is treated as stale.
func TestTokenSource_StalenessWindow(t *testing.T) {
	// Two distinct tokens so we can prove a second exchange happened.
	var n int
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		tok := "tok-" + map[bool]string{true: "1", false: "2"}[n == 1]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": tok,
			"expires_in":   30, // < TokenSkew (60s) → always stale
			"token_type":   "Bearer",
		})
	}))
	defer tokenSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	// First call mints tok-1.
	t1, _ := ts.Token(context.Background())
	// Second call should re-exchange because expires_in is below skew.
	t2, _ := ts.Token(context.Background())
	if t1 == t2 {
		t.Errorf("expected re-exchange when within skew window; got t1=%q t2=%q", t1, t2)
	}
}

// Sanity: NewTokenSource rejects a malformed PEM.
func TestNewTokenSource_BadPEM(t *testing.T) {
	sa := &vertex.ServiceAccountKey{
		Type:        "service_account",
		ClientEmail: "x@y.com",
		PrivateKey:  "not a pem",
		TokenURI:    "https://example.invalid",
	}
	_, err := vertex.NewTokenSource(sa, "", nil)
	if err == nil {
		t.Fatal("expected error for non-PEM key")
	}
}

// silence unused-import lint when time is referenced indirectly
var _ = time.Second
