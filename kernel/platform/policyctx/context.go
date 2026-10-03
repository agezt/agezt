// SPDX-License-Identifier: MIT

// Package policyctx carries resolved tool metadata and observation provenance
// between admission and the policy decision.
package policyctx

// Policy-hook context: carried from the loop to the policy decision for the
// call being gated. Tool-facing invocation context (correlation, acting agent,
// workdir) lives in kernel/contract/toolapi.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

type policyToolDefKey struct{}
type untrustedObservationTaintKey struct{}

// WithPolicyToolDef carries the already-resolved ToolDef for the tool call
// currently being gated. It lets runtime policy inspect dynamic MCP/forge tool
// metadata without giving the model authority to mutate that metadata.
func WithPolicyToolDef(ctx context.Context, def toolapi.ToolDef) context.Context {
	return context.WithValue(ctx, policyToolDefKey{}, def)
}

// PolicyToolDefFromContext returns the ToolDef attached by the agent loop
// before invoking the policy hook.
func PolicyToolDefFromContext(ctx context.Context) (toolapi.ToolDef, bool) {
	if ctx == nil {
		return toolapi.ToolDef{}, false
	}
	def, ok := ctx.Value(policyToolDefKey{}).(toolapi.ToolDef)
	return def, ok
}

// WithUntrustedObservationTaint attaches the current run's external-observation
// taint to a policy context. Empty taints leave ctx unchanged.
func WithUntrustedObservationTaint(ctx context.Context, t policyapi.UntrustedObservationTaint) context.Context {
	if len(t.Sources) == 0 && len(t.Matches) == 0 && !t.DirectiveLike {
		return ctx
	}
	return context.WithValue(ctx, untrustedObservationTaintKey{}, t)
}

// UntrustedObservationTaintFromContext returns the taint attached to a policy
// context, if any.
func UntrustedObservationTaintFromContext(ctx context.Context) (policyapi.UntrustedObservationTaint, bool) {
	if ctx == nil {
		return policyapi.UntrustedObservationTaint{}, false
	}
	t, ok := ctx.Value(untrustedObservationTaintKey{}).(policyapi.UntrustedObservationTaint)
	return t, ok
}
