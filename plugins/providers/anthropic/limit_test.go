// SPDX-License-Identifier: MIT

package anthropic_test

// Cross-dialect live proof for M190: the shared httpread bound (M189)
// applies to the anthropic provider too — an endpoint returning an
// over-cap body yields a clean error, not an OOM.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/anthropic"
	"github.com/agezt/agezt/plugins/providers/internal/httpread"
)

func TestComplete_RejectsOversizedResponseBody(t *testing.T) {
	old := httpread.DefaultMaxResponseBytes
	httpread.DefaultMaxResponseBytes = 1024
	defer func() { httpread.DefaultMaxResponseBytes = old }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 8192)) // 8 KiB >> 1 KiB cap
	}))
	defer srv.Close()

	p := anthropic.New("test-key")
	p.Endpoint = srv.URL
	_, err := p.Complete(context.Background(), agent.CompletionRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected an error for an oversized response body")
	}
	if !errors.Is(err, httpread.ErrResponseTooLarge) {
		t.Errorf("err = %v; want it to wrap ErrResponseTooLarge", err)
	}
}
