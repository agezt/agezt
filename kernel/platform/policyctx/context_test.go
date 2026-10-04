// SPDX-License-Identifier: MIT

package policyctx_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/policyctx"
)

var _ agent.UntrustedObservationTaint = policyapi.UntrustedObservationTaint{}

func TestPolicyContextCrossesCompatibilityBoundary(t *testing.T) {
	for _, useAgentSetter := range []bool{false, true} {
		name := "platform-to-agent"
		if useAgentSetter {
			name = "agent-to-platform"
		}
		t.Run(name, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			parent = toolapi.WithCorrelation(parent, "corr")
			def := toolapi.ToolDef{Name: "dynamic", Capability: toolapi.ToolCapability{Name: "file.write"}}
			taint := policyapi.UntrustedObservationTaint{Sources: []string{"web:one"}, DirectiveLike: true, Matches: []string{"directive:one"}}
			var ctx context.Context
			if useAgentSetter {
				ctx = policyctx.WithPolicyToolDef(parent, def)
				ctx = agent.WithUntrustedObservationTaint(ctx, taint)
			} else {
				ctx = policyctx.WithPolicyToolDef(parent, def)
				ctx = policyctx.WithUntrustedObservationTaint(ctx, taint)
			}
			for name, get := range map[string]func(context.Context) (toolapi.ToolDef, bool){"agent": agent.PolicyToolDefFromContext, "platform": policyctx.PolicyToolDefFromContext} {
				got, ok := get(ctx)
				if !ok || !reflect.DeepEqual(got, def) {
					t.Errorf("%s def=%+v present=%v", name, got, ok)
				}
			}
			for name, get := range map[string]func(context.Context) (policyapi.UntrustedObservationTaint, bool){"agent": agent.UntrustedObservationTaintFromContext, "platform": policyctx.UntrustedObservationTaintFromContext} {
				got, ok := get(ctx)
				if !ok || !reflect.DeepEqual(got, taint) {
					t.Errorf("%s taint=%+v present=%v", name, got, ok)
				}
			}
			if toolapi.CorrelationFromContext(ctx) != "corr" {
				t.Fatal("parent correlation lost")
			}
			cancel()
			if ctx.Err() != context.Canceled {
				t.Fatal("parent cancellation lost")
			}
		})
	}
}

func TestPolicyContextEmptyTaintDoesNotReplaceParent(t *testing.T) {
	parent := policyctx.WithUntrustedObservationTaint(context.Background(), policyapi.UntrustedObservationTaint{Sources: []string{"prior"}})
	if got := policyctx.WithUntrustedObservationTaint(parent, policyapi.UntrustedObservationTaint{}); got != parent {
		t.Fatal("empty taint replaced parent")
	}
	if got := agent.WithUntrustedObservationTaint(parent, agent.UntrustedObservationTaint{}); got != parent {
		t.Fatal("empty compatibility taint replaced parent")
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if _, ok := policyctx.PolicyToolDefFromContext(ctx); ok {
			t.Fatal("invented tool metadata")
		}
		if _, ok := policyctx.UntrustedObservationTaintFromContext(ctx); ok {
			t.Fatal("invented taint")
		}
		if _, ok := agent.PolicyToolDefFromContext(ctx); ok {
			t.Fatal("compatibility invented tool metadata")
		}
		if _, ok := agent.UntrustedObservationTaintFromContext(ctx); ok {
			t.Fatal("compatibility invented taint")
		}
	}
}
