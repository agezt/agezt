// SPDX-License-Identifier: MIT

package agent

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/platform/policyctx"
)

// WithPolicyToolDef forwards the resolved tool metadata context contract.
func WithPolicyToolDef(ctx context.Context, def ToolDef) context.Context {
	return policyctx.WithPolicyToolDef(ctx, def)
}

// PolicyToolDefFromContext forwards the resolved tool metadata context contract.
func PolicyToolDefFromContext(ctx context.Context) (ToolDef, bool) {
	return policyctx.PolicyToolDefFromContext(ctx)
}

// UntrustedObservationTaint retains the observation provenance type identity.
type UntrustedObservationTaint = policyapi.UntrustedObservationTaint

// WithUntrustedObservationTaint forwards the observation provenance contract.
func WithUntrustedObservationTaint(ctx context.Context, t UntrustedObservationTaint) context.Context {
	return policyctx.WithUntrustedObservationTaint(ctx, t)
}

// UntrustedObservationTaintFromContext forwards the observation provenance contract.
func UntrustedObservationTaintFromContext(ctx context.Context) (UntrustedObservationTaint, bool) {
	return policyctx.UntrustedObservationTaintFromContext(ctx)
}
