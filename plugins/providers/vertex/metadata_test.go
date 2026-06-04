// SPDX-License-Identifier: MIT

package vertex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/vertex"
)

// TestMetadataTokenSource_FetchesCachesAndSetsFlavorHeader: the metadata
// source GETs the token endpoint with the mandatory Metadata-Flavor:Google
// header, returns the access token, and serves a second call from cache
// (one server hit for two Token() calls).
func TestMetadataTokenSource_FetchesCachesAndSetsFlavorHeader(t *testing.T) {
	var hits atomic.Int32
	var sawFlavor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		sawFlavor = r.Header.Get("Metadata-Flavor")
		if !strings.HasSuffix(r.URL.Path, "/instance/service-accounts/default/token") {
			t.Errorf("unexpected token path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"access_token":"ya29.meta","expires_in":3599,"token_type":"Bearer"}`))
	}))
	defer srv.Close()

	m := vertex.NewMetadataTokenSource(srv.URL, nil)
	tok, err := m.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "ya29.meta" {
		t.Errorf("token=%q want ya29.meta", tok)
	}
	if sawFlavor != "Google" {
		t.Errorf("Metadata-Flavor header=%q want Google (the metadata server requires it)", sawFlavor)
	}
	// Second call within expiry must be served from cache — no new hit.
	if _, err := m.Token(context.Background()); err != nil {
		t.Fatalf("Token #2: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hit %d times, want 1 (second Token should hit cache)", got)
	}
}

// TestMetadataTokenSource_ProjectID: the project-id endpoint returns a
// plain-text body (not JSON); ProjectID trims and returns it.
func TestMetadataTokenSource_ProjectID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			t.Errorf("missing Metadata-Flavor header")
		}
		_, _ = w.Write([]byte("my-ambient-project\n"))
	}))
	defer srv.Close()

	m := vertex.NewMetadataTokenSource(srv.URL, nil)
	id, err := m.ProjectID(context.Background())
	if err != nil {
		t.Fatalf("ProjectID: %v", err)
	}
	if id != "my-ambient-project" {
		t.Errorf("project id=%q want my-ambient-project (trimmed)", id)
	}
}

// TestMetadataTokenSource_Non2xxErrors: a non-2xx metadata response is a
// clean error, not a panic or an empty token.
func TestMetadataTokenSource_Non2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("metadata server error"))
	}))
	defer srv.Close()

	m := vertex.NewMetadataTokenSource(srv.URL, nil)
	if _, err := m.Token(context.Background()); err == nil {
		t.Fatal("expected an error on a 500 from the metadata server, got nil")
	}
}

// TestMetadataTokenSource_MissingAccessTokenErrors: a well-formed body with
// no access_token must error rather than caching an empty token.
func TestMetadataTokenSource_MissingAccessTokenErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"expires_in":3599,"token_type":"Bearer"}`))
	}))
	defer srv.Close()

	m := vertex.NewMetadataTokenSource(srv.URL, nil)
	_, err := m.Token(context.Background())
	if err == nil || !strings.Contains(err.Error(), "access_token") {
		t.Fatalf("err=%v, want a missing-access_token error", err)
	}
}

// TestComplete_MetadataTokenFlowsToAuthHeader proves the full plumbing: a
// Provider built with a *MetadataTokenSource (via the TokenMinter interface)
// mints its bearer token from the metadata server and sends it on the Vertex
// request — the ambient-credentials happy path end to end.
func TestComplete_MetadataTokenFlowsToAuthHeader(t *testing.T) {
	metaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"ya29.ambient","expires_in":3599,"token_type":"Bearer"}`))
	}))
	defer metaSrv.Close()

	var gotAuth string
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hi ambient"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2}}`))
	}))
	defer apiSrv.Close()

	p := vertex.New(vertex.NewMetadataTokenSource(metaSrv.URL, nil), "test-project", "us-central1")
	p.Endpoint = apiSrv.URL + "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent"

	resp, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "hi ambient" {
		t.Errorf("content=%q", resp.Message.Content)
	}
	if gotAuth != "Bearer ya29.ambient" {
		t.Errorf("Authorization=%q want 'Bearer ya29.ambient' (metadata token should flow into the request)", gotAuth)
	}
}
