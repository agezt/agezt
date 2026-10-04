// SPDX-License-Identifier: MIT

package toolpipeline_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/policyctx"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

func TestDecidePreservesPolicyAndAuditBoundary(t *testing.T) {
	for _, failAudit := range []bool{false, true} {
		t.Run(map[bool]string{false: "allow", true: "audit-failure"}[failAudit], func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			parent = toolapi.WithCorrelation(parent, "corr")
			parent = policyctx.WithPolicyToolDef(parent, toolapi.ToolDef{Name: "spoof"})
			taint := policyapi.UntrustedObservationTaint{Sources: []string{"source"}, DirectiveLike: true, Matches: []string{"directive"}}
			parent = policyctx.WithUntrustedObservationTaint(parent, taint)
			call := llm.ToolCall{ID: "call", Name: "probe", Input: json.RawMessage(`{"x":1}`)}
			def := toolapi.ToolDef{Name: "probe", Capability: toolapi.ToolCapability{Name: "introspect"}}
			verdict := policyapi.PolicyVerdict{Allow: true, Capability: "introspect", Reason: "decision", AffectedResources: []string{"resource"}, EpistemicConfidence: 0.75}
			cause := errors.New("audit cause")
			var order []string
			d, err := toolpipeline.Decide(parent, call, def, func(ctx context.Context, got llm.ToolCall) policyapi.PolicyVerdict {
				order = append(order, "policy")
				resolved, ok := policyctx.PolicyToolDefFromContext(ctx)
				if !ok || !reflect.DeepEqual(resolved, def) || !reflect.DeepEqual(got, call) || toolapi.CorrelationFromContext(ctx) != "corr" {
					t.Errorf("policy context/call lost: def=%+v call=%+v", resolved, got)
				}
				gotTaint, ok := policyctx.UntrustedObservationTaintFromContext(ctx)
				if !ok || !reflect.DeepEqual(gotTaint, taint) {
					t.Errorf("taint=%+v", gotTaint)
				}
				return verdict
			}, func(got llm.ToolCall, v policyapi.PolicyVerdict) error {
				order = append(order, "audit")
				if !reflect.DeepEqual(got, call) || !reflect.DeepEqual(v, verdict) {
					t.Errorf("audit inputs: call=%+v verdict=%+v", got, v)
				}
				if failAudit {
					return cause
				}
				return nil
			})
			if failAudit {
				if !errors.Is(err, cause) {
					t.Errorf("audit cause=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(order, []string{"policy", "audit"}) || !reflect.DeepEqual(d.Verdict, verdict) {
				t.Errorf("order=%v decision=%+v", order, d)
			}
			if got, ok := policyctx.PolicyToolDefFromContext(d.Context); !ok || !reflect.DeepEqual(got, def) {
				t.Errorf("returned execution context metadata=%+v", got)
			}
			cancel()
			if d.Context.Err() != context.Canceled {
				t.Error("parent cancellation lost")
			}
		})
	}
}
