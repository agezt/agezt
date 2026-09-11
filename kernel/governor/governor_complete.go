// SPDX-License-Identifier: MIT

package governor

// Governor Complete pipeline (the main runChain / openChain / callWithRetry /
// Complete / CompleteStream / completeChained cluster). Carved out of
// governor.go during the Day 29 god file split #1 so the main file can
// focus on constructor + setter + small accessor surface.

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

func (g *Governor) preflightAndRoute(req *agent.CompletionRequest) ([]*ProviderInfo, error) {
	if err := g.runPreflight(req); err != nil {
		return nil, err
	}

	chain := g.routeChain(*req)
	if len(chain) == 0 {
		return nil, errors.New("governor: no eligible providers")
	}

	// Initial routing decision (first pick). task_type is included
	// so operators using `agt pulse --kind routing.decision` can see
	// which task-type overrides actually fired.
	g.publish(event.Spec{
		Subject:       "governor.route",
		Kind:          event.KindRoutingDecision,
		Actor:         "governor",
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"primary":    chain[0].Name,
			"chain":      providerNames(chain),
			"task_model": req.Model,
			"task_type":  req.TaskType,
		},
	})

	return chain, nil
}

// runChain executes req against the routed provider chain, recording usage on
// the first success and falling back on retryable errors. callOne performs the
// actual provider call for one chain entry — Complete passes the non-streaming
// call, CompleteStream the streaming one — so routing, fallback and usage
// accounting are identical for both.
func (g *Governor) runChain(ctx context.Context, req agent.CompletionRequest, chain []*ProviderInfo, callOne func(*ProviderInfo) (*agent.CompletionResponse, error)) (*agent.CompletionResponse, error) {
	// Skip providers whose circuit breaker is open, UNLESS that would skip the
	// whole chain — then try them all rather than hard-fail (a possibly-recovered
	// provider beats a guaranteed outage). M997.
	chain = g.openChain(chain)
	tried := make([]string, 0, len(chain))
	var lastErr error
	for i, p := range chain {
		tried = append(tried, p.Name)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := g.callWithRetry(ctx, req, p, callOne)
		if err == nil {
			g.recordUsage(p, req, resp)
			if g.breaker != nil && g.breaker.success(p.Name) {
				g.publish(event.Spec{
					Subject:       "governor.breaker_closed",
					Kind:          event.KindProviderBreakerClosed,
					Actor:         "governor",
					CorrelationID: req.CorrelationID,
					Payload:       map[string]any{"provider": p.Name},
				})
			}
			return resp, nil
		}
		lastErr = err
		if !shouldFallback(err) {
			// Don't try further providers when the user cancelled or the
			// budget is exhausted. Such errors are not the provider's fault,
			// so they don't count against its circuit breaker.
			return nil, err
		}
		// A genuine provider failure that triggers fall-back counts toward the
		// breaker tripping this provider out of the chain.
		if g.breaker != nil && g.breaker.failure(p.Name) {
			g.publish(event.Spec{
				Subject:       "governor.breaker_open",
				Kind:          event.KindProviderBreakerOpen,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload:       map[string]any{"provider": p.Name, "reason": err.Error()},
			})
		}
		// Fall back to next in chain.
		if i+1 < len(chain) {
			g.publish(event.Spec{
				Subject:       "governor.fallback",
				Kind:          event.KindProviderFallback,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"failed": p.Name,
					"next":   chain[i+1].Name,
					"reason": err.Error(),
				},
			})
		}
	}
	return nil, &ErrNoProviders{Tried: tried, Last: lastErr}
}

// openChain filters out providers whose circuit breaker is currently open. If
// that would empty the chain (every provider is tripped), it returns the
// original chain so the Governor still attempts a (possibly recovered) provider
// rather than hard-failing. M997.
func (g *Governor) openChain(chain []*ProviderInfo) []*ProviderInfo {
	if g.breaker == nil || !g.breaker.enabled() {
		return chain
	}
	allowed := make([]*ProviderInfo, 0, len(chain))
	for _, p := range chain {
		if g.breaker.allow(p.Name) {
			allowed = append(allowed, p)
		}
	}
	if len(allowed) == 0 {
		return chain
	}
	return allowed
}

// ProviderHealth returns the circuit-breaker state ("closed"/"open"/
// "half-open"/"disabled") for each registered provider, for diagnostics and the
// Models/health surface (M997).
func (g *Governor) ProviderHealth() map[string]string {
	out := map[string]string{}
	if g.breaker == nil {
		return out
	}
	for _, p := range g.cfg.Registry.All() {
		out[p.Name] = g.breaker.state(p.Name)
	}
	return out
}

// callWithRetry invokes one chain entry, retrying IN PLACE with exponential
// backoff + jitter on transient errors (M882) before the caller falls back to
// the next provider. Terminal errors (cancel, budget, stream-interrupted) and
// non-transient provider errors (auth, invalid request) return immediately —
// retrying those wastes time at best and duplicates output at worst.
func (g *Governor) callWithRetry(ctx context.Context, req agent.CompletionRequest, p *ProviderInfo, callOne func(*ProviderInfo) (*agent.CompletionResponse, error)) (*agent.CompletionResponse, error) {
	retries := g.cfg.ProviderRetries
	if retries == 0 {
		retries = DefaultProviderRetries
	}
	if retries < 0 {
		retries = 0
	}
	base := g.cfg.RetryBaseDelay
	if base <= 0 {
		base = DefaultRetryBaseDelay
	}
	for attempt := 0; ; attempt++ {
		resp, err := callOne(p)
		if err == nil {
			return resp, nil
		}
		if attempt >= retries || !shouldFallback(err) || !isTransient(err) {
			return nil, err
		}
		delay := base << attempt // 0.5s, 1s, 2s, …
		if delay >= 4 {
			delay += time.Duration(rand.Int64N(int64(delay) / 4)) // +0–25% jitter
		}
		g.publish(event.Spec{
			Subject:       "governor.retry",
			Kind:          event.KindProviderRetry,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload: map[string]any{
				"provider": p.Name,
				"attempt":  attempt + 1,
				"of":       retries,
				"delay_ms": delay.Milliseconds(),
				"reason":   err.Error(),
			},
		})
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, ctx.Err()
		case <-t.C:
		}
	}
}

