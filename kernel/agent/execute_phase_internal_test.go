// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

type executeJobProbe struct {
	invoke func(context.Context, json.RawMessage) (Result, error)
}

type executionContextPanic struct{ ctx context.Context }

func (p executionContextPanic) Error() string {
	if p.ctx.Err() != nil {
		return "after-cancel"
	}
	return "before-cancel"
}

func (executeJobProbe) Definition() ToolDef { panic("execution must not repeat admission") }
func (p executeJobProbe) Invoke(ctx context.Context, input json.RawMessage) (Result, error) {
	return p.invoke(ctx, input)
}

func TestInvokeToolJobRetainsExecutionContract(t *testing.T) {
	for _, mode := range []string{"disabled", "negative", "success", "error", "panic", "context-panic", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = toolapi.WithAgent(ctx, "actor")
			ctx = toolapi.WithWorkdir(ctx, "isolated-workspace")
			input := json.RawMessage(" {\n \"n\": 1 } ")
			cause := errors.New("opaque backend cause")
			partial := Result{Output: "partial", IsError: true}
			calls := 0
			var captured context.Context
			tool := executeJobProbe{invoke: func(got context.Context, raw json.RawMessage) (Result, error) {
				captured = got
				calls++
				if toolapi.AgentFromContext(got) != "actor" || toolapi.WorkdirFromContext(got) != "isolated-workspace" || toolapi.CorrelationFromContext(got) != "corr" || !reflect.DeepEqual(raw, input) {
					t.Error("caller context/input lost")
				}
				switch mode {
				case "context-panic":
					panic(executionContextPanic{ctx: got})
				case "panic":
					panic(cause)
				case "timeout":
					<-got.Done()
					return partial, cause
				case "cancel":
					cancel()
					return partial, got.Err()
				case "error":
					return partial, cause
				}
				return Result{Output: "ok"}, nil
			}}
			timeout := time.Minute
			if mode == "disabled" {
				timeout = 0
			}
			if mode == "negative" {
				timeout = -time.Second
			}
			if mode == "timeout" {
				timeout = 10 * time.Millisecond
			}
			job := &toolJob{tool: tool, tc: ToolCall{ID: "call", Name: "probe", Input: input}}
			invokeToolJob(ctx, LoopConfig{CorrelationID: "corr", ToolTimeout: timeout}, job)
			if calls != 1 || captured == nil || job.toolTimedOut != (mode == "timeout") || job.panicked != (mode == "panic" || mode == "context-panic") {
				t.Fatalf("calls=%d job=%+v", calls, job)
			}
			if _, deadline := captured.Deadline(); deadline != (timeout > 0) {
				t.Errorf("deadline=%t timeout=%s", deadline, timeout)
			}
			if timeout > 0 && captured.Err() == nil {
				t.Error("per-call context not released")
			}
			switch mode {
			case "panic":
				if !errors.Is(job.invokeErr, ErrPanic) {
					t.Fatalf("panic cause lost: %v", job.invokeErr)
				}
			case "context-panic":
				if !errors.Is(job.invokeErr, ErrPanic) || !strings.Contains(job.invokeErr.Error(), "before-cancel") {
					t.Fatalf("panic classified after cleanup: %v", job.invokeErr)
				}
			case "timeout", "error":
				if job.invokeErr != cause || !reflect.DeepEqual(job.result, partial) {
					t.Fatalf("backend outcome=%+v", job)
				}
			case "cancel":
				if !errors.Is(job.invokeErr, context.Canceled) || job.toolTimedOut {
					t.Fatalf("run cancellation=%+v", job)
				}
			default:
				if job.invokeErr != nil || job.result.Output != "ok" {
					t.Fatalf("success=%+v", job)
				}
			}
		})
	}
}
