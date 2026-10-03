// SPDX-License-Identifier: MIT

package toolinvoke_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/toolinvoke"
)

type probeTool struct {
	call func(context.Context, json.RawMessage) (toolapi.Result, error)
}

func (probeTool) Definition() toolapi.ToolDef { panic("admission belongs to the caller") }
func (p probeTool) Invoke(ctx context.Context, in json.RawMessage) (toolapi.Result, error) {
	return p.call(ctx, in)
}

func TestInvokePreservesCallContract(t *testing.T) {
	for _, mode := range []string{"success", "reported-error", "error", "cancel", "panic"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = toolapi.WithCorrelation(ctx, "run-corr")
			input := json.RawMessage(`{"n":42}`)
			calls := 0
			cause := errors.New("backend unavailable")
			tool := probeTool{call: func(got context.Context, raw json.RawMessage) (toolapi.Result, error) {
				calls++
				if got != ctx || toolapi.CorrelationFromContext(got) != "run-corr" || string(raw) != string(input) {
					t.Error("context/input changed")
				}
				switch mode {
				case "panic":
					panic(cause)
				case "cancel":
					cancel()
					return toolapi.Result{}, got.Err()
				case "error":
					return toolapi.Result{Output: "partial"}, cause
				case "reported-error":
					return toolapi.Result{Output: "failure", IsError: true}, nil
				}
				return toolapi.Result{Output: "done"}, nil
			}}
			res, err, panicked := toolinvoke.Invoke(ctx, tool, input)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			switch mode {
			case "panic":
				if panicked != cause || err == nil || err.Error() != "tool invocation panicked: backend unavailable" || res.Output != "" {
					t.Fatalf("panic result=%+v error=%v value=%v", res, err, panicked)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) || panicked != nil {
					t.Fatalf("cancel error=%v panic=%v", err, panicked)
				}
			case "error":
				if err != cause || res.Output != "partial" || panicked != nil {
					t.Fatalf("error result=%+v error=%v panic=%v", res, err, panicked)
				}
			case "reported-error":
				if err != nil || !res.IsError || res.Output != "failure" || panicked != nil {
					t.Fatalf("reported result=%+v error=%v", res, err)
				}
			default:
				if err != nil || res.Output != "done" || panicked != nil {
					t.Fatalf("success result=%+v error=%v", res, err)
				}
			}
		})
	}
}
