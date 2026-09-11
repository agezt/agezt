// SPDX-License-Identifier: MIT

package agent

// Agent Run: the main tool-loop driver + its failureReason helper. The
// biggest single function in the package (~340 lines). Carved out of
// agent.go during the Day 31 god file split #1 so the main file can
// focus on ToolCapability + context helpers.

import (
	"context"
	"errors"
	"fmt"
	"time"
	"github.com/agezt/agezt/kernel/event"
)

func Run(ctx context.Context, cfg LoopConfig, userIntent string) (answer string, runErr error) {
	if err := validateLoopConfig(cfg); err != nil {
		return "", err
	}
	normalizeLoopConfig(&cfg)

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

	// 1. task.received — the intent plus the run's provenance (image count,
	// agent attribution, wake source, the schedule/standing order that fired it).
	if _, err := publish(event.KindTaskReceived, "task", taskReceivedPayload(cfg, userIntent)); err != nil {
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

	// Panic firewall (M168): the loop calls into providers and tools that may be
	// third-party out-of-process plugins. A panic in any of them would otherwise
	// unwind through this bare goroutine and crash the WHOLE daemon, killing every
	// concurrent run. Recover it into a normal error so the blast radius is this
	// one run: the panic message is captured in runErr (and thus journaled by the
	// task.failed defer above, which runs AFTER this one — defers are LIFO, and
	// this is registered last so it sets runErr first). Registered after
	// task.received so a pre-run validation panic isn't double-counted as a run.
	defer func() {
		if r := recover(); r != nil {
			runErr = fmt.Errorf("%w: %v", ErrPanic, r)
		}
	}()

	messages := initialMessages(cfg, userIntent)

	tools, err := buildToolDefs(cfg.Tools)
	if err != nil {
		return "", err
	}

	// Mutable per-run state — the loop guard's counters, the policy denial
	// tallies, the observation cache, and the prompt-injection causal window —
	// lives in one documented struct (see run_tools.go) rather than a dozen
	// loop-locals captured by closures.
	st := newRunState(cfg, publish)

	// spentMicrocents accumulates this run's provider spend for the per-run cost
	// cap (M166). A local stack variable — no shared state, no lifecycle, no
	// cleanup — so the cap adds zero concurrency surface.
	var spentMicrocents int64

	// Per-run cached summarizer for elided tool outputs (M398); nil when the
	// caller configured none.
	summarizeElided := newElisionSummarizer(ctx, cfg)

	// Auto-continue (M833): the loop runs in SEGMENTS of cfg.MaxIter rounds. When a
	// segment exhausts without a final answer, instead of failing immediately we
	// inject a "keep going" turn and grant another segment — up to cfg.MaxAutoContinue
	// times — so a long task finishes autonomously instead of stopping at the cap.
	// `iter` is monotonic across segments (so journal iter numbers keep climbing);
	// segmentEnd is the round budget for the current segment.
	autoContinuesUsed := 0
	// Resume (M1002) continues from StartIter with a full fresh MaxIter segment;
	// StartIter == 0 (a fresh run) reduces to the original segmentEnd := cfg.MaxIter.
	iterStart := cfg.StartIter
	if iterStart < 0 {
		iterStart = 0
	}
	segmentEnd := iterStart + cfg.MaxIter
	for iter := iterStart; ; iter++ {
		if iter >= segmentEnd {
			// Segment exhausted without a final answer. Continue automatically if
			// budget remains; otherwise fall through to ErrMaxIter.
			if autoContinuesUsed >= cfg.MaxAutoContinue {
				break
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
			autoContinuesUsed++
			if _, err := publish(event.KindTaskContinued, "task", map[string]any{
				"attempt":      autoContinuesUsed,
				"of":           cfg.MaxAutoContinue,
				"iters_so_far": iter,
			}); err != nil {
				return "", fmt.Errorf("agent: publish task.continued: %w", err)
			}
			// Breather before pressing on (ctx-aware so a halt during the wait
			// ends the run immediately rather than after the sleep).
			if cfg.AutoContinueWait > 0 {
				t := time.NewTimer(cfg.AutoContinueWait)
				select {
				case <-ctx.Done():
					t.Stop()
					return "", ctx.Err()
				case <-t.C:
				}
			}
			messages = append(messages, Message{Role: RoleUser, Content: autoContinuePrompt})
			segmentEnd += cfg.MaxIter
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Resume checkpoint (M1002): snapshot the conversation at this safe
		// boundary. The previous iteration's model turn and its tool results are
		// fully settled here (every early exit between them is a return, not a
		// continue), so `messages` ends on a complete assistant→tool set with no
		// dangling tool_call — the daemon can persist and later re-seed it without
		// corrupting tool-call pairing. Placed before Steer so a paused run's state
		// is still captured.
		if cfg.Checkpoint != nil {
			cfg.Checkpoint(iter, messages)
		}

		// Live steering (M608): honour an operator pause at this safe boundary
		// (the in-flight call + tool results of the previous iteration are already
		// settled), then fold any injected directives into the conversation as
		// fresh user turns so the model acts on them next. Wait returns ctx.Err()
		// if the run is cancelled/halted/timed-out while paused, so steering never
		// makes a run un-killable. nil Steer skips this entirely.
		if cfg.Steer != nil {
			if err := cfg.Steer.Wait(ctx); err != nil {
				return "", err
			}
			for _, d := range cfg.Steer.Drain() {
				prefix, mode := steeringPrefix, "steer"
				if d.Note {
					prefix, mode = noteSteeringPrefix, "note"
				}
				messages = append(messages, Message{Role: RoleUser, Content: prefix + d.Text})
				if _, err := publish(event.KindRunSteered, "steer", map[string]any{
					"iter":      iter,
					"directive": d.Text,
					"mode":      mode,
				}); err != nil {
					return "", fmt.Errorf("agent: publish run.steered: %w", err)
				}
			}
		}

		// Context budgeting (SPEC-10 §3): before measuring/sending, trim the
		// assembled context to fit ContextBudget by eliding the oldest tool
		// outputs (system + recent turns protected). The drop is journaled so it
		// is auditable, not silent. No-op when ContextBudget is 0.
		if cfg.ContextBudget > 0 {
			before, _ := contextSize(cfg.System, messages)
			compacted, stats := compactMessagesDetailed(cfg.System, messages, cfg.ContextBudget, cfg.ContextProtectLast, cfg.ContextProtectFirst, summarizeElided, cfg.ContextRescueMarkers)
			if stats.Elided > 0 {
				messages = compacted
				after, _ := contextSize(cfg.System, messages)
				payload := map[string]any{
					"elided":               stats.Elided,
					"reclaimed_chars":      stats.Reclaimed,
					"context_chars_before": before,
					"context_chars_after":  after,
					"budget":               cfg.ContextBudget,
				}
				if stats.Rescued > 0 {
					payload["skill_rescued_count"] = stats.Rescued
					payload["skill_rescued_chars"] = stats.RescuedChars
				}
				if _, err := publish(event.KindContextCompacted, "context", payload); err != nil {
					return "", fmt.Errorf("agent: publish context.compacted: %w", err)
				}
			}
		}

		// Offer only the tools the policy hasn't repeatedly refused this run (M605).
		offered := st.offeredTools(tools)
		toolsBeforeDiscovery := len(offered)
		toolDiscovery := false
		if cfg.ToolSelector != nil {
			selected, err := cfg.ToolSelector(ctx, ToolSelectionRequest{
				Intent:   userIntent,
				Iter:     iter,
				Messages: messages,
				Tools:    offered,
			})
			if err != nil {
				return "", fmt.Errorf("agent: tool discovery: %w", err)
			}
			offered = normalizeSelectedTools(offered, selected)
			toolDiscovery = true
		}

		// 2a. llm.request — record what was sent: message count plus the
		// assembled context size, broken down by role (SPEC-10 §3.5 context
		// observability — the foundation of the context inspector). Lets an
		// operator see how big each call's context was and where it came from,
		// the #1 driver of cost and "lost in the middle" quality loss.
		ctxChars, ctxByRole := contextSize(cfg.System, messages)
		reqPayload := map[string]any{
			"iter":            iter,
			"messages":        len(messages),
			"model":           cfg.Model,
			"tools":           len(offered),
			"context_chars":   ctxChars,
			"context_by_role": ctxByRole,
		}
		if toolDiscovery {
			reqPayload["tool_discovery"] = true
			reqPayload["tools_before_discovery"] = toolsBeforeDiscovery
		}
		if _, err := publish(event.KindLLMRequest, "llm", reqPayload); err != nil {
			return "", fmt.Errorf("agent: publish llm.request: %w", err)
		}

		req := completionRequestFor(cfg, messages, offered)
		resp, err := callProvider(ctx, cfg, req, iter)
		if err != nil {
			return "", err
		}

		// 2c. llm.response
		if _, err := publish(event.KindLLMResponse, "llm", map[string]any{
			"iter":            iter,
			"stop_reason":     resp.StopReason,
			"usage":           resp.Usage,
			"text_chars":      len(resp.Message.Content),
			"reasoning_chars": len(resp.ReasoningContent), // M317: reasoning size (content streamed separately)
			"tool_calls":      len(resp.Message.ToolCalls),
		}); err != nil {
			return "", fmt.Errorf("agent: publish llm.response: %w", err)
		}

		// Per-run cost cap (M166): add this call's spend and stop if the run has
		// reached its cap. Like the daily ceiling, the check is post-call, so a run
		// can overshoot by at most the call that crosses the line — bounded and
		// predictable. The model the call billed under is the response's reported
		// model, falling back to the requested one (same rule as the Governor).
		if cfg.CostFn != nil && cfg.MaxRunCostMicrocents > 0 {
			billed := resp.Usage.Model
			if billed == "" {
				billed = cfg.Model
			}
			spentMicrocents += cfg.CostFn(billed, resp.Usage.InputTokens, resp.Usage.OutputTokens)
			if spentMicrocents >= cfg.MaxRunCostMicrocents {
				return "", fmt.Errorf("%w (spent ~%d, cap %d microcents)", ErrRunBudgetExceeded, spentMicrocents, cfg.MaxRunCostMicrocents)
			}
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

		// 2e. tool calls — gate (in call order), execute (concurrently when the
		// turn carries more than one), finalize (journal + append in the original
		// order). See run_tools.go for why the three phases exist and how they
		// share the causal window.
		jobs, err := st.gateToolCalls(ctx, resp.Message.ToolCalls, iter)
		if err != nil {
			return "", err
		}
		executeToolJobs(ctx, cfg, jobs)
		messages, err = st.finalizeToolJobs(ctx, jobs, iter, messages)
		if err != nil {
			return "", err
		}
	}

	return "", ErrMaxIter
}

// failureReason classifies a run's terminal error into a short, stable
// tag carried on the task.failed payload (the full error string rides
// alongside in "error"). Operators and `agt runs` use the tag to group
// failures without parsing free-form error text:
//
//	panic       — a provider/tool panicked; the firewall recovered it (M168).
//	max_iters   — the loop exhausted MaxIter without a final answer.
//	cost_budget — the per-run cost cap (MaxRunCostMicrocents) was reached (M166).
//	canceled    — the context was cancelled (operator halt / shutdown).
//	timeout     — the context deadline elapsed (a per-run / daemon timeout).
//	error       — anything else (provider error, publish failure, …).
//
// errors.Is is used so a wrapped provider error that ultimately carries
// context.Canceled/DeadlineExceeded is still classified correctly; the
// ctx.Err() fallback covers the bare-return cancellation paths.
func failureReason(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, ErrPanic):
		return "panic"
	case errors.Is(err, ErrMaxIter):
		return "max_iters"
	case errors.Is(err, ErrRunBudgetExceeded):
		return "cost_budget"
	case errors.Is(err, context.Canceled) || ctx.Err() == context.Canceled:
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded:
		return "timeout"
	default:
		return "error"
	}
}
