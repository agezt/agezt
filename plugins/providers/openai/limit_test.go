// SPDX-License-Identifier: MIT

package openai_test

// Live proof for M189: an openai-compatible endpoint (the operator-
// configurable / untrusted base-URL path) returning an over-cap response
// body must produce a clean error, not OOM the daemon. We lower the cap
// and point the provider at a mock server that returns a body past it.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
	"github.com/agezt/agezt/plugins/providers/openai"
)

func TestComplete_RejectsOversizedResponseBody(t *testing.T) {
	old := httpread.DefaultMaxResponseBytes
	httpread.DefaultMaxResponseBytes = 1024
	defer func() { httpread.DefaultMaxResponseBytes = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 8192)) // 8 KiB >> 1 KiB cap
	}))
	defer srv.Close()

	p := openai.New("sk-test")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "gpt-4o-mini",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected an error for an oversized response body")
	}
	if !errors.Is(err, httpread.ErrResponseTooLarge) {
		t.Errorf("err = %v; want it to wrap ErrResponseTooLarge", err)
	}
}
