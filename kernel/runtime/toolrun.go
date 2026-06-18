// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// RunTool executes one registered in-process tool under the same schema and
// policy gate used by agent/workflow tool calls, then journals tool.invoked and
// tool.result under corr.
func (k *Kernel) RunTool(ctx context.Context, corr, callID, toolName string, args json.RawMessage) (agent.Result, error) {
	tool, ok := k.tools[toolName]
	if !ok {
		return agent.Result{}, fmt.Errorf("unknown tool %q", toolName)
	}
	if err := agent.ValidateToolInput(tool.Definition(), args); err != nil {
		return agent.Result{}, fmt.Errorf("tool %s input rejected by schema: %w", toolName, err)
	}
	verdict := k.policyHook(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args})
	// Journal the gating decision for the direct (operator/CLI) tool path too, so it
	// is audited exactly like a loop tool call (kernel/agent publishes the same
	// policy.decision for in-loop calls). Without this, a refused direct tool run
	// left no journal trace and never folded into the per-agent denial audit.
	_, _ = k.bus.Publish(event.Spec{
		Subject:       "policy",
		Kind:          event.KindPolicyDecision,
		Actor:         "policy",
		CorrelationID: corr,
		Payload: map[string]any{
			"tool":         toolName,
			"call_id":      callID,
			"capability":   verdict.Capability,
			"allow":        verdict.Allow,
			"reason":       verdict.Reason,
			"would_ask":    verdict.WouldAsk,
			"hard_denied":  verdict.HardDenied,
			"effect_class": verdict.EffectClass,
		},
	})
	if !verdict.Allow {
		reason := verdict.Reason
		if reason == "" {
			reason = "denied by policy"
		}
		return agent.Result{}, fmt.Errorf("tool %s refused: %s", toolName, reason)
	}
	if _, err := k.bus.Publish(event.Spec{
		Subject:       "tool",
		Kind:          event.KindToolInvoked,
		Actor:         "tool",
		CorrelationID: corr,
		Payload: map[string]any{
			"tool":    toolName,
			"call_id": callID,
			"input":   args,
		},
	}); err != nil {
		return agent.Result{}, err
	}
	res, err := tool.Invoke(agent.WithCorrelation(ctx, corr), args)
	if err != nil {
		_, _ = k.bus.Publish(event.Spec{
			Subject:       "tool",
			Kind:          event.KindToolResult,
			Actor:         "tool",
			CorrelationID: corr,
			Payload:       map[string]any{"tool": toolName, "call_id": callID, "output": err.Error(), "error": true},
		})
		k.completeAgentNoiseNotify(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, agent.Result{Output: err.Error(), IsError: true})
		return agent.Result{}, err
	}
	if _, err := k.bus.Publish(event.Spec{
		Subject:       "tool",
		Kind:          event.KindToolResult,
		Actor:         "tool",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": toolName, "call_id": callID, "output": res.Output, "error": res.IsError},
	}); err != nil {
		return agent.Result{}, err
	}
	k.completeAgentNoiseNotify(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, res)
	return res, nil
}
