// SPDX-License-Identifier: MIT

package vertex_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/vertex"
)

// TestComplete_EmptyCandidatesErrorsNotPanic locks in the Gemini-on-Vertex
// decodeResponse guard (vertex.go: "response has no candidates"). Two servers:
// the OAuth token exchange and the generateContent endpoint, the latter
// returning a well-formed body with an empty candidates array. Decode must
// error cleanly rather than index candidates[0] and panic. Fails on a panic as
// well as on a nil error. (generateTestSAJSON lives in vertex_test.go.)
func TestComplete_EmptyCandidatesErrorsNotPanic(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"ya29.real","expires_in":3600,"token_type":"Bearer"}`))
	}))
	defer tokenSrv.Close()
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer apiSrv.Close()

	sa, _ := vertex.ParseServiceAccountJSON(generateTestSAJSON(t, tokenSrv.URL))
	ts, _ := vertex.NewTokenSource(sa, "", nil)
	p := vertex.New(ts, "test-project", "us-central1")
	p.Endpoint = apiSrv.URL + "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent"

	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gemini-1.5-flash",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected an error on empty candidates, got nil")
	}
	if !strings.Contains(err.Error(), "no candidates") {
		t.Errorf("error doesn't mention empty candidates: %v", err)
	}
}
