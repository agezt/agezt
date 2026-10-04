// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/toolexec"
)

func TestRun_ExecutionRetainsCallerBudget(t *testing.T) {
	for _, withDeadline := range []bool{false, true} {
		parent, cancel := context.WithCancel(context.Background())
		if withDeadline {
			cancel()
			parent, cancel = context.WithTimeout(context.Background(), time.Minute)
		}
		func() {
			defer cancel()
			var captured context.Context
			calls := 0
			tool := &fakeTool{def: toolapi.ToolDef{Name: "probe"}, invoke: func(ctx context.Context, _ json.RawMessage) (toolapi.Result, error) {
				calls++
				captured = ctx
				want, wantOK := parent.Deadline()
				got, gotOK := ctx.Deadline()
				if gotOK != wantOK || !got.Equal(want) || ctx.Done() != parent.Done() {
					t.Error("direct execution changed caller budget")
				}
				return toolapi.Result{Output: "ok"}, nil
			}}
			_, err := toolexec.Run(parent, "corr", "call", "probe", json.RawMessage(`{}`), mockLookup{"probe": tool}, &preflightPolicy{allow: true}, &mockEvents{}, &mockNoise{})
			if err != nil || calls != 1 || captured == nil || captured.Err() != nil {
				t.Fatalf("error=%v calls=%d execution context=%v", err, calls, captured)
			}
		}()
	}
}
