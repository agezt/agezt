// SPDX-License-Identifier: MIT

// governor_complete_helpers.go: preflightAndRoute + runChain + openChain +
// ProviderHealth + callWithRetry + isTransient split off from governor_complete.go
// during the Day 211 god-file refactor (#141). Public API unchanged.
package governor

import (
	"context"
	"errors"
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
