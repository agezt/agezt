// SPDX-License-Identifier: MIT

package google_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/google"
)

// TestComplete_EmptyCandidatesErrorsNotPanic locks in the decodeResponse guard
// (google.go: "response has no candidates"). Gemini can legitimately return a
// body with no candidates (e.g. a safety block), and a degraded proxy can
// truncate one; either way decode must error cleanly rather than index
// candidates[0] and panic. The test fails on a panic as well as on a nil error.
func TestComplete_EmptyCandidatesErrorsNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	p := google.New("test-key")
	p.Endpoint = srv.URL + "/v1beta/models/gemini-1.5-flash:generateContent"
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
