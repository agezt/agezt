// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
)

// flakyProvider fails its first `failures` calls with errText, then succeeds.
type flakyProvider struct {
	name     string
	failures int64
	errText  string
	calls    atomic.Int64
}

func (p *flakyProvider) Name() string { return p.name }
func (p *flakyProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	n := p.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n <= p.failures {
		return nil, errors.New(p.errText)
	}
	return okResp(p.name, 1, 1), nil
}

// TestGovernor_RetryInPlaceOnTransient (M882): a transient error (429) on the
// primary is retried with backoff ON THE SAME provider — the call recovers
// without ever falling back, and a provider.retry event is journaled.
func TestGovernor_RetryInPlaceOnTransient(t *testing.T) {
	b, j := newBus(t)
	primary := &flakyProvider{name: "p1", failures: 1, errText: "upstream 429 too many requests"}
	fallback := &fakeProvider{name: "p2", resp: okResp("p2", 1, 1)}
	r := governor.NewRegistry()
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p1", Provider: primary, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "p2", Provider: fallback, IsFallback: true},
	)
	g, err := governor.New(governor.Config{Registry: r, Bus: b, RetryBaseDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "m"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "ok from p1" {
		t.Errorf("answered by %q, want the retried primary", resp.Message.Content)
	}
	if got := primary.calls.Load(); got != 2 {
		t.Errorf("primary calls = %d, want 2 (initial + 1 retry)", got)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Errorf("fallback calls = %d, want 0 — the retry should have kept the call on the primary", got)
	}

	var retries, fallbacks int
	_ = j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindProviderRetry:
			retries++
		case event.KindProviderFallback:
			fallbacks++
		}
		return nil
	})
	if retries != 1 {
		t.Errorf("provider.retry events = %d, want 1", retries)
	}
	if fallbacks != 0 {
		t.Errorf("provider.fallback events = %d, want 0", fallbacks)
	}
}

// TestGovernor_NoRetryOnNonTransient (M882): a non-transient provider error
// (auth) is NOT retried in place — the chain falls back immediately, exactly
// the historical behaviour.
func TestGovernor_NoRetryOnNonTransient(t *testing.T) {
	b, _ := newBus(t)
	primary := &fakeProvider{name: "p1", err: errors.New("401 invalid api key")}
	fallback := &fakeProvider{name: "p2", resp: okResp("p2", 1, 1)}
	r := governor.NewRegistry()
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p1", Provider: primary, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "p2", Provider: fallback, IsFallback: true},
	)
	g, err := governor.New(governor.Config{Registry: r, Bus: b, RetryBaseDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "m"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "ok from p2" {
		t.Errorf("answered by %q, want the fallback", resp.Message.Content)
	}
	if got := primary.calls.Load(); got != 1 {
		t.Errorf("primary calls = %d, want exactly 1 (no in-place retry of an auth error)", got)
	}
}

// tornStreamProvider emits one text delta and then fails — a stream that dies
// mid-flight after output reached the consumer.
type tornStreamProvider struct {
	fakeProvider
}

func (p *tornStreamProvider) CompleteStream(ctx context.Context, _ agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	p.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := onChunk(agent.Chunk{TextDelta: "partial "}); err != nil {
		return nil, err
	}
	return nil, errors.New("upstream 503 mid-stream")
}

// TestGovernor_StreamInterruptedIsTerminal (M882): once chunks have reached
// the consumer, a streaming failure must NOT retry or fall back — that would
// replay the stream and duplicate the output the user already saw.
func TestGovernor_StreamInterruptedIsTerminal(t *testing.T) {
	b, _ := newBus(t)
	primary := &tornStreamProvider{fakeProvider: fakeProvider{name: "p1"}}
	fallback := &fakeProvider{name: "p2", resp: okResp("p2", 1, 1)}
	r := governor.NewRegistry()
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p1", Provider: primary, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "p2", Provider: fallback, IsFallback: true},
	)
	g, err := governor.New(governor.Config{Registry: r, Bus: b, RetryBaseDelay: time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = g.CompleteStream(context.Background(), agent.CompletionRequest{Model: "m"}, func(agent.Chunk) error { return nil })
	if err == nil {
		t.Fatal("CompleteStream succeeded, want a terminal stream-interrupted error")
	}
	if !errors.Is(err, governor.ErrStreamInterrupted) {
		t.Errorf("error = %v, want ErrStreamInterrupted", err)
	}
	if got := primary.calls.Load(); got != 1 {
		t.Errorf("primary calls = %d, want 1 (no retry after output started)", got)
	}
	if got := fallback.calls.Load(); got != 0 {
		t.Errorf("fallback calls = %d, want 0 (no fallback after output started)", got)
	}
}
