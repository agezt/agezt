// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolexec"
)

func TestRun_InvocationAdmissionContract(t *testing.T) {
	for _, mode := range []string{"allow", "deny", "invoked-audit-failure"} {
		t.Run(mode, func(t *testing.T) {
			cause := errors.New("invocation audit unavailable")
			events := &mockEvents{}
			if mode == "invoked-audit-failure" {
				events.failKind, events.failure = event.KindToolInvoked, cause
			}
			policy := &preflightPolicy{allow: mode != "deny"}
			noise := &mockNoise{}
			input := json.RawMessage(" {\n \"n\": [1, 2] } ")
			calls := 0
			tool := &fakeTool{def: toolapi.ToolDef{Name: "implementation", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)}, invoke: func(context.Context, json.RawMessage) (toolapi.Result, error) {
				calls++
				if len(events.published) != 2 || events.published[0].Kind != event.KindPolicyDecision || events.published[1].Kind != event.KindToolInvoked {
					t.Errorf("effects before admission: %+v", events.published)
				}
				return toolapi.Result{Output: "ok"}, nil
			}}
			res, err := toolexec.Run(context.Background(), "corr", "call", "probe", input, mockLookup{"probe": tool}, policy, events, noise)
			if mode == "invoked-audit-failure" {
				if err != cause || !errors.Is(err, cause) || calls != 0 || noise.calls != 0 || len(events.published) != 1 || !reflect.DeepEqual(res, toolapi.Result{}) {
					t.Fatalf("error=%v calls=%d hooks=%d records=%d result=%+v", err, calls, noise.calls, len(events.published), res)
				}
			} else if mode == "deny" {
				if err == nil || calls != 0 || noise.calls != 0 || len(events.published) != 2 || events.published[1].Kind != event.KindToolResult {
					t.Fatalf("deny error=%v calls=%d hooks=%d records=%v", err, calls, noise.calls, events.published)
				}
			} else {
				if err != nil || calls != 1 || noise.calls != 1 || res.Output != "ok" || len(events.published) != 3 {
					t.Fatalf("error=%v calls=%d hooks=%d records=%d", err, calls, noise.calls, len(events.published))
				}
				e := events.published[1]
				if e.Subject != "tool" || e.Actor != "tool" || e.CorrelationID != "corr" || !reflect.DeepEqual(e.Payload, map[string]any{"tool": "probe", "call_id": "call", "input": input}) {
					t.Errorf("invocation record=%+v", e)
				}
			}
			if policy.calls != 1 {
				t.Errorf("policy calls=%d", policy.calls)
			}
		})
	}
}
