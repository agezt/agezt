// SPDX-License-Identifier: MIT

// Package agent defines the canonical, dialect-free conversation/tool types
// and the first-party single-agent tool-loop (DECISIONS B0d).
//
// The loop is owned end-to-end by Agezt — no third-party agent SDK
// (deliberate, non-negotiable). Provider plugins translate the canonical
// Message / ToolCall shapes to/from their backend dialect
// (Anthropic / OpenAI / Gemini / ...; SPEC-15).
//
// Every step the loop takes is journaled via the bus
// (durable-before-publish). Bounded by MaxIter and honors context
// cancellation (which is how `agt halt` stops a run).
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ersinkoc/agezt/kernel/bus"
	"github.com/ersinkoc/agezt/kernel/event"
)

// Role is the canonical conversation role.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one canonical conversation turn.
//
// For role=assistant, the model may return either Content (final text),
// ToolCalls (a request to invoke one or more tools), or both. For
// role=tool, ToolCallID identifies which assistant ToolCall this responds
// to, and Content carries the textual result.
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a model-issued request to invoke a tool.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolDef advertises a tool to the model.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// CompletionRequest is what the loop sends to a Provider.
type CompletionRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolDef
	MaxTokens int
	// TaskType is an optional hint to the Governor's per-task-type
	// routing layer (M1.cc). Free-form string; callers set it to
	// classify what kind of work this completion is — e.g. "plan"
	// for planner LLM calls, "salience" for memory-pruning calls,
	// "code" for code-gen-heavy work. Empty (the default) means
	// "no hint; use the standard subscription-first routing chain."
	//
	// The string is opaque to providers — they don't see it. Only
	// the Governor consults it, and only when the operator has
	// configured TaskRoutes for the key. So providers and tests
	// can ignore the field; setting or not setting it never changes
	// what the provider receives.
	TaskType string
}

// StopReason is the canonical reason a Provider stopped emitting tokens.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
)

// CompletionResponse is what a Provider returns from Complete.
type CompletionResponse struct {
	Message    Message    // role=assistant; may contain ToolCalls
	StopReason StopReason
	Usage      Usage
}

// Usage carries per-call token accounting (cost translation lives in the
// Governor at MVP time, SPEC-10; M0.5 just records the raw tokens).
type Usage struct {
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	Model        string `json:"model,omitempty"`
}

// Provider is implemented by anything that can drive a chat completion. For
// M0.5 we only support non-streaming Complete; streaming lands later.
type Provider interface {
	// Name identifies the provider plugin (e.g. "anthropic", "mock").
	Name() string
	// Complete sends one request and returns one response. Implementations
	// must honor ctx cancellation.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
}

// Tool is implemented by anything the agent loop can invoke during a
// task. In-process by default (DECISIONS B0a); out-of-process plugins
// satisfy the same interface via a thin client.
type Tool interface {
	// Definition is the schema/description advertised to the model.
	Definition() ToolDef
	// Invoke executes the tool with the parsed input and returns the
	// textual result for the model. Implementations must honor ctx.
	Invoke(ctx context.Context, input json.RawMessage) (Result, error)
}

// Result is what a Tool returns. IsError signals to the loop that the model
// should see an error (still appended as a tool result message, so the
// model can retry or adjust).
type Result struct {
	Output  string
	IsError bool
}

// LoopConfig configures one tool-loop run.
type LoopConfig struct {
	Provider Provider
	Tools    map[string]Tool
	Bus      *bus.Bus
	Model    string
	System   string
	// MaxIter caps tool-call rounds (DECISIONS E5: default 25).
	MaxIter int
	// MaxTokens passed to the provider per call. 0 → provider default.
	MaxTokens int
	// Actor is the journaling actor for emitted events (e.g. "agent-01H").
	Actor string
	// CorrelationID ties every event in this run together.
	CorrelationID string
	// Policy is the optional pre-tool-call gate. When non-nil, the loop
	// calls it before invoking each ToolCall and journals a policy.decision
	// event. A Deny verdict skips the tool invocation; the model sees a
	// tool result containing the deny reason so it can adjust.
	Policy Policy
}

