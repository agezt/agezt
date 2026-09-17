// SPDX-License-Identifier: MIT

// Governor: preflightAndRoute + runChain + openChain + ProviderHealth + callWithRetry + isTransient + Complete + CompleteStream.
// Code extracted from governor_complete.go during the Day-129 god-file split.
// Public API unchanged.
package governor


import (
	"context"
	"errors"
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)




// ErrStreamInterrupted marks a streaming call that failed AFTER chunks were
// already delivered to the consumer (M882). Retrying or falling back would
// replay the stream from the start and duplicate everything the user already
// saw, so the governor treats it as terminal and surfaces the upstream error.
var ErrStreamInterrupted = errors.New("governor: stream interrupted after output started")

// Complete implements agent.Provider: the full pre-call gate set, routing and
// fallback over the non-streaming provider call. The agent tool-loop sees a
// single Provider.
func (g *Governor) Complete(ctx context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	// Opt-in response cache (M888): an exact repeat within the TTL is served
	// from memory — checked BEFORE preflight so a hit consumes neither a
	// rate-window slot nor budget (it costs the upstream nothing).
	var key string
	if g.respCache != nil {
		key = cacheKey(req)
		if resp, ok := g.respCache.get(key); ok {
			g.publish(event.Spec{
				Subject:       "governor.cache",
				Kind:          event.KindRoutingDecision,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload:       map[string]any{"cache": "hit", "task_model": req.Model, "task_type": req.TaskType},
			})
			return resp, nil
		}
	}
	resp, err := g.completeChained(req, func(r agent.CompletionRequest) (*agent.CompletionResponse, error) {
		chain, err := g.preflightAndRoute(&r)
		if err != nil {
			return nil, err
		}
		return g.runChain(ctx, r, chain, func(p *ProviderInfo) (*agent.CompletionResponse, error) {
			// A provider returning (nil, nil) violates the contract and would
			// panic every downstream resp.Message deref (and the governor's own
			// usage accounting). Normalize it to an error at the provider
			// boundary — the one choke point every governed call flows through —
			// so callers degrade via their existing err checks instead of
			// crashing (mirrors kernel/agent's loop guard). A nil-returning entry
			// is then treated like a failing one, so the fallback chain proceeds.
			resp, err := p.Provider.Complete(ctx, r)
			if err == nil && resp == nil {
				return nil, fmt.Errorf("governor: provider %s returned a nil response without an error", p.Name)
			}
			return resp, err
		})
	})
	if err == nil && resp != nil && g.respCache != nil {
		g.respCache.put(key, *resp)
	}
	return resp, err
}

// CompleteStream implements agent.StreamingProvider so the GOVERNED provider
// streams token/reasoning deltas to the loop (M1.q.y) instead of collapsing to
// a single response — through the exact same routing, fallback, budget and usage
// path as Complete. Before this, the governor (which every real run goes
// through) only satisfied agent.Provider, so the loop's streaming branch never
// engaged and the Web UI Chat never streamed live with a real provider. A chain
// entry that isn't itself streaming-capable (e.g. the offline mock fallback) is
// called non-streaming and simply yields no deltas; the assembled response still
// flows back unchanged.
func (g *Governor) CompleteStream(ctx context.Context, req agent.CompletionRequest, onChunk func(agent.Chunk) error) (*agent.CompletionResponse, error) {
	// Track whether any chunk has reached the consumer (M882). Until the first
	// chunk, a streaming failure retries / falls back exactly like Complete —
	// full parity. AFTER output has started flowing, a retry or fallback would
	// replay the stream from the start and duplicate everything already shown
	// (and journaled as llm.token events), so the failure becomes terminal.
	// onChunk is called sequentially within one attempt and attempts are
	// sequential, so a plain bool needs no lock.
	emitted := false
	wrapped := func(c agent.Chunk) error {
		if !c.IsEmpty() || c.ReasoningDelta != "" {
			emitted = true
		}
		return onChunk(c)
	}
	return g.completeChained(req, func(r agent.CompletionRequest) (*agent.CompletionResponse, error) {
		chain, err := g.preflightAndRoute(&r)
		if err != nil {
			return nil, err
		}
		return g.runChain(ctx, r, chain, func(p *ProviderInfo) (*agent.CompletionResponse, error) {
			if sp, ok := p.Provider.(agent.StreamingProvider); ok {
				resp, err := sp.CompleteStream(ctx, r, wrapped)
				if err != nil && emitted {
					return nil, fmt.Errorf("%w: %w", ErrStreamInterrupted, err)
				}
				return resp, err
			}
			return p.Provider.Complete(ctx, r)
		})
	})
}
