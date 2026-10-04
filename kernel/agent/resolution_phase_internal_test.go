// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
)

type resolutionProbe struct {
	def                ToolDef
	definitions, calls int
}

func (p *resolutionProbe) Definition() ToolDef { p.definitions++; return p.def }
func (p *resolutionProbe) Invoke(context.Context, json.RawMessage) (Result, error) {
	p.calls++
	return Result{Output: "ok"}, nil
}

func TestGateResolutionPrecedesGuardAndPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, tool, input, rejection string
		guarded                      bool
	}{
		{"unknown", "missing", `{`, `tool "missing" is not available`, false},
		{"invalid-json", "probe", `{`, "input is not valid JSON", false},
		{"required", "probe", `{}`, "$.n is required", false},
		{"wrong-type", "probe", `{"n":"one"}`, "$.n has wrong type", false},
		{"extra-field", "probe", `{"n":1,"extra":true}`, "$.extra is not allowed", false},
		{"guard", "probe", `{"n":1}`, "loop guard:", true},
		{"allow", "probe", `{"n":1}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := ToolDef{Name: "implementation", Description: "registry alias", InputSchema: json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`), Capability: ToolCapability{Name: "introspect"}, Effect: ToolEffect{Class: EffectReadOnly}}
			tool := &resolutionProbe{def: def}
			policyCalls := 0
			call := ToolCall{ID: "call", Name: tc.tool, Input: json.RawMessage(tc.input)}
			s := testState(LoopConfig{Tools: map[string]Tool{"probe": tool}, MaxIdenticalToolCalls: 1, Policy: func(ctx context.Context, got ToolCall) PolicyVerdict {
				policyCalls++
				resolved, ok := PolicyToolDefFromContext(ctx)
				if !ok || !reflect.DeepEqual(resolved, def) || !reflect.DeepEqual(got, call) {
					t.Errorf("policy metadata/call: def=%+v call=%+v", resolved, got)
				}
				return PolicyVerdict{Allow: true}
			}})
			key := call.Name + "\x00" + string(call.Input)
			if tc.guarded {
				s.callCounts[key] = 1
			}
			var records []event.Kind
			s.publish = func(kind event.Kind, _ string, _ any) (*event.Event, error) {
				records = append(records, kind)
				return nil, nil
			}
			jobs, err := s.gateToolCalls(context.Background(), []ToolCall{call}, 0)
			if err != nil || len(jobs) != 1 || tool.calls != 0 {
				t.Fatalf("jobs=%v error=%v effects=%d", jobs, err, tool.calls)
			}
			wantDefs := 1
			if tc.tool == "missing" {
				wantDefs = 0
			}
			if tool.definitions != wantDefs {
				t.Fatalf("definitions=%d want=%d", tool.definitions, wantDefs)
			}
			if tc.rejection == "" {
				if jobs[0].tool != tool || policyCalls != 1 || !reflect.DeepEqual(records, []event.Kind{event.KindPolicyDecision, event.KindToolInvoked}) {
					t.Fatalf("admission=%+v policy=%d records=%v", jobs[0], policyCalls, records)
				}
			} else if jobs[0].tool != nil || !jobs[0].result.IsError || !strings.Contains(jobs[0].result.Output, tc.rejection) || policyCalls != 0 || len(records) != 0 {
				t.Fatalf("rejection=%+v policy=%d records=%v", jobs[0], policyCalls, records)
			}
			wantCount := 0
			if tc.guarded {
				wantCount = 2
			} else if tc.rejection == "" {
				wantCount = 1
			}
			if s.callCounts[key] != wantCount {
				t.Errorf("schema/availability consumed guard quota: counts=%v want=%d", s.callCounts, wantCount)
			}
		})
	}
}