// PolicyVerdict is the contract between the loop and the policy engine
// (kernel/edict). The loop journals it as the payload of policy.decision.
type PolicyVerdict struct {
	// Allow is true → the tool will be invoked.
	Allow bool
	// Capability is the policy-engine's classification of this call
	// (e.g. "shell", "file.write", "http.post"). Free-form string so the
	// loop need not import kernel/edict.
	Capability string
	// Reason is human-readable; used in journal entries and (on deny) in
	// the synthetic tool result returned to the model.
	Reason string
	// WouldAsk indicates an approval would have been requested in a future
	// release with live HITL routing. Captured for audit.
	WouldAsk bool
	// HardDenied indicates a non-overridable rule fired.
	HardDenied bool
}

// Policy is the signature the loop expects. Implementations are free to
// be pure functions (e.g. kernel/edict.Engine.Decide adapted by
// kernel/runtime).
type Policy func(ctx context.Context, tc ToolCall) PolicyVerdict

// DefaultMaxIter is DECISIONS E5.
const DefaultMaxIter = 25

// ErrMaxIter is returned by Run when MaxIter rounds elapse without a final
// assistant message.
var ErrMaxIter = errors.New("agent: max iterations exceeded")

// ErrUnknownTool is returned when the model asks for a tool the loop does
// not have registered.
var ErrUnknownTool = errors.New("agent: unknown tool")

