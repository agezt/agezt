// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"

	"github.com/agezt/agezt/kernel/event"
)

// The provider call, extracted from the loop (refactor Phase 3.2).
//
// Streaming and non-streaming used to be two inline branches that each had to
// remember to emit llm.reasoning, wrap their error with the provider name, and
// leave `resp` in a consistent state. They differ in exactly one way — WHEN the
// reasoning text is known (per-chunk vs. all at once) — so that difference is
// all that is left branched here, and everything downstream (the nil-response
// contract check, the error wrapping, the caller's llm.response publish) is
// shared by construction rather than by two copies staying in sync.

// callProvider issues one completion. It streams when the provider supports it,
// publishing each text fragment as an ephemeral llm.token event and each
// reasoning delta as llm.reasoning so the CLI can render them live; the durable
// llm.response is the caller's to publish once this returns.
//
// A streaming failure is NOT retried through Complete: the StreamingProvider
// contract guarantees same-response semantics, so an error there is a real
// upstream problem worth surfacing rather than papering over with a second
// (billable) call.
func callProvider(ctx context.Context, cfg LoopConfig, req CompletionRequest, iter int) (*CompletionResponse, error) {
	// streamEphemeral publishes one live-only event for this run. Failures are
	// ignored by design: a dropped live frame must never fail a run whose
	// durable record is published separately.
	streamEphemeral := func(kind event.Kind, text string) {
		_, _ = cfg.Bus.PublishStreaming(event.Spec{
			Subject:       fmt.Sprintf("agent.%s.llm", cfg.Actor),
			Kind:          kind,
			Actor:         cfg.Actor,
			CorrelationID: cfg.CorrelationID,
			Payload:       map[string]any{"iter": iter, "text": text},
		})
	}

	var resp *CompletionResponse
	if sp, ok := cfg.Provider.(StreamingProvider); ok {
		r, err := sp.CompleteStream(ctx, req, func(c Chunk) error {
			// Reasoning delta (M317): a reasoning model's chain of thought is
			// visible live (agt pulse, the ACP relay) without bloating the
			// hash-chained journal.
			if c.ReasoningDelta != "" {
				streamEphemeral(event.KindLLMReasoning, c.ReasoningDelta)
			}
			if c.TextDelta != "" {
				streamEphemeral(event.KindLLMToken, c.TextDelta)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("agent: provider %s (stream): %w", cfg.Provider.Name(), err)
		}
		resp = r
	} else {
		r, err := cfg.Provider.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: provider %s: %w", cfg.Provider.Name(), err)
		}
		resp = r
		// A non-streaming provider returns the reasoning whole, with no deltas
		// (M325). Emit it as ONE ephemeral event so the same consumers the
		// streaming branch feeds live still see it — otherwise only
		// reasoning_chars on llm.response would record that it existed.
		if r != nil && r.ReasoningContent != "" {
			streamEphemeral(event.KindLLMReasoning, r.ReasoningContent)
		}
	}

	// A provider must return a non-nil response with a nil error (the Provider
	// contract). An out-of-process plugin is third-party code that can break
	// that — e.g. (nil, nil) on an unexpected empty upstream body. Guard it here:
	// every field access at the call site assumes a non-nil resp, and a nil deref
	// would panic the run (and, without the firewall, the whole daemon).
	if resp == nil {
		return nil, fmt.Errorf("agent: provider %s returned a nil response without an error", cfg.Provider.Name())
	}
	return resp, nil
}

// completionRequestFor assembles the per-iteration provider request from the
// loop's config and the current conversation. Every routing/accounting field the
// governor and the ledgers rely on is threaded here in one place, so a new one
// cannot be added to the config and silently forgotten on the wire.
func completionRequestFor(cfg LoopConfig, messages []Message, offered []ToolDef) CompletionRequest {
	return CompletionRequest{
		Model:               cfg.Model,
		System:              cfg.System,
		Messages:            messages,
		Tools:               offered,
		MaxTokens:           cfg.MaxTokens,
		TaskType:            cfg.TaskType,   // M703: per-task model routing hint
		ModelChain:          cfg.ModelChain, // M787: per-agent model fallback chain
		Agent:               cfg.Agent,      // M793: identity for the per-agent ledger
		AgentDailyCeilingMc: cfg.AgentDailyCeilingMc,
		CorrelationID:       cfg.CorrelationID, // M47: attribute spend to this run
		JSONMode:            cfg.JSONMode,      // M314: structured-output request
		Params:              cfg.Params,        // M997: per-request sampling knobs
	}
}
