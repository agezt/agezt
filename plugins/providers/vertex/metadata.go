// SPDX-License-Identifier: MIT

package vertex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

// Vertex ambient credentials: the GCE/GKE instance metadata server.
//
// On Google Cloud (Compute Engine, GKE with Workload Identity, Cloud Run),
// the platform exposes a short-lived OAuth access token for the instance's
// service account at a link-local metadata endpoint — no static key file on
// disk. This is the credential path Google recommends for production
// (rotating, scoped, never exfiltrable as a file). We mint via:
//
//	GET http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token
//	    Metadata-Flavor: Google
//	→ {"access_token":"...","expires_in":3599,"token_type":"Bearer"}
//
// The `Metadata-Flavor: Google` header is mandatory: the metadata server
// rejects requests without it, which defends against DNS-rebinding / SSRF
// that could otherwise trick a browser-like client into fetching tokens.

const (
	// DefaultMetadataBaseURL is the link-local GCE/GKE metadata server.
	DefaultMetadataBaseURL = "http://metadata.google.internal"

	metadataTokenPath     = "/computeMetadata/v1/instance/service-accounts/default/token"
	metadataProjectIDPath = "/computeMetadata/v1/project/project-id"
	metadataFlavorHeader  = "Metadata-Flavor"
	metadataFlavorValue   = "Google"

	// metadataMaxBytes bounds a metadata response. Tokens and the
	// project-id are tiny; anything larger is a misconfigured proxy.
	metadataMaxBytes = 1 << 20
)

// MetadataTokenSource mints (and caches) OAuth access tokens from the
// Google Cloud instance metadata server — the ambient-credential path for
// GCE / GKE Workload Identity / Cloud Run. Safe for concurrent use.
// Implements TokenMinter.
type MetadataTokenSource struct {
	baseURL string
	http    *http.Client
	now     func() time.Time

	mu        sync.Mutex
	cached    string
	expiresAt time.Time
}

// NewMetadataTokenSource builds a metadata-backed token source. baseURL is
// the metadata server root (empty → DefaultMetadataBaseURL; tests and proxy
// setups override it). httpClient may be nil (→ http.DefaultClient).
func NewMetadataTokenSource(baseURL string, httpClient *http.Client) *MetadataTokenSource {
	if baseURL == "" {
		baseURL = DefaultMetadataBaseURL
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &MetadataTokenSource{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    httpClient,
		now:     time.Now,
	}
}

// Token returns a valid access token, fetching a fresh one from the
// metadata server when the cached token is missing or near expiry.
func (m *MetadataTokenSource) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cached != "" && m.now().Add(TokenSkew).Before(m.expiresAt) {
		return m.cached, nil
	}
	tok, expiresIn, err := m.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	m.cached = tok
	m.expiresAt = m.now().Add(time.Duration(expiresIn) * time.Second)
	return tok, nil
}

func (m *MetadataTokenSource) fetchToken(ctx context.Context) (token string, expiresIn int, err error) {
	raw, err := m.get(ctx, metadataTokenPath)
	if err != nil {
		return "", 0, err
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", 0, fmt.Errorf("vertex: parse metadata token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", 0, errors.New("vertex: metadata token response missing access_token")
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600 // default 1h, matching Google's spec
	}
	return tr.AccessToken, tr.ExpiresIn, nil
}

// ProjectID fetches the project id of the instance from the metadata
// server. Used to fill GOOGLE_VERTEX_PROJECT when the operator leaves it
// unset — the fully ambient experience. The metadata server returns the
// project id as a plain-text body (not JSON), so we trim and return it.
func (m *MetadataTokenSource) ProjectID(ctx context.Context) (string, error) {
	raw, err := m.get(ctx, metadataProjectIDPath)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(raw))
	if id == "" {
		return "", errors.New("vertex: metadata project-id is empty")
	}
	return id, nil
}

// get performs an authenticated metadata GET against path, returning the
// bounded response body on a 2xx. It always sets the mandatory
// Metadata-Flavor header.
func (m *MetadataTokenSource) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("vertex: build metadata request %s: %w", path, err)
	}
	req.Header.Set(metadataFlavorHeader, metadataFlavorValue)
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vertex: metadata request %s (is this host on GCE/GKE/Cloud Run?): %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := httpread.All(resp.Body, metadataMaxBytes)
	if err != nil {
		return nil, fmt.Errorf("vertex: read metadata response %s: %w", path, err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("vertex: metadata %s status %d: %s", path, resp.StatusCode, string(raw))
	}
	return raw, nil
}