// Run executes the tool-loop end-to-end:
//
//  1. Publish task.received with the user's intent.
//  2. For up to MaxIter rounds:
//     a. Publish llm.request with the current messages.
//     b. Call Provider.Complete.
//     c. Publish llm.response with the assistant's message.
//     d. If stop_reason is not tool_use → publish task.completed and
//     return Content.
//     e. Otherwise for each ToolCall: publish tool.invoked, invoke the
//     tool, publish tool.result, append a tool message.
//  3. If MaxIter elapses, return ErrMaxIter.
//
// Every step is durable-before-publish through cfg.Bus.
func Run(ctx context.Context, cfg LoopConfig, userIntent string) (string, error) {
	if cfg.Provider == nil {
		return "", errors.New("agent: provider required")
	}
	if cfg.Bus == nil {
		return "", errors.New("agent: bus required")
	}
	if cfg.Actor == "" {
		return "", errors.New("agent: actor required")
	}
	if cfg.MaxIter <= 0 {
		cfg.MaxIter = DefaultMaxIter
	}
	if cfg.Tools == nil {
		cfg.Tools = map[string]Tool{}
	}

	subject := func(suffix string) string {
		// Every event in this run shares an "agent.<actor>.<suffix>"
		// subject so subscribers can scope-filter to a single agent.
		return fmt.Sprintf("agent.%s.%s", cfg.Actor, suffix)
	}

	publish := func(kind event.Kind, suffix string, payload any) (*event.Event, error) {
		return cfg.Bus.Publish(event.Spec{
			Subject:       subject(suffix),
			Kind:          kind,
			Actor:         cfg.Actor,
			CorrelationID: cfg.CorrelationID,
			Payload:       payload,
		})
	}

	// 1. task.received
	if _, err := publish(event.KindTaskReceived, "task", map[string]string{"intent": userIntent}); err != nil {
		return "", fmt.Errorf("agent: publish task.received: %w", err)
	}

	messages := []Message{
		{Role: RoleUser, Content: userIntent},
	}

	tools := make([]ToolDef, 0, len(cfg.Tools))
	for _, t := range cfg.Tools {
		tools = append(tools, t.Definition())
	}

	for iter := range cfg.MaxIter {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// 2a. llm.request
		if _, err := publish(event.KindLLMRequest, "llm", map[string]any{
			"iter":     iter,
			"messages": len(messages),
			"model":    cfg.Model,
			"tools":    len(tools),
		}); err != nil {
			return "", fmt.Errorf("agent: publish llm.request: %w", err)
		}

		req := CompletionRequest{
			Model:     cfg.Model,
			System:    cfg.System,
			Messages:  messages,
			Tools:     tools,
			MaxTokens: cfg.MaxTokens,
		}
		var resp *CompletionResponse
		// Use the streaming path when the provider advertises it
		// (M1.q.y). Each text fragment is published as an ephemeral
		// KindLLMToken event so the CLI can render tokens inline;
		// the canonical llm.response (durable) still publishes below
		// with the assembled message + final usage. Streaming
		// failures fall back to fail-the-call rather than retry via
		// Complete — the StreamingProvider contract guarantees same-
		// response semantics, so any error is a real upstream
		// problem worth surfacing.
		if sp, ok := cfg.Provider.(StreamingProvider); ok {
			tokenIter := iter
			r, err := sp.CompleteStream(ctx, req, func(c Chunk) error {
				if c.TextDelta == "" {
					return nil
				}
				_, _ = cfg.Bus.PublishStreaming(event.Spec{
					Subject:       subject("llm"),
					Kind:          event.KindLLMToken,
					Actor:         cfg.Actor,
					CorrelationID: cfg.CorrelationID,
					Payload: map[string]any{
						"iter": tokenIter,
						"text": c.TextDelta,
					},
				})
				return nil
			})
			if err != nil {
				return "", fmt.Errorf("agent: provider %s (stream): %w", cfg.Provider.Name(), err)
			}
			resp = r
		} else {
			r, err := cfg.Provider.Complete(ctx, req)
			if err != nil {
				return "", fmt.Errorf("agent: provider %s: %w", cfg.Provider.Name(), err)
			}
			resp = r
		}

		// 2c. llm.response
		if _, err := publish(event.KindLLMResponse, "llm", map[string]any{
			"iter":        iter,
			"stop_reason": resp.StopReason,
			"usage":       resp.Usage,
			"text_chars":  len(resp.Message.Content),
			"tool_calls":  len(resp.Message.ToolCalls),
		}); err != nil {
			return "", fmt.Errorf("agent: publish llm.response: %w", err)
		}

		messages = append(messages, resp.Message)

		// 2d. final answer?
		if resp.StopReason != StopToolUse || len(resp.Message.ToolCalls) == 0 {
			if _, err := publish(event.KindTaskCompleted, "task", map[string]any{
				"iters":   iter + 1,
				"chars":   len(resp.Message.Content),
				"stopped": resp.StopReason,
			}); err != nil {
				return "", fmt.Errorf("agent: publish task.completed: %w", err)
			}
			return resp.Message.Content, nil
		}

		// 2e. tool calls
		for _, tc := range resp.Message.ToolCalls {
			if err := ctx.Err(); err != nil {
				return "", err
			}

			// Policy gate (Edict). Publishes a policy.decision event even
			// when no Policy is configured, so the journal makes the
			// gating posture explicit. A Deny verdict short-circuits the
			// invocation and synthesises a tool result the model sees.
			verdict := PolicyVerdict{Allow: true, Capability: tc.Name, Reason: "no policy configured"}
			if cfg.Policy != nil {
				verdict = cfg.Policy(ctx, tc)
			}
			if _, err := publish(event.KindPolicyDecision, "policy", map[string]any{
				"tool":        tc.Name,
				"call_id":     tc.ID,
				"capability":  verdict.Capability,
				"allow":       verdict.Allow,
				"reason":      verdict.Reason,
				"would_ask":   verdict.WouldAsk,
				"hard_denied": verdict.HardDenied,
			}); err != nil {
				return "", fmt.Errorf("agent: publish policy.decision: %w", err)
			}

			var result Result
			if !verdict.Allow {
				result = Result{
					Output:  "tool call denied by policy: " + verdict.Reason,
					IsError: true,
				}
				// Skip tool.invoked when the call never ran; jump to result.
			} else {
				if _, err := publish(event.KindToolInvoked, "tool", map[string]any{
					"tool":    tc.Name,
					"call_id": tc.ID,
					"input":   tc.Input,
				}); err != nil {
					return "", fmt.Errorf("agent: publish tool.invoked: %w", err)
				}
				tool, ok := cfg.Tools[tc.Name]
				if !ok {
					result = Result{
						Output:  fmt.Sprintf("tool %q is not available", tc.Name),
						IsError: true,
					}
				} else {
					r, invokeErr := tool.Invoke(ctx, tc.Input)
					if invokeErr != nil {
						result = Result{Output: invokeErr.Error(), IsError: true}
					} else {
						result = r
					}
				}
			}

			if _, err := publish(event.KindToolResult, "tool", map[string]any{
				"tool":    tc.Name,
				"call_id": tc.ID,
				"output":  result.Output,
				"error":   result.IsError,
			}); err != nil {
				return "", fmt.Errorf("agent: publish tool.result: %w", err)
			}

			messages = append(messages, Message{
				Role:       RoleTool,
				Content:    result.Output,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", ErrMaxIter
}
