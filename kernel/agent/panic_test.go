// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// nilRespProvider violates the Provider contract by returning (nil, nil) — the
// classic third-party-plugin bug the loop must survive (M168).
type nilRespProvider struct{}

func (nilRespProvider) Name() string { return "nilresp" }
func (nilRespProvider) Complete(context.Context, agent.CompletionRequest) (*agent.CompletionResponse, error) {
	return nil, nil
}

// panicProvider panics inside Complete — must not crash the process (M168).
type panicProvider struct{}

func (panicProvider) Name() string { return "panicky" }
func (panicProvider) Complete(context.Context, agent.CompletionRequest) (*agent.CompletionResponse, error) {
	panic("boom from a misbehaving provider")
}

func TestRun_NilResponse_FailsGracefully(t *testing.T) {
	b, j := newTestBus(t)
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: nilRespProvider{}, Bus: b, Actor: "a", CorrelationID: "corr-nil",
	}, "x")
	if err == nil || !strings.Contains(err.Error(), "nil response") {
		t.Fatalf("err = %v; want a nil-response error", err)
	}
	assertTaskFailed(t, j, "error") // a returned (non-panic) error → reason=error
}

func TestRun_ProviderPanic_RecoveredAsFailure(t *testing.T) {
	b, j := newTestBus(t)
	// The whole point: this call returns instead of crashing the test process.
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: panicProvider{}, Bus: b, Actor: "a", CorrelationID: "corr-panic",
	}, "x")
	if !errors.Is(err, agent.ErrPanic) {
		t.Fatalf("err = %v; want ErrPanic", err)
	}
	if !strings.Contains(err.Error(), "boom from a misbehaving provider") {
		t.Errorf("err = %v; want it to carry the original panic value", err)
	}
	assertTaskFailed(t, j, "panic")
}

// assertTaskFailed checks exactly one task.failed with the given reason, and no
// task.completed, was journaled.
func assertTaskFailed(t *testing.T, j *journal.Journal, wantReason string) {
	t.Helper()
	var failed, completed int
	var reason string
	_ = j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskFailed:
			failed++
			var p struct {
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			reason = p.Reason
		case event.KindTaskCompleted:
			completed++
		}
		return nil
	})
	if failed != 1 {
		t.Errorf("task.failed count = %d, want exactly 1", failed)
	}
	if completed != 0 {
		t.Errorf("task.completed count = %d, want 0", completed)
	}
	if reason != wantReason {
		t.Errorf("task.failed reason = %q, want %q", reason, wantReason)
	}
}
