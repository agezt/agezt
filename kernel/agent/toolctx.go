// SPDX-License-Identifier: MIT

package agent

// Policy-hook context: carried from the loop to the policy decision for the
// call being gated. Tool-facing invocation context (correlation, acting agent,
// workdir) lives in kernel/contract/toolapi.

import "context"

type policyToolDefKey struct{}
type untrustedObservationTaintKey struct{}

// WithPolicyToolDef carries the already-resolved ToolDef for the tool call
// currently being gated. It lets runtime policy inspect dynamic MCP/forge tool
// metadata without giving the model authority to mutate that metadata.
func WithPolicyToolDef(ctx context.Context, def ToolDef) context.Context {
	return context.WithValue(ctx, policyToolDefKey{}, def)
}

// PolicyToolDefFromContext returns the ToolDef attached by the agent loop
// before invoking the policy hook.
func PolicyToolDefFromContext(ctx context.Context) (ToolDef, bool) {
	if ctx == nil {
		return ToolDef{}, false
	}
	def, ok := ctx.Value(policyToolDefKey{}).(ToolDef)
	return def, ok
}

// UntrustedObservationTaint is carried from the tool-output boundary to the
// next policy decision. It lets policy see that a proposed action is downstream
// of external data without asking the LLM to self-report that dependency.
type UntrustedObservationTaint struct {
	Sources       []string
	DirectiveLike bool
	Matches       []string
}

// WithUntrustedObservationTaint attaches the current run's external-observation
// taint to a policy context. Empty taints leave ctx unchanged.
func WithUntrustedObservationTaint(ctx context.Context, t UntrustedObservationTaint) context.Context {
	if len(t.Sources) == 0 && len(t.Matches) == 0 && !t.DirectiveLike {
		return ctx
	}
	return context.WithValue(ctx, untrustedObservationTaintKey{}, t)
}

// UntrustedObservationTaintFromContext returns the taint attached to a policy
// context, if any.
func UntrustedObservationTaintFromContext(ctx context.Context) (UntrustedObservationTaint, bool) {
	if ctx == nil {
		return UntrustedObservationTaint{}, false
	}
	t, ok := ctx.Value(untrustedObservationTaintKey{}).(UntrustedObservationTaint)
	return t, ok
}