// isTransient reports whether a provider error looks like a passing condition
// worth retrying on the SAME provider: rate limiting, upstream overload/5xx,
// or a network blip. Provider adapters surface upstream failures as wrapped
// text errors (no structured status crosses the plugin boundary), so this is
// a deliberately conservative substring match — an unrecognised error falls
// back to the next provider immediately, the historical behaviour.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"429", "rate limit", "rate_limit", "too many requests",
		"overloaded", "overloaded_error", "529",
		"500", "502", "503", "504",
		"internal server error", "bad gateway", "service unavailable", "gateway timeout",
		"timeout", "timed out", "deadline exceeded",
		"connection refused", "connection reset", "broken pipe",
		"unexpected eof", "eof",
		"temporarily unavailable", "try again",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

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

// completeChained runs req across its task type's model fallback chain (M703),
// if any: it tries each model in order via runOne (the full preflight + provider
// chain), and on a fallback-eligible failure of one model's WHOLE attempt it
// moves to the NEXT MODEL. With no chain configured it calls runOne once with req
// unchanged — byte-for-byte the pre-M703 single-model path. Terminal errors
// (context cancel, budget exhaustion) stop the walk immediately.
//
// Note: each model re-runs preflight (rate/budget pre-checks), so a request that
// actually falls back consumes one rate-window slot per model TRIED — acceptable
// for a soft burst cap, and only on the rare provider-failure path.
func (g *Governor) completeChained(req agent.CompletionRequest, runOne func(agent.CompletionRequest) (*agent.CompletionResponse, error)) (*agent.CompletionResponse, error) {
	// A per-request chain (M787 — a named agent's own fallbacks) WINS over
	// the task type's configured chain: the more specific identity beats the
	// broader category. The fallback events stay distinguishable via scope.
	models := req.ModelChain
	scope := "agent-chain"
	if len(models) == 0 {
		models = g.modelChainFor(req.TaskType)
		scope = "model-chain"
	}
	if len(models) == 0 {
		// No agent/task/explicit chain — fall to the operator's default named
		// chain so even a bare run gets the configured fallback ladder (M963).
		if def := g.defaultChainModels(); len(def) > 0 {
			models = def
			scope = "default-chain"
		}
	}
	// Expand any "@<name>" references into the named chain's models (M963). One
	// pass covers every source (agent, task, default) since they all flow here.
	models = g.expandChains(models)
	if len(models) == 0 {
		// No chain resolved a model. With the daemon's default-model removed,
		// an empty req.Model has nowhere to come from — refuse with an
		// actionable error instead of dispatching a blank model to the provider
		// (which would 400 with an opaque message).
		if strings.TrimSpace(req.Model) == "" {
			return nil, &ErrNoModelConfigured{TaskType: req.TaskType}
		}
		return runOne(req)
	}
	var lastErr error
	for i, m := range models {
		// Skip a chain model that NO registered provider can serve (M955).
		// Without this, applyModelRoute leaves the default chain in place and
		// the model id is dispatched to the primary provider, which 400s on an
		// id it doesn't recognise — one failed call PER provider in the chain,
		// a fallback storm — before the walk finally reaches the next model.
		// Skipping straight to the next model produces a real answer with zero
		// doomed calls. Guarded to "definitively unservable" (every provider
		// declares a model list and none include m) so an unknown-coverage
		// provider (empty Models, e.g. the mock/echo fallback) still gets the
		// benefit of the doubt — its presence preserves the legacy fall-through.
		if g.modelKnownUnservable(m) {
			lastErr = fmt.Errorf("%w: %q", ErrModelUnservable, m)
			if i+1 < len(models) {
				g.publish(event.Spec{
					Subject:       "governor.fallback",
					Kind:          event.KindProviderFallback,
					Actor:         "governor",
					CorrelationID: req.CorrelationID,
					Payload: map[string]any{
						"failed_model": m,
						"next_model":   models[i+1],
						"reason":       "no registered provider serves this model",
						"scope":        scope,
						"task_type":    req.TaskType,
						"skipped":      true,
					},
				})
			}
			continue
		}
		attempt := req
		attempt.Model = m
		resp, err := runOne(attempt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldFallback(err) {
			return nil, err
		}
		if i+1 < len(models) {
			g.publish(event.Spec{
				Subject:       "governor.fallback",
				Kind:          event.KindProviderFallback,
				Actor:         "governor",
				CorrelationID: req.CorrelationID,
				Payload: map[string]any{
					"failed_model": m,
					"next_model":   models[i+1],
					"reason":       err.Error(),
					"scope":        scope,
					"task_type":    req.TaskType,
				},
			})
		}
	}
	return nil, lastErr
}

// modelChainFor returns a copy of the configured model fallback chain for the
// task type, or nil if none. Read under mu (SetTaskModelChains may swap it).
