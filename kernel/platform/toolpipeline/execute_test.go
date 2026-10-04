// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

type executionProbe struct {
	invoke func(context.Context, json.RawMessage) (toolapi.Result, error)
}

func (executionProbe) Definition() toolapi.ToolDef { panic("execution must not repeat admission") }
func (p executionProbe) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	return p.invoke(ctx, raw)
}

func TestExecuteRetainsOutcomeAndContext(t *testing.T) {
	for _, mode := range []string{"success", "mapper-success", "fast-budget", "negative-budget", "error", "reported-error", "panic", "mapped-panic", "timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = toolapi.WithCorrelation(ctx, "corr")
			ctx = toolapi.WithAgent(ctx, "actor")
			ctx = toolapi.WithWorkdir(ctx, "workspace")
			input := json.RawMessage(" {\n \"n\": 1 } ")
			partial := toolapi.Result{Output: "partial", IsError: true}
			cause, mappedCause := errors.New("backend cause"), errors.New("caller panic")
			calls, mapped := 0, 0
			var captured context.Context
			timeout := time.Duration(0)
			if mode == "fast-budget" || mode == "mapped-panic" || mode == "cancel" {
				timeout = time.Minute
			}
			if mode == "negative-budget" {
				timeout = -time.Second
			}
			if mode == "timeout" {
				timeout = 10 * time.Millisecond
			}
			tool := executionProbe{invoke: func(got context.Context, raw json.RawMessage) (toolapi.Result, error) {
				calls++
				captured = got
				if !reflect.DeepEqual(raw, input) || toolapi.CorrelationFromContext(got) != "corr" || toolapi.AgentFromContext(got) != "actor" || toolapi.WorkdirFromContext(got) != "workspace" || (timeout <= 0 && got != ctx) {
					t.Error("caller context/input changed")
				}
				switch mode {
				case "panic", "mapped-panic":
					panic(cause)
				case "timeout":
					<-got.Done()
					return partial, cause
				case "cancel":
					cancel()
					return partial, got.Err()
				case "error":
					return partial, cause
				case "reported-error":
					return partial, nil
				}
				return toolapi.Result{Output: "ok"}, nil
			}}
			var mapPanic func(any) error
			if mode == "mapped-panic" || mode == "mapper-success" {
				mapPanic = func(value any) error {
					mapped++
					if value != cause || captured.Err() != nil {
						t.Error("panic classified after cleanup")
					}
					return mappedCause
				}
			}
			outcome := toolpipeline.Execute(ctx, tool, input, timeout, mapPanic)
			if calls != 1 || outcome.TimedOut != (mode == "timeout") || captured == nil {
				t.Fatalf("calls=%d outcome=%+v", calls, outcome)
			}
			if _, deadline := captured.Deadline(); deadline != (timeout > 0) {
				t.Errorf("deadline=%t budget=%s", deadline, timeout)
			}
			if timeout > 0 && captured.Err() == nil {
				t.Error("per-call context not released")
			}
			switch mode {
			case "panic":
				if outcome.PanicValue != cause || outcome.Err == nil || outcome.Err.Error() != "tool invocation panicked: backend cause" || !reflect.DeepEqual(outcome.Result, toolapi.Result{}) {
					t.Fatalf("direct panic=%+v", outcome)
				}
			case "mapped-panic":
				if mapped != 1 || outcome.PanicValue != cause || outcome.Err != mappedCause {
					t.Fatalf("loop panic=%+v mapped=%d", outcome, mapped)
				}
			case "timeout", "error":
				if outcome.Err != cause || !reflect.DeepEqual(outcome.Result, partial) || outcome.PanicValue != nil {
					t.Fatalf("backend outcome=%+v", outcome)
				}
			case "cancel":
				if !errors.Is(outcome.Err, context.Canceled) || outcome.PanicValue != nil {
					t.Fatalf("cancel outcome=%+v", outcome)
				}
			case "reported-error":
				if outcome.Err != nil || !reflect.DeepEqual(outcome.Result, partial) {
					t.Fatalf("reported error=%+v", outcome)
				}
			default:
				if mapped != 0 || outcome.Err != nil || outcome.PanicValue != nil || outcome.Result.Output != "ok" {
					t.Fatalf("success=%+v", outcome)
				}
			}
		})
	}
}
