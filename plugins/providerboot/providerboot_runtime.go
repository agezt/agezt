// SPDX-License-Identifier: MIT

package providerboot

import (
	"fmt"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
)

func Boot(d Deps) (*Result, error) {
	cat := d.Catalog
	reg := governor.NewRegistry()
	mw := Middleware(d.Get) // M997: opt-in; empty by default → providers registered unwrapped
	primary, primaryDesc, model, authMode, err := SelectPrimary(d)
	if err != nil {
		return nil, err
	}
	primaryName := primary.Name()
	if err := reg.Register(&governor.ProviderInfo{
		Name:     primaryName,
		Provider: agent.Wrap(primary, mw...),
		AuthMode: authMode,
		Models:   catalogModelIDs(cat, primaryName),
	}); err != nil {
		return nil, fmt.Errorf("register primary: %w", err)
	}

	// Track which catalog providers actually got registered — the eligible
	// set for cross-provider down-routing (M40). Keyed by catalog provider id,
	// so it matches catalog lookups. (For the "unconfigured" sentinel this is a
	// non-catalog name that simply won't match, which is fine.) The set is
	// mutex-guarded and REFRESHED by Reload — the altFinder closure below reads
	// it live instead of freezing the boot-time snapshot.
	registered := registerAlternates(reg, d, primaryName, mw, false)
	extraProviders := len(registered) - 1 // everything but the primary
	es := &eligibleSet{m: registered}
	liveEligible.Store(reg, es)

	// No offline mock fallback: the daemon never silently answers with a mock
	// (owner rule). When the primary fails and no fallback chain / alternate
	// serves the request, the governor surfaces the real error.
	fallbackDesc := ""

	ceiling := governor.DefaultDailyCeilingMicrocents

	ec, err := governorConfigFromEnv(d.Get)
	if err != nil {
		return nil, err
	}

	// The alternative finder: same-provider by default, cross-provider (among
	// the actually-registered providers, read from the LIVE eligibility set)
	// when enabled.
	altFinder := cat.ToolCapableAlternative
	if ec.crossDownRoute {
		altFinder = func(model string) (string, bool) {
			return cat.ToolCapableAlternativeAmong(model, es.has)
		}
	}

	gov, err := governor.New(governor.Config{
		Registry:                reg,
		ResponseCacheTTL:        ec.respCacheTTL,
		DailyCeilingMicrocents:  ceiling,
		RateLimitPerMin:         ec.ratePerMin,
		TaskRoutes:              ec.taskRoutes,
		TaskRouteRequires:       ec.taskRequires,
		TaskModelOverrides:      ec.taskModels,
		TaskModelChains:         ec.taskModelChains,
		FallbackChains:          ec.fallbackChains,
		DefaultChain:            ec.defaultChain,
		TaskBudgets:             ec.taskBudgets,
		StrictModelCapabilities: ec.strictCaps,
		StrictPricing:           ec.strictPricing,
		DownRouteToolModels:     ec.downRoute,
		ModelToolCapable: func(model string) (bool, bool) {
			_, m := cat.FindModel(model)
			if m == nil {
				return false, false
			}
			return m.ToolCall, true
		},
		ToolCapableAlternative: altFinder,
		ModelJSONNative: func(model string) (bool, bool) {
			p, m := cat.FindModel(model)
			if p == nil || m == nil {
				return false, false
			}
			return catalog.FamilySupportsNativeJSONMode(p.Family()), true
		},
		ModelStrictToolArgsNative: cat.StrictToolArgsNative,
	})
	if err != nil {
		return nil, err
	}
	desc := fmt.Sprintf("primary=%s%s, daily_ceiling=$%.2f",
		primaryDesc, fallbackDesc, float64(ceiling)/1e9)
	if ec.strictCaps {
		desc += ", strict-capabilities"
	}
	if ec.downRoute {
		if ec.crossDownRoute {
			desc += ", tool-downrouting(cross)"
		} else {
			desc += ", tool-downrouting"
		}
	}
	if extraProviders > 0 {
		desc += fmt.Sprintf(", model-routable_alternates=%d", extraProviders)
	}
	if len(ec.taskRoutes) > 0 {
		desc += fmt.Sprintf(", task_routes=%d", len(ec.taskRoutes))
	}
	if len(ec.taskBudgets) > 0 {
		desc += fmt.Sprintf(", task_budgets=%d", len(ec.taskBudgets))
	}
	return &Result{
		Governor: gov,
		Primary:  primaryName,
		Model:    model,
		Desc:     desc,
		AuthMode: authMode,
		Eligible: es.has,
	}, nil
}

