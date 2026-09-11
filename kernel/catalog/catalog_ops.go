// SPDX-License-Identifier: MIT

// Catalog operations: NewEmpty, ProviderList, FindModel, StrictToolArgsNative, ToolCapableAlternative, ToolCapableAlternativeAmong, bestToolCapableModel, pickBestToolCapable, VisionCapableAmong, BestModelsAcross, pickBestModel, pickBestVision.
// Code extracted from types.go during the Day-57 god-file split. Public API unchanged.
package catalog


import (
	"sort"
	"strings"
)


func NewEmpty() *Catalog {
	return &Catalog{Providers: map[string]*Provider{}}
}

// ProviderList returns providers sorted by ID for stable iteration.
func (c *Catalog) ProviderList() []*Provider {
	out := make([]*Provider, 0, len(c.Providers))
	for _, p := range c.Providers {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FindModel returns the (Provider, Model) pair for a model ID. Search
// is two-pass: exact provider/model first when modelID contains "/"
// (e.g. "anthropic/claude-opus-4-5"); otherwise scan every provider
// for a matching model ID and return the first hit (deterministic via
// sorted provider iteration). Returns (nil, nil) if not found.
func (c *Catalog) FindModel(modelID string) (*Provider, *Model) {
	if modelID == "" {
		return nil, nil
	}
	if idx := strings.Index(modelID, "/"); idx > 0 {
		provID := modelID[:idx]
		mID := modelID[idx+1:]
		if p, ok := c.Providers[provID]; ok {
			if m, ok := p.Models[mID]; ok {
				return p, m
			}
		}
		return nil, nil
	}
	for _, p := range c.ProviderList() {
		if m, ok := p.Models[modelID]; ok {
			return p, m
		}
	}
	return nil, nil
}

// StrictToolArgsNative reports whether the catalog knows modelID and whether
// that model advertises sampler/provider-level enforcement for tool arguments.
// Unknown models return known=false so callers do not invent a degradation from
// missing catalog data.
func (c *Catalog) StrictToolArgsNative(modelID string) (native, known bool) {
	_, m := c.FindModel(modelID)
	if m == nil {
		return false, false
	}
	return m.SupportsStrictToolArgs(), true
}

// ToolCapableAlternative finds a tool-capable substitute for a
// tool-incapable model, within the SAME provider (M37 down-routing).
// Restricting to the same provider keeps the substitute on a provider that
// is already configured/credentialed — cross-provider remaps would risk
// routing to an unregistered provider. Among the provider's tool-capable
// models (excluding modelID itself) it picks the one with the largest
// context window, tie-broken by model ID ascending, so the choice is the
// most-capable sibling and deterministic. Returns (altID, true) on success,
// ("", false) if the model is unknown or the provider has no other
// tool-capable model. The returned ID is bare (no "provider/" prefix),
// matching how the catalog keys models.
func (c *Catalog) ToolCapableAlternative(modelID string) (string, bool) {
	// Same-provider only: the eligible set is the model's own provider.
	p, _ := c.FindModel(modelID)
	if p == nil {
		return "", false
	}
	return c.ToolCapableAlternativeAmong(modelID, func(provID string) bool { return provID == p.ID })
}

// ToolCapableAlternativeAmong generalises ToolCapableAlternative to
// cross-provider down-routing (M40): it finds a tool-capable substitute for a
// tool-incapable model among the providers for which providerEligible
// returns true. The daemon supplies providerEligible so only
// registered+credentialed providers are considered — a substitute on a
// provider the governor can't actually route to would be useless.
//
// Selection: the model's OWN provider is preferred (stay in-provider when
// possible); only if it has no eligible tool-capable sibling does the search
// widen to other eligible providers. Within the chosen scope the model with
// the largest context window wins, tie-broken by model id ascending, so the
// result is the most-capable option and deterministic despite random map
// iteration. Returns (altID, true) or ("", false) when nothing qualifies.
func (c *Catalog) ToolCapableAlternativeAmong(modelID string, providerEligible func(provID string) bool) (string, bool) {
	p, _ := c.FindModel(modelID)
	selfID := modelID
	if idx := strings.Index(modelID, "/"); idx > 0 {
		selfID = modelID[idx+1:]
	}

	// Pass 1: the model's own provider (preferred — keeps the remap on the
	// same, already-serving provider).
	if p != nil && providerEligible(p.ID) {
		if alt, ok := bestToolCapableModel(p, selfID); ok {
			return alt, true
		}
	}

	// Pass 2: widen to every OTHER eligible provider, deterministically.
	bestID, bestCtx := "", -1
	for _, q := range c.ProviderList() { // sorted by provider id
		if p != nil && q.ID == p.ID {
			continue
		}
		if !providerEligible(q.ID) {
			continue
		}
		if id, ctx, ok := pickBestToolCapable(q, ""); ok {
			if ctx > bestCtx || (ctx == bestCtx && id < bestID) {
				bestID, bestCtx = id, ctx
			}
		}
	}
	if bestID == "" {
		return "", false
	}
	return bestID, true
}

// bestToolCapableModel returns the largest-context tool-capable model in p,
// excluding excludeID, or ("", false) if none. Tie-broken by id ascending.
func bestToolCapableModel(p *Provider, excludeID string) (string, bool) {
	id, _, ok := pickBestToolCapable(p, excludeID)
	return id, ok
}

// pickBestToolCapable is the shared selection: largest Limit.Context among
// p's tool-capable models (excluding excludeID), tie-broken by id ascending.
// Returns the id, its context, and whether any qualified.
func pickBestToolCapable(p *Provider, excludeID string) (string, int, bool) {
	best := ""
	bestCtx := -1
	for id, m := range p.Models {
		if id == excludeID || !m.ToolCall {
			continue
		}
		if m.Limit.Context > bestCtx || (m.Limit.Context == bestCtx && id < best) {
			best, bestCtx = id, m.Limit.Context
		}
	}
	if best == "" {
		return "", 0, false
	}
	return best, bestCtx, true
}

// VisionCapableAmong finds a vision-capable model among the providers for which
// providerEligible returns true (the daemon passes registered+credentialed
// providers, so the result is one the governor can actually route to). It walks
// ProviderList() (sorted by id) and returns the first eligible provider's
// largest-context vision model (tie-broken by id ascending) — deterministic
// despite random map iteration. Returns (modelID, true) or ("", false) when no
// eligible provider has a vision model. Used by the vision sidecar (M821) to
// caption images when the active model can't see them.
func (c *Catalog) VisionCapableAmong(providerEligible func(provID string) bool) (string, bool) {
	for _, p := range c.ProviderList() {
		if !providerEligible(p.ID) {
			continue
		}
		if id, ok := pickBestVision(p); ok {
			return id, true
		}
	}
	return "", false
}

// BestModelsAcross returns one representative model id per ELIGIBLE provider —
// that provider's largest-context model, tie-broken by id ascending — across
// providers sorted by id, up to max ids (max<=0 = no cap). Fewer than max when
// fewer providers are eligible. Used to assemble a multi-provider Council of
// Elders (M837) so each seat speaks for a DIFFERENT keyed provider; the daemon
// passes the registered+credentialed predicate (same as VisionCapableAmong).
func (c *Catalog) BestModelsAcross(providerEligible func(provID string) bool, max int) []string {
	var out []string
	for _, p := range c.ProviderList() {
		if max > 0 && len(out) >= max {
			break
		}
		if !providerEligible(p.ID) {
			continue
		}
		if id, ok := pickBestModel(p); ok {
			out = append(out, id)
		}
	}
	return out
}

// pickBestModel returns the largest-context model in p (any modality), tie-broken
// by id ascending, or ("", false) if the provider lists none.
func pickBestModel(p *Provider) (string, bool) {
	best := ""
	bestCtx := -1
	for id, m := range p.Models {
		if m.Limit.Context > bestCtx || (m.Limit.Context == bestCtx && id < best) {
			best, bestCtx = id, m.Limit.Context
		}
	}
	return best, best != ""
}

// pickBestVision returns the largest-context vision-capable model in p, tie-broken
// by id ascending, or ("", false) if none. Mirrors pickBestToolCapable.
func pickBestVision(p *Provider) (string, bool) {
	best := ""
	bestCtx := -1
	for id, m := range p.Models {
		if !m.SupportsVision() {
			continue
		}
		if m.Limit.Context > bestCtx || (m.Limit.Context == bestCtx && id < best) {
			best, bestCtx = id, m.Limit.Context
		}
	}
	return best, best != ""
}

// Merge folds src into dst with src winning on key conflict. Mutates
// dst.Providers in place; per-provider Models maps are also merged
// (src model wins). Used by the loader to apply local/custom on top