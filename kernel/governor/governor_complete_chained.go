// SPDX-License-Identifier: MIT

// Governor: completeChained (caller-supplied runOne + fallback-chain iteration) + modelChainFor.
// Code extracted from governor_complete.go during the Day-129 god-file split.
// Public API unchanged.
package governor


import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

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