// Reload is the hot-reload path (`agt provider reload` / control plane
// provider_reload): it re-runs the same selection + registration logic Boot
// uses against fresh Deps (freshly loaded catalog + credential chain) and
// atomically swaps the Governor's primary. Ordering is LOAD-BEARING: every
// registry mutation (sentinel removal, alternate reconciliation) happens
// BEFORE gov.Replace(primary), because Replace rebuilds the governor's cached
// primary/fallback routing chains from the registry — alternates registered
// after Replace would be invisible to routing until the next reload.
//
// Returns the freshly-resolved run model so the caller can k.SetModel it
// (M816: without that, the governor routes to the new provider while runs
// still carry the OLD model id).
//
// DELIBERATE (survey decision, 2026-08): the governor env knobs
// (AGEZT_RATE_PER_MIN, TASK_ROUTES, MODEL_STRICT, PRICING_STRICT, DOWNROUTE,
// LLM_CACHE_TTL, ...) are NOT re-read here — a malformed live edit must not
// fail a provider reload. They remain boot-only until a follow-up ApplyConfig
// pass. Task-model chains already have their own live path (chainsSetter).
func Reload(gov *governor.Governor, d Deps) (string, error) {
	mw := Middleware(d.Get) // M997 middleware follows the current env, same as Boot
	// Re-run the same selection logic the boot path uses. Errors are surfaced
	// to the operator rather than swallowed — a missing credential after
	// rotation should be visible immediately, not next time the daemon happens
	// to dispatch an LLM call.
	prov, _, model, auth, err := SelectPrimary(d)
	if err != nil {
		return "", fmt.Errorf("select primary: %w", err)
	}
	// Demote the stale "unconfigured" sentinel before installing the real one
	// (M816). When the daemon booted with no AGEZT_PROVIDER, Boot registered
	// the unconfigured sentinel as the PRIMARY. Registry.Replace only swaps an
	// entry of the SAME name, so replacing "unconfigured" with "deepseek" would
	// APPEND deepseek behind the sentinel — leaving the sentinel at primary[0],
	// still refusing every run (the first-run-wizard case: add a key + set
	// AGEZT_PROVIDER, reload, but runs still error "no provider configured").
	// Remove the sentinel entirely — it has no fallback role. If the reload
	// still resolves to the sentinel (operator added a key but no
	// AGEZT_PROVIDER), we keep it.
	reg := gov.Registry()
	if prov.Name() != UnconfiguredName {
		reg.Remove(UnconfiguredName) // no-op when absent
	}
	// Reconcile alternates through the SAME path Boot uses (replace semantics +
	// stale-drop sweep), middleware-wrapped like Boot.
	eligible := registerAlternates(reg, d, prov.Name(), mw, true)
	// Refresh the LIVE cross-provider down-route eligibility set the
	// governor's altFinder closure reads (drift fix: it used to stay frozen at
	// the boot-time snapshot).
	if v, ok := liveEligible.Load(reg); ok {
		v.(*eligibleSet).set(eligible)
	}
	// The primary is installed LAST via gov.Replace, which also rebuilds the
	// routing chains over the reconciled registry.
	if err := gov.Replace(&governor.ProviderInfo{
		Name:     prov.Name(),
		Provider: agent.Wrap(prov, mw...),
		AuthMode: auth,
		Models:   catalogModelIDs(d.Catalog, prov.Name()),
	}); err != nil {
		return "", fmt.Errorf("registry replace: %w", err)
	}
	return model, nil
}

// catalogModelIDs returns the sorted model ids the catalog lists for the given
// provider id, used to populate ProviderInfo.Models for per-request routing.
// Returns nil when the catalog or entry is absent (e.g. the mock primary).
