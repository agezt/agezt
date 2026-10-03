// SPDX-License-Identifier: MIT

// Package toolexec provides the shared execution service used by direct
// operator/CLI calls and registered workflow tool nodes. It narrows the
// composition root's responsibility and to make tool-execution behaviour
// independently testable.
package toolexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/toolinvoke"
)

// ToolLookup is the interface for resolving tool names to their implementations.
type ToolLookup interface {
	LookupTool(name string) (toolapi.Tool, bool)
}

// PolicyChecker is the interface for gating tool invocations.
type PolicyChecker interface {
	CheckPolicy(ctx context.Context, tc llm.ToolCall) agent.PolicyVerdict
}

// EventPublisher is the interface for emitting tool and policy events.
type EventPublisher interface {
	PublishEvent(spec event.Spec) error
}

// NoiseNotifier is the interface for agent-noise completion callbacks.
type NoiseNotifier interface {
	NotifyNoise(ctx context.Context, tc llm.ToolCall, res toolapi.Result)
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
) (toolapi.Result, error) {
	tool, ok := tools.LookupTool(toolName)
	if !ok {
		return toolapi.Result{}, fmt.Errorf("unknown tool %q", toolName)
	}
	def := tool.Definition()
	if err := agent.ValidateToolInput(def, args); err != nil {
		return toolapi.Result{}, fmt.Errorf("tool %s input rejected by schema: %w", toolName, err)
	}
	ctx = toolapi.WithCorrelation(ctx, corr)
	ctx = agent.WithPolicyToolDef(ctx, def)
	verdict := policy.CheckPolicy(ctx, llm.ToolCall{ID: callID, Name: toolName, Input: args})
	// Journal the gating decision for the direct (operator/CLI) tool path too, so it
	// is audited exactly like a loop tool call (kernel/agent publishes the same
	// policy.decision for in-loop calls). Without this, a refused direct tool run
	// left no journal trace and never folded into the per-agent denial audit.
	if err := events.PublishEvent(event.Spec{
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
	}); err != nil {
		return toolapi.Result{}, err
	}
	if !verdict.Allow {
		reason := verdict.Reason
		if reason == "" {
			reason = "denied by policy"
		}
		res := toolapi.Result{Output: "tool call denied by policy: " + reason, IsError: true}
		refusal := fmt.Errorf("tool %s refused: %s", toolName, reason)
		if err := events.PublishEvent(event.Spec{
			Subject: "tool", Kind: event.KindToolResult, Actor: "tool", CorrelationID: corr,
			Payload: map[string]any{"tool": toolName, "call_id": callID, "output": res.Output, "error": true},
		}); err != nil {
			return res, errors.Join(refusal, err)
		}
		return res, refusal
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
		return toolapi.Result{}, err
	}
	res, err := invokeSafely(ctx, tool, args)
	if err != nil {
		auditErr := events.PublishEvent(event.Spec{
			Subject:       "tool",
			Kind:          event.KindToolResult,
			Actor:         "tool",
			CorrelationID: corr,
			Payload:       map[string]any{"tool": toolName, "call_id": callID, "output": err.Error(), "error": true},
		})
		noise.NotifyNoise(ctx, llm.ToolCall{ID: callID, Name: toolName, Input: args}, toolapi.Result{Output: err.Error(), IsError: true})
		if auditErr != nil {
			err = errors.Join(err, auditErr)
		}
		return toolapi.Result{}, err
	}
	if err := events.PublishEvent(event.Spec{
		Subject:       "tool",
		Kind:          event.KindToolResult,
		Actor:         "tool",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": toolName, "call_id": callID, "output": res.Output, "error": res.IsError},
	}); err != nil {
		return toolapi.Result{}, err
	}
	noise.NotifyNoise(ctx, llm.ToolCall{ID: callID, Name: toolName, Input: args}, res)
	return res, nil
}

// A faulty tool must still produce its terminal audit record and cannot take
// down a workflow or the direct-tool caller.
func invokeSafely(ctx context.Context, tool toolapi.Tool, args json.RawMessage) (toolapi.Result, error) {
	res, err, _ := toolinvoke.Invoke(ctx, tool, args)
	return res, err
}
