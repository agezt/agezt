// SPDX-License-Identifier: MIT

// Package toolpipeline supplies the generic governed tool-call mechanism.
// Hosts inject policy, audit and completion ports; business policy stays outside.
package toolpipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/toolaudit"
	"github.com/agezt/agezt/kernel/platform/tooloutput"
)

// ToolLookup is the interface for resolving tool names to their implementations.
type ToolLookup = toolapi.ToolLookup

// PolicyChecker is the interface for gating tool invocations.
type PolicyChecker interface {
	CheckPolicy(ctx context.Context, tc llm.ToolCall) policyapi.PolicyVerdict
}

// EventPublisher is the interface for emitting tool and policy events.
type EventPublisher interface {
	PublishEvent(spec event.Spec) error
}

// NoiseNotifier is the interface for agent-noise completion callbacks.
type NoiseNotifier interface {
	NotifyNoise(ctx context.Context, tc llm.ToolCall, res toolapi.Result)
}

// Options configures audit-only output representation. Callers and completion
// hooks still receive the full output; nil stores retain legacy inline behavior.
type Options struct {
	Artifacts         tooloutput.ArtifactPutter
	ArtifactThreshold int
	// Phases selects an injected service; nil retains the legacy entry path.
	Phases toolphaseapi.Phases
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
	service := &invoker{deps: Dependencies{Tools: tools, Policy: policy, Events: events, Noise: noise}}
	return run(ctx, corr, callID, toolName, args, tools, policy, events, noise, service)
}

func run(ctx context.Context, corr, callID, toolName string, args json.RawMessage, tools ToolLookup, policy PolicyChecker, events EventPublisher, noise NoiseNotifier, phases toolphaseapi.Phases) (toolapi.Result, error) {

	call := llm.ToolCall{ID: callID, Name: toolName, Input: args}
	resolved := phases.Resolve(call, tools.LookupTool)
	if !resolved.Found {
		return toolapi.Result{}, fmt.Errorf("unknown tool %q", toolName)
	}
	if err := resolved.InputError; err != nil {
		return toolapi.Result{}, fmt.Errorf("tool %s input rejected by schema: %w", toolName, err)
	}
	tool, def := resolved.Tool, resolved.Definition
	ctx = toolapi.WithCorrelation(ctx, corr)
	decision, err := phases.Decide(ctx, call, def, policy.CheckPolicy,
		func(call llm.ToolCall, verdict policyapi.PolicyVerdict) error {
			return events.PublishEvent(event.Spec{
				Subject: "policy", Kind: event.KindPolicyDecision, Actor: "policy", CorrelationID: corr,
				Payload: toolaudit.PolicyDecisionPayload(call, verdict),
			})
		})
	if err != nil {
		return toolapi.Result{}, err
	}
	ctx, verdict := decision.Context, decision.Verdict
	terminalAudit := func(result toolapi.Result) error {
		return phases.Settle(call, result, nil, func(kind string, payload map[string]any) error {
			return events.PublishEvent(event.Spec{
				Subject: "tool", Kind: event.Kind(kind), Actor: "tool", CorrelationID: corr,
				Payload: payload,
			})
		})
	}
	if !verdict.Allow {
		reason := verdict.Reason
		if reason == "" {
			reason = "denied by policy"
		}
		res := toolapi.Result{Output: "tool call denied by policy: " + reason, IsError: true}
		refusal := fmt.Errorf("tool %s refused: %s", toolName, reason)
		if err := terminalAudit(res); err != nil {
			return res, errors.Join(refusal, err)
		}
		return res, refusal
	}
	if err := phases.Announce(call, func(kind string, payload map[string]any) error {
		return events.PublishEvent(event.Spec{
			Subject: "tool", Kind: event.Kind(kind), Actor: "tool", CorrelationID: corr,
			Payload: payload,
		})
	}); err != nil {
		return toolapi.Result{}, err
	}
	execution := phases.Execute(ctx, tool, args, 0, nil)
	res, err := execution.Result, execution.Err
	if err != nil {
		errorResult := toolapi.Result{Output: err.Error(), IsError: true}
		auditErr := terminalAudit(errorResult)
		noise.NotifyNoise(ctx, llm.ToolCall{ID: callID, Name: toolName, Input: args}, errorResult)
		if auditErr != nil {
			err = errors.Join(err, auditErr)
		}
		return toolapi.Result{}, err
	}
	if err := terminalAudit(res); err != nil {
		return toolapi.Result{}, err
	}
	noise.NotifyNoise(ctx, llm.ToolCall{ID: callID, Name: toolName, Input: args}, res)
	return res, nil
}

// RunWithOptions is Run with an optional artifact-backed journal representation.
// The publisher adapter keeps execution, caller/hook output and error ownership
// in the existing Run pipeline, retaining its public compatibility contract.
func RunWithOptions(ctx context.Context, corr, callID, toolName string, args json.RawMessage, tools ToolLookup, policy PolicyChecker, events EventPublisher, noise NoiseNotifier, options Options) (toolapi.Result, error) {
	if options.Phases != nil {
		return run(ctx, corr, callID, toolName, args, tools, policy, WithOutputOptions(events, options), noise, options.Phases)
	}
	return Run(ctx, corr, callID, toolName, args, tools, policy, WithOutputOptions(events, options), noise)
}

// WithOutputOptions decorates terminal audit without changing caller/hook bytes.
// Compatibility entry points use the same representation without copying it.
func WithOutputOptions(events EventPublisher, options Options) EventPublisher {
	return outputPublisher{events, options}
}

type outputPublisher struct {
	events  EventPublisher
	options Options
}

func (p outputPublisher) PublishEvent(spec event.Spec) error {
	if spec.Kind == event.KindToolResult {
		if original, ok := spec.Payload.(map[string]any); ok {
			if full, ok := original["output"].(string); ok {
				output, ref, bytes, offloaded := tooloutput.Offload(p.options.Artifacts, p.options.ArtifactThreshold, full)
				if offloaded {
					payload := make(map[string]any, len(original)+2)
					for key, value := range original {
						payload[key] = value
					}
					payload["output"] = output
					payload["raw_ref"] = ref
					payload["output_bytes"] = bytes
					spec.Payload = payload
				}
			}
		}
	}
	return p.events.PublishEvent(spec)
}
