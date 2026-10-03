// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

func TestFinalizeToolJobs_TerminalAuditFailurePreservesCauses(t *testing.T) {
	for _, mode := range []string{"panic", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			auditFailure := errors.New("terminal journal unavailable")
			attempts := 0
			hooks := 0
			s := newRunState(LoopConfig{ToolResultHook: func(context.Context, ToolCall, Result) { hooks++ }}, func(kind event.Kind, _ string, _ any) (*event.Event, error) {
				if kind == event.KindToolResult {
					attempts++
					if attempts == 1 {
						return nil, auditFailure
					}
				}
				return &event.Event{}, nil
			})
			fault := &toolJob{tc: ToolCall{ID: "one", Name: "fault"}, tool: &stubTool{}, invokeErr: ErrPanic, panicked: true}
			want := ErrPanic
			if mode == "cancel" {
				cancel()
				fault.panicked = false
				fault.invokeErr = context.Canceled
				want = context.Canceled
			}
			jobs := []*toolJob{fault, {tc: ToolCall{ID: "two", Name: "sibling"}, tool: &stubTool{}, result: Result{Output: "finished"}}}
			messages, err := s.finalizeToolJobs(ctx, jobs, 0, nil)
			if !errors.Is(err, want) || !errors.Is(err, auditFailure) {
				t.Fatalf("causes lost: %v", err)
			}
			if attempts != 2 || hooks != 0 || messages != nil {
				t.Errorf("attempts=%d hooks=%d messages=%v", attempts, hooks, messages)
			}
		})
	}
}

func TestFinalizeToolJobs_CancellationDuringAuditStopsHooks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hooks, results := 0, 0
	s := newRunState(LoopConfig{ToolResultHook: func(context.Context, ToolCall, Result) { hooks++ }}, func(kind event.Kind, _ string, _ any) (*event.Event, error) {
		if kind == event.KindToolResult {
			results++
			cancel()
		}
		return &event.Event{}, nil
	})
	jobs := []*toolJob{{tc: ToolCall{ID: "one"}, tool: &stubTool{}, result: Result{Output: "done"}}, {tc: ToolCall{ID: "two"}, tool: &stubTool{}, result: Result{Output: "done"}}}
	messages, err := s.finalizeToolJobs(ctx, jobs, 0, nil)
	if !errors.Is(err, context.Canceled) || hooks != 0 || results != 2 || messages != nil {
		t.Fatalf("error=%v hooks=%d records=%d messages=%v", err, hooks, results, messages)
	}
}
