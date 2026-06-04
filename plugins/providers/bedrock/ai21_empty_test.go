// SPDX-License-Identifier: MIT

package bedrock_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/bedrock"
)

// TestComplete_AI21EmptyChoicesErrorsNotPanic locks in the AI21-Jamba-on-Bedrock
// decodeResponse guard (ai21.go: "response has no choices"), mirroring the
// existing Mistral coverage. A server returning a well-formed body with an empty
// choices array must error cleanly rather than index choices[0] and panic. Fails
// on a panic as well as on a nil error.
func TestComplete_AI21EmptyChoicesErrorsNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	p := bedrock.New("t", "us-east-1")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "ai21.jamba-1-5-large-v1:0",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected an error on empty choices, got nil")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error doesn't mention empty choices: %v", err)
	}
}
