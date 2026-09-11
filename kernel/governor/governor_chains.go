// SPDX-License-Identifier: MIT

package governor

// Governor model-chain + fallback-chain accessors: modelChainFor /
// modelKnownUnservable / SetTaskModelChains / TaskModelChainsView /
// SetFallbackChains / FallbackChainsView / defaultChainModels /
// expandChains. Carved out of governor.go during the Day 29 god file
// split #1.

import (
	"slices"
	"strings"
)

func (g *Governor) modelChainFor(taskType string) []string {
	if taskType == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	src := g.taskModelChains[taskType]
	if len(src) == 0 {
		return nil
	}
	return slices.Clone(src)
}

// modelKnownUnservable reports whether the registered providers DEFINITIVELY
// cannot serve model: no registered provider lists it AND every registered
// provider declares a non-empty catalog model list. An empty Models list means
// "unknown coverage" (the comment on ProviderInfo.Models) — such a provider may
// accept an unlisted id, so its presence makes the verdict false (don't skip).
// Mirrors the routeChain snapshot discipline: read the chain slices under
// chainMu so a concurrent Replace (hot reload) can't race the scan.
func (g *Governor) modelKnownUnservable(model string) bool {
	if model == "" {
		return false
	}
	g.chainMu.RLock()
	defer g.chainMu.RUnlock()
	for _, p := range g.sortedPrimary {
		if p.Serves(model) || len(p.Models) == 0 {
			return false
		}
	}
	for _, p := range g.fallback {
		if p.Serves(model) || len(p.Models) == 0 {
			return false
		}
	}
	// Guard: with no providers at all (impossible post-New, but cheap) treat as
	// servable so we never skip every model on an empty registry.
	if len(g.sortedPrimary) == 0 && len(g.fallback) == 0 {
		return false
	}
	return true
}

// SetTaskModelChains atomically replaces the per-task-type model fallback chains
// (M703) — the hot-reload path from the control plane / Routing UI. A nil/empty
// map clears all chains (routing reverts to single-model + provider fallback).
func (g *Governor) SetTaskModelChains(chains map[string][]string) {
	g.mu.Lock()
	g.taskModelChains = copyStringSliceMap(chains)
	g.mu.Unlock()
}

// TaskModelChainsView returns a copy of the effective per-task-type model chains.
func (g *Governor) TaskModelChainsView() map[string][]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return copyStringSliceMap(g.taskModelChains)
}

// ChainPrefix marks a model-id slot as a reference to a NAMED fallback chain
// (M963): "@fast" means "expand to the models of the chain called fast". Model
// ids never start with "@", so the token is unambiguous and works inside the
// comma/semicolon task-chain syntax too.
const ChainPrefix = "@"

// SetFallbackChains atomically replaces the named-chain registry and the default
// chain name (M963) — the hot-reload path from the control plane / Chains UI.
func (g *Governor) SetFallbackChains(chains map[string][]string, defaultChain string) {
	g.mu.Lock()
	g.fallbackChains = copyStringSliceMap(chains)
	g.defaultChain = defaultChain
	g.mu.Unlock()
}

// FallbackChainsView returns a copy of the named-chain registry and the default
// chain name.
func (g *Governor) FallbackChainsView() (map[string][]string, string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return copyStringSliceMap(g.fallbackChains), g.defaultChain
}

// defaultChainModels returns the models of the configured default chain, or nil.
func (g *Governor) defaultChainModels() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.defaultChain == "" {
		return nil
	}
	if src := g.fallbackChains[g.defaultChain]; len(src) > 0 {
		return slices.Clone(src)
	}
	return nil
}

// expandChains replaces every "@<name>" reference in a model list with the
// models of that named chain (M963), flattening and de-duplicating while
// preserving order. Unknown chains are dropped (a deleted chain must not crash a
// run). Non-reference ids pass through unchanged. One level only — chains hold
// real model ids, validated on save, so there is no recursion to worry about.
func (g *Governor) expandChains(models []string) []string {
	if len(models) == 0 {
		return models
	}
	g.mu.Lock()
	reg := g.fallbackChains
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	add := func(m string) {
		if m == "" {
			return
		}
		if _, dup := seen[m]; dup {
			return
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	for _, m := range models {
		if name, ok := strings.CutPrefix(m, ChainPrefix); ok {
			for _, cm := range reg[name] {
				add(cm)
			}
			continue
		}
		add(m)
	}
	g.mu.Unlock()
	return out
}

// SpentMicrocents returns the total spend for the current UTC day so far.
// Useful for the future `agt budget` command and for tests.
