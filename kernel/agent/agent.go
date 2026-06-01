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
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
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
	// Images carries image-attachment references for a vision-capable run
	// (M93). Additive + omitempty — providers that don't read it are
	// unaffected, and the M91 capability gate ensures a non-vision model never
	// receives a message carrying images.
	Images []string `json:"images,omitempty"`
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
	// CorrelationID identifies the run this completion serves. Like
	// TaskType it is a Governor-only hint — opaque to providers, who
	// never see it — letting the Governor stamp its budget.consumed
	// event with the spending run's correlation (M47) so spend can be
	// attributed per run / per delegation. Empty means "unattributed".
	CorrelationID string
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
	Message    Message // role=assistant; may contain ToolCalls
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
	// ToolTimeout, when > 0, bounds each individual tool invocation's
	// wall-clock (M34). A tool that overruns has its call context cancelled
	// and the loop feeds an IsError result ("tool X exceeded its … timeout")
	// back to the model — the RUN continues, unlike the per-run MaxDuration
	// (M31) which terminates the whole run. 0 = unbounded tool calls. A
	// genuine run-level cancel/timeout (operator halt, M32 cancel, or the
	// per-run deadline) still propagates and fails the run.
	ToolTimeout time.Duration
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
	// Images attaches image references to the initial user message (M93).
	// Only set on a vision-capable run (gated upstream by M91); the loop
	// puts them on the first user Message so the provider can encode them.
	Images []string
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

// maxJournaledAnswerRunes caps the answer text stored on task.completed (M51).
// The full answer is always returned to the caller; only the journaled copy —
// which lands in the append-only, hash-chained journal and is replayed on every
// projection rebuild — is bounded, so a pathologically large final message can't
// bloat the journal. The true length is preserved in the event's `chars` field.
const maxJournaledAnswerRunes = 8192

// truncateForJournal returns s unchanged when it fits the journal cap, else a
// rune-safe prefix with a marker. The byte-length fast path avoids the []rune
// allocation for the overwhelmingly common short answer (bytes ≤ cap ⇒ runes ≤
// cap, since a rune is ≥ 1 byte).
func truncateForJournal(s string) string {
	if len(s) <= maxJournaledAnswerRunes {
		return s
	}
	r := []rune(s)
	if len(r) <= maxJournaledAnswerRunes {
		return s
	}
	return string(r[:maxJournaledAnswerRunes]) + "…[truncated]"
}

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
func Run(ctx context.Context, cfg LoopConfig, userIntent string) (answer string, runErr error) {
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

	// 1. task.received — records the intent and, when present, the count of
	// image attachments (M93) so the run's provenance shows it had vision input.
	received := map[string]any{"intent": userIntent}
	if len(cfg.Images) > 0 {
		received["images"] = len(cfg.Images)
	}
	if _, err := publish(event.KindTaskReceived, "task", received); err != nil {
		return "", fmt.Errorf("agent: publish task.received: %w", err)
	}

	// From here the run has started: any error return is a run that began
	// but never reached task.completed. Emit a terminal task.failed exactly
	// once (best-effort) so `agt runs` can tell a real failure apart from a
	// true orphan (M28) and `agt runs stats` can split the success rate
	// (M30). A clean completion returns runErr==nil and is already terminal
	// via task.completed, so the defer no-ops. The best-effort publish must
	// not mask runErr, and must not run for the pre-task validation errors
	// above (no run started) — hence it's registered only after
	// task.received succeeds.
	defer func() {
		if runErr == nil {
			return
		}
		_, _ = publish(event.KindTaskFailed, "task", map[string]any{
			"error":  runErr.Error(),
			"reason": failureReason(ctx, runErr),
		})
	}()

	messages := []Message{
		{Role: RoleUser, Content: userIntent, Images: cfg.Images},
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
			Model:         cfg.Model,
			System:        cfg.System,
			Messages:      messages,
			Tools:         tools,
			MaxTokens:     cfg.MaxTokens,
			CorrelationID: cfg.CorrelationID, // M47: attribute spend to this run
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
			// Journal the answer text (M51) so `agt runs show` can display what
			// the run produced — the renderers expected it but it was never
			// emitted. The bus redactor scrubs secrets from the payload before it
			// lands in the journal (M15); the stored copy is length-capped so a
			// pathological output can't bloat the hash-chained, replayed journal.
			// The FULL answer is still returned to the caller unchanged.
			if _, err := publish(event.KindTaskCompleted, "task", map[string]any{
				"iters":   iter + 1,
				"chars":   len(resp.Message.Content),
				"stopped": resp.StopReason,
				"answer":  truncateForJournal(resp.Message.Content),
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
					// Optional per-tool wall-clock (M34): bound this single
					// invocation without bounding the whole run. A tool that
					// overruns gets an IsError result fed back to the model so
					// it can adapt; the run keeps going. The per-run deadline
					// (M31) and operator/M32 cancellation, by contrast, fail
					// the run — distinguished below by checking the PARENT ctx.
					toolCtx := ctx
					var toolCancel context.CancelFunc
					if cfg.ToolTimeout > 0 {
						toolCtx, toolCancel = context.WithTimeout(ctx, cfg.ToolTimeout)
					}
					r, invokeErr := tool.Invoke(toolCtx, tc.Input)
					// Capture whether the tool's OWN per-call deadline fired
					// BEFORE cancelling — calling toolCancel() would flip a
					// not-yet-expired toolCtx to Canceled and mask the
					// distinction. We key on toolCtx's state rather than the
					// returned error so a tool that wraps its error without the
					// DeadlineExceeded sentinel (e.g. the warden's "context
					// deadline exceeded" string) is still classified cleanly.
					toolTimedOut := cfg.ToolTimeout > 0 && toolCtx.Err() == context.DeadlineExceeded
					if toolCancel != nil {
						toolCancel()
					}
					switch {
					case invokeErr == nil:
						result = r
					case ctx.Err() != nil:
						// The RUN context itself ended (operator halt, M32
						// cancel, or the M31 per-run deadline) — a run-level
						// terminal, not a tool fault. Propagate so the run
						// fails with the correct reason instead of limping on.
						return "", ctx.Err()
					case toolTimedOut:
						// The tool overran its own budget while the run is fine:
						// hand the model a clear error and keep the run going.
						result = Result{
							Output:  fmt.Sprintf("tool %q exceeded its %s timeout", tc.Name, cfg.ToolTimeout),
							IsError: true,
						}
					default:
						result = Result{Output: invokeErr.Error(), IsError: true}
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

// failureReason classifies a run's terminal error into a short, stable
// tag carried on the task.failed payload (the full error string rides
// alongside in "error"). Operators and `agt runs` use the tag to group
// failures without parsing free-form error text:
//
//	max_iters — the loop exhausted MaxIter without a final answer.
//	canceled  — the context was cancelled (operator halt / shutdown).
//	timeout   — the context deadline elapsed (a future per-run timeout).
//	error     — anything else (provider error, publish failure, …).
//
// errors.Is is used so a wrapped provider error that ultimately carries
// context.Canceled/DeadlineExceeded is still classified correctly; the
// ctx.Err() fallback covers the bare-return cancellation paths.
func failureReason(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, ErrMaxIter):
		return "max_iters"
	case errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled:
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded:
		return "timeout"
	default:
		return "error"
	}
}
