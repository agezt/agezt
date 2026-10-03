// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolexec"
)

// compile-time check: Kernel satisfies the toolexec dependency interfaces.
var _ toolexec.ToolLookup = (*Kernel)(nil)
var _ toolexec.PolicyChecker = (*Kernel)(nil)
var _ toolexec.EventPublisher = (*Kernel)(nil)
var _ toolexec.NoiseNotifier = (*Kernel)(nil)

// LookupTool implements toolexec.ToolLookup.
func (k *Kernel) LookupTool(name string) (toolapi.Tool, bool) {
	t, ok := k.mergeMCPTools(k.mergeScriptTools(k.tools))[name]
	return t, ok
}

// CheckPolicy implements toolexec.PolicyChecker.
func (k *Kernel) CheckPolicy(ctx context.Context, tc llm.ToolCall) agent.PolicyVerdict {
	return k.policyHook(ctx, tc)
}

// PublishEvent implements toolexec.EventPublisher.
func (k *Kernel) PublishEvent(spec event.Spec) error {
	_, err := k.bus.Publish(spec)
	return err
}

// NotifyNoise implements toolexec.NoiseNotifier.
func (k *Kernel) NotifyNoise(ctx context.Context, tc llm.ToolCall, res toolapi.Result) {
	k.completeAgentNoiseNotify(ctx, tc, res)
}

// RunTool executes one registered in-process tool under the same schema and
// policy gate used by agent/workflow tool calls, then journals tool.invoked and
// tool.result under corr. The implementation is delegated through the injected invocation port.
func (k *Kernel) RunTool(ctx context.Context, corr, callID, toolName string, args json.RawMessage) (toolapi.Result, error) {
	return k.runToolWithLookup(ctx, corr, callID, toolName, args, k)
}

// runToolWithLookup also admits invocation-local adapters, without modifying
// the registered tool map or bypassing the shared policy/audit machinery.
func (k *Kernel) runToolWithLookup(ctx context.Context, corr, callID, toolName string, args json.RawMessage, lookup toolexec.ToolLookup) (toolapi.Result, error) {
	actor := actorFromCtx(ctx)
	if actor == "" {
		actor = toolapi.AgentFromContext(ctx)
	}
	if actor == "" {
		actor = "tool"
	}
	ctx = k.WithActorCorrelation(ctx, actor, corr)
	cfg := k.effectiveConfig(ctx)
	return k.toolInvoker.Invoke(ctx, toolapi.Invocation{
		CorrelationID: corr, CallID: callID, Name: toolName, Input: args, Lookup: lookup,
		Artifacts: k.artifacts, ArtifactThreshold: cfg.ArtifactThreshold,
	})
}
