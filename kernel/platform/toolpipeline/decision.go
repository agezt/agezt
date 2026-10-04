// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/policyctx"
)

// Decision is the policy phase outcome, before memo lookup or execution. Context
// retains resolved metadata for direct execution/hook compatibility. A loop may
// keep its existing execution context while consuming the same verdict.
type Decision struct {
	Context context.Context
	Verdict policyapi.PolicyVerdict
}

// DecisionAuditor persists a policy decision in the caller's journal envelope.
// It preserves each caller's actor/correlation stamping and error ownership.
type DecisionAuditor func(llm.ToolCall, policyapi.PolicyVerdict) error

// Decide binds trusted resolved metadata, checks policy and records the decision.
// It cannot execute a tool or consult memo; callers keep those phases separate.
// Both policy and audit callbacks are mandatory, including a caller's explicit
// no-policy/default-allow callback when that is the existing contract.
func Decide(ctx context.Context, call llm.ToolCall, def toolapi.ToolDef, policy policyapi.Policy, audit DecisionAuditor) (Decision, error) {
	d := Decision{Context: policyctx.WithPolicyToolDef(ctx, def)}
	d.Verdict = policy(d.Context, call)
	if err := audit(call, d.Verdict); err != nil {
		return d, err
	}
	return d, nil
}
