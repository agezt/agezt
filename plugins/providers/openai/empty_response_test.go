// SPDX-License-Identifier: MIT

package openai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/openai"
)

// TestComplete_EmptyChoicesErrorsNotPanic locks in the decodeResponse guard
// (openai.go: "response has no choices"). A server — or a flaky proxy that
// truncates the JSON — returning a well-formed body with an EMPTY choices array
// must surface a clean error, never index choices[0] and panic. This guard is
// load-bearing for availability; a future refactor that drops it would
// reintroduce a crash on a hostile/degraded response, and this test fails on a
// panic (the harness reports it) as well as on a nil error.
func TestComplete_EmptyChoicesErrorsNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"cmpl-x","object":"chat.completion","choices":[]}`))
	}))
	defer srv.Close()

	p := openai.New("sk-test")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected an error on empty choices, got nil")
	}
	if !strings.Contains(err.Error(), "no choices") {
		t.Errorf("error doesn't mention empty choices: %v", err)
	}
}
