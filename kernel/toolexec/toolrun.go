// SPDX-License-Identifier: MIT

// Package toolexec provides the direct tool execution service used by the
// operator/CLI tool path. It is extracted from kernel/runtime to narrow the
// composition root's responsibility and to make tool-execution behaviour
// independently testable.
package toolexec

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

// ToolLookup is the interface for resolving tool names to their implementations.
type ToolLookup interface {
	LookupTool(name string) (agent.Tool, bool)
}

// PolicyChecker is the interface for gating tool invocations.
type PolicyChecker interface {
	CheckPolicy(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict
}

// EventPublisher is the interface for emitting tool and policy events.
type EventPublisher interface {
	PublishEvent(spec event.Spec) error
}

// NoiseNotifier is the interface for agent-noise completion callbacks.
type NoiseNotifier interface {
	NotifyNoise(ctx context.Context, tc agent.ToolCall, res agent.Result)
}

// Run executes one registered in-process tool under the same schema and
// policy gate used by agent/workflow tool calls, then journals tool.invoked and
// tool.result under corr.
//
// All injected interfaces are expected to be safe for concurrent use (the
// Kernel's fields are read-only after Open or are concurrency-safe).
func Run(
	ctx context.Context,
	corr, callID, toolName string,
	args json.RawMessage,
	tools ToolLookup,
	policy PolicyChecker,
	events EventPublisher,
	noise NoiseNotifier,
) (agent.Result, error) {
	tool, ok := tools.LookupTool(toolName)
	if !ok {
		return agent.Result{}, fmt.Errorf("unknown tool %q", toolName)
	}
	if err := agent.ValidateToolInput(tool.Definition(), args); err != nil {
		return agent.Result{}, fmt.Errorf("tool %s input rejected by schema: %w", toolName, err)
	}
	verdict := policy.CheckPolicy(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args})
	// Journal the gating decision for the direct (operator/CLI) tool path too, so it
	// is audited exactly like a loop tool call (kernel/agent publishes the same
	// policy.decision for in-loop calls). Without this, a refused direct tool run
	// left no journal trace and never folded into the per-agent denial audit.
	_ = events.PublishEvent(event.Spec{
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
	if err := events.PublishEvent(event.Spec{
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
		_ = events.PublishEvent(event.Spec{
			Subject:       "tool",
			Kind:          event.KindToolResult,
			Actor:         "tool",
			CorrelationID: corr,
			Payload:       map[string]any{"tool": toolName, "call_id": callID, "output": err.Error(), "error": true},
		})
		noise.NotifyNoise(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, agent.Result{Output: err.Error(), IsError: true})
		return agent.Result{}, err
	}
	if err := events.PublishEvent(event.Spec{
		Subject:       "tool",
		Kind:          event.KindToolResult,
		Actor:         "tool",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": toolName, "call_id": callID, "output": res.Output, "error": res.IsError},
	}); err != nil {
		return agent.Result{}, err
	}
	noise.NotifyNoise(ctx, agent.ToolCall{ID: callID, Name: toolName, Input: args}, res)
	return res, nil
}
