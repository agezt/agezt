// SPDX-License-Identifier: MIT

package toolexec_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/policyctx"
	"github.com/agezt/agezt/kernel/toolexec"
)

type preflightPolicy struct {
	calls       int
	allow       bool
	call        llm.ToolCall
	definition  toolapi.ToolDef
	taint       policyapi.UntrustedObservationTaint
	corr, actor string
}

func (p *preflightPolicy) CheckPolicy(ctx context.Context, tc llm.ToolCall) policyapi.PolicyVerdict {
	p.calls++
	p.call = tc
	p.definition, _ = policyctx.PolicyToolDefFromContext(ctx)
	p.taint, _ = policyctx.UntrustedObservationTaintFromContext(ctx)
	p.corr = toolapi.CorrelationFromContext(ctx)
	p.actor = toolapi.AgentFromContext(ctx)
	return policyapi.PolicyVerdict{Allow: p.allow, Capability: "introspect", Reason: "preflight decision"}
}

func TestRun_SharedPreflightBoundary(t *testing.T) {
	for _, options := range []bool{false, true} {
		path := "legacy"
		if options {
			path = "options"
		}
		for _, tc := range []struct {
			name, tool, input, errorText                string
			allow                                       bool
			wantPolicy, wantTool, wantEvents, wantNoise int
		}{
			{"unknown", "missing", `{}`, `unknown tool "missing"`, true, 0, 0, 0, 0},
			{"invalid-json", "probe", `{`, "input rejected by schema: input is not valid JSON", true, 0, 0, 0, 0},
			{"missing-required", "probe", `{}`, "input rejected by schema: $.target is required", true, 0, 0, 0, 0},
			{"wrong-type", "probe", `{"target":1}`, "input rejected by schema: $.target has wrong type", true, 0, 0, 0, 0},
			{"allow", "probe", `{"target":"value"}`, "", true, 1, 1, 3, 1},
			{"deny", "probe", `{"target":"value"}`, "refused: preflight decision", false, 1, 0, 2, 0},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				calls := 0
				def := toolapi.ToolDef{Name: "probe", InputSchema: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}},"required":["target"]}`), Capability: toolapi.ToolCapability{Name: "introspect"}}
				tool := &fakeTool{def: def, invoke: func(ctx context.Context, input json.RawMessage) (toolapi.Result, error) {
					calls++
					if toolapi.CorrelationFromContext(ctx) != "corr" || string(input) != tc.input {
						t.Errorf("invoke context/input lost")
					}
					return toolapi.Result{Output: "ok"}, nil
				}}
				policy := &preflightPolicy{allow: tc.allow}
				events := &mockEvents{}
				noise := &mockNoise{}
				taint := agent.UntrustedObservationTaint{Sources: []string{"web:one"}, DirectiveLike: true, Matches: []string{"directive:one"}}
				ctx := toolapi.WithAgent(context.Background(), "profile")
				ctx = agent.WithPolicyToolDef(ctx, toolapi.ToolDef{Name: "caller-spoof", Capability: toolapi.ToolCapability{Name: "provider.call"}})
				ctx = agent.WithUntrustedObservationTaint(ctx, taint)
				var res toolapi.Result
				var err error
				if options {
					res, err = toolexec.RunWithOptions(ctx, "corr", "call", tc.tool, json.RawMessage(tc.input), mockLookup{"probe": tool}, policy, events, noise, toolexec.Options{})
				} else {
					res, err = toolexec.Run(ctx, "corr", "call", tc.tool, json.RawMessage(tc.input), mockLookup{"probe": tool}, policy, events, noise)
				}
				if tc.errorText == "" {
					if err != nil || res.Output != "ok" {
						t.Fatalf("result=%+v error=%v", res, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("error=%v want %q", err, tc.errorText)
				}
				if policy.calls != tc.wantPolicy || calls != tc.wantTool || len(events.published) != tc.wantEvents || noise.calls != tc.wantNoise {
					t.Fatalf("policy=%d tool=%d events=%d noise=%d", policy.calls, calls, len(events.published), noise.calls)
				}
				if policy.calls > 0 {
					if !reflect.DeepEqual(policy.definition, def) || !reflect.DeepEqual(policy.taint, taint) || policy.corr != "corr" || policy.actor != "profile" || policy.call.ID != "call" || policy.call.Name != "probe" || string(policy.call.Input) != tc.input {
						t.Errorf("resolved policy inputs=%+v", policy)
					}
				}
			})
		}
	}
}
