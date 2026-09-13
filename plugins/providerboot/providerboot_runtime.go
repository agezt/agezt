// SPDX-License-Identifier: MIT

package providerboot

// Provider boot/registration runtime: registerAlternates + Boot + Reload +
// catalogModelIDs + SelectPrimary + demoEchoProvider + BuildFromCatalog.
// Carved out of providerboot.go during the Day 163 god-file split so the
// main file can stay focused on types + env config + helpers.
// Public API unchanged.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/mock"
)
func registerAlternates(reg *governor.Registry, d Deps, primaryName string, mw []agent.Middleware, replace bool) map[string]bool {
	eligible := map[string]bool{primaryName: true}
	for _, entry := range d.Catalog.ProviderList() {
		if entry.ID == primaryName {
			continue // already the primary
		}
		if !Eligible(entry, d.Lookup) {
			continue
		}
		p, _, _, auth, err := BuildFromCatalog(d, entry, "")
		if err != nil {
			continue
		}
		info := &governor.ProviderInfo{
			Name:     p.Name(),
			Provider: agent.Wrap(p, mw...),
			AuthMode: auth,
			Models:   catalogModelIDs(d.Catalog, entry.ID),
		}
		var rerr error
		if replace {
			rerr = reg.Replace(info)
		} else {
			rerr = reg.Register(info)
		}
		if rerr != nil {
			continue // duplicate name or similar — skip gracefully
		}
		eligible[entry.ID] = true
	}
	// ChatGPT ("Sign in with ChatGPT") registers as a subscription alternate
	// when signed in (and not already the primary) — its models route to the
	// Responses backend adapter.
	if registerChatGPTAlternate(reg, d.BaseDir, primaryName, replace, mw) {
		eligible["chatgpt"] = true
	}
	if replace {
		// Stale-drop sweep (reload only): alternates that lost eligibility are
		// removed; fallback entries are never touched.
		for _, info := range reg.All() {
			if info.IsFallback || eligible[info.Name] {
				continue
			}
			reg.Remove(info.Name)
		}
	}
	return eligible
}

// Boot constructs the routing layer: one primary provider plus every other
// credentialed catalog provider as a model-routable alternate.
//
// The daemon has NO default provider, NO credential auto-pick, NO silent offline
// mock fallback, and NO default model (owner rule: "hiçbir default
// provider/model"). The only mock path is the explicit AGEZT_DEMO_ECHO=1 e2e /
// demo escape hatch.
//
// **Provider selection (catalog-driven):**
//
//	$AGEZT_PROVIDER=<catalog-id>    → e.g. "anthropic", "ollama-local",
//	                                  "groq", "openai" — any provider in the
//	                                  synced catalog. The ONLY way to select a
//	                                  primary. An unknown id is a hard error.
//	(unset)                          → UNCONFIGURED: a sentinel primary that
//	                                  fails every LLM call with an actionable
//	                                  "configure a provider" error. The daemon,
//	                                  Web UI, and Setup still run.
//	$AGEZT_DEMO_ECHO=1               → explicit offline demo/e2e mock provider
//	                                  when $AGEZT_PROVIDER is unset.
//	$AGEZT_MODEL=<model-id>         → the run model. If unset, runs resolve their
//	                                  model from per-task routing or a fallback
//	                                  chain; with neither, the governor returns
//	                                  ErrNoModelConfigured.
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
func catalogModelIDs(cat *catalog.Catalog, providerID string) []string {
	if cat == nil {
		return nil
	}
	entry, ok := cat.Providers[providerID]
	if !ok || len(entry.Models) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entry.Models))
	for m := range entry.Models {
		ids = append(ids, m)
	}
	sort.Strings(ids)
	return ids
}

// SelectPrimary returns the primary provider, a banner description,
// the resolved run model id (may be ""), the auth-mode tag for the
// Governor's registry, and an error.
//
// Selection:
//
//  1. AGEZT_PROVIDER=<catalog id> → look up in cat; compat.Build it. The ONLY
//     way to select a real primary; an unknown id is a hard error.
//  2. AGEZT_PROVIDER unset and AGEZT_DEMO_ECHO=1 → explicit offline e2e/demo
//     mock provider.
//  3. AGEZT_PROVIDER unset        → the "unconfigured" sentinel primary. No
//     auto-pick, no silent mock. The daemon boots so Setup/routing can be
//     configured, but LLM runs fail fast with an actionable error.
//
// The run model comes from AGEZT_MODEL when set; otherwise it is left empty and
// resolved per-run from routing / a fallback chain (or ErrNoModelConfigured).
func SelectPrimary(d Deps) (agent.Provider, string, string, governor.AuthMode, error) {
	cat := d.Catalog
	// AGEZT_PROVIDER and AGEZT_MODEL are *config*, not credentials —
	// always read from the config env directly (operators may want a one-off
	// override that doesn't sit in the vault).
	want := strings.ToLower(strings.TrimSpace(d.get(brand.EnvPrefix + "PROVIDER")))
	modelOverride := strings.TrimSpace(d.get(brand.EnvPrefix + "MODEL"))

	// ChatGPT ("Sign in with ChatGPT") is not a compat catalog provider — it uses
	// the OAuth token store + Responses adapter, so build it directly.
	if want == "chatgpt" {
		prov, desc, auth, ok := buildChatGPTPrimary(d.BaseDir, modelOverride)
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=chatgpt but not signed in — use Setup → Providers → Sign in with ChatGPT first", brand.EnvPrefix)
		}
		return prov, desc, modelOverride, auth, nil
	}

	// Explicit catalog id is the ONLY way to select a primary. The daemon has no
	// default provider, never auto-picks from credentials, and has no offline mock
	// fallback (owner rule: "hiçbir default provider/model"). An unknown id is a
	// hard error so a typo is loud, not silently degraded.
	if want != "" {
		entry, ok := cat.Providers[want]
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=%q not in catalog; run `agt catalog sync` then `agt catalog list`",
				brand.EnvPrefix, want)
		}
		return BuildFromCatalog(d, entry, modelOverride)
	}

	if strings.TrimSpace(d.get(brand.EnvPrefix+"DEMO_ECHO")) == "1" {
		model := modelOverride
		if model == "" {
			model = "mock"
		}
		return demoEchoProvider(),
			"demo echo mock (explicit " + brand.EnvPrefix + "DEMO_ECHO=1; offline e2e/demo)",
			model, governor.AuthLocal, nil
	}

	// AGEZT_PROVIDER unset → boot UNCONFIGURED. The daemon, Web UI, and Setup all
	// run so the operator can add a provider + key and configure routing/chains,
	// but any LLM call fails fast with an actionable error (unconfiguredProvider).
	// No credential auto-pick, no mock.
	return unconfiguredProvider{},
		"unconfigured (no " + brand.EnvPrefix + "PROVIDER set — add a provider + key in Setup → Providers; LLM runs fail until then)",
		"", governor.AuthLocal, nil
}

func demoEchoProvider() *mock.Provider {
	p := mock.New()
	p.Responder = func(req agent.CompletionRequest) agent.CompletionResponse {
		text := ""
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == agent.RoleUser {
				text = strings.TrimSpace(req.Messages[i].Content)
				if text != "" {
					break
				}
			}
		}
		if text == "" {
			text = "ok"
		}
		return mock.FinalText("[echo] " + text)
	}
	return p
}

// BuildFromCatalog finalises a catalog entry into a wire Provider.
// Shared by both the explicit-id path and the alternate-registration path.
// Credentials resolve through d.Lookup (the chained vault+env resolver).
func BuildFromCatalog(d Deps, entry *catalog.Provider, modelOverride string) (agent.Provider, string, string, governor.AuthMode, error) {
	lookup := d.Lookup
	// The daemon has NO default run model. AGEZT_MODEL, when set, is the model
	// every run uses unless per-task routing or a fallback chain overrides it.
	// When AGEZT_MODEL is empty the returned run model stays "" — so cfg.Model is
	// empty and the governor refuses any run that doesn't resolve a model via
	// routing/chain (ErrNoModelConfigured), per the owner's no-default rule.
	//
	// compat.Build still needs *a* concrete, catalog-valid model id to construct
	// the provider wire, so when AGEZT_MODEL is empty we fall back to the first
	// catalog model as an INERT construction placeholder. It is never surfaced as
	// a run default (cfg.Model stays "") and is never reached at call time (the
	// governor guard + per-provider model-required errors fire first).
	runModel := modelOverride
	constructModel := modelOverride
	if constructModel == "" {
		constructModel = compat.FirstModelID(entry)
	}
	if constructModel == "" {
		return nil, "", "", "", fmt.Errorf("provider %q in catalog has no models; set %sMODEL", entry.ID, brand.EnvPrefix)
	}
	// Auto-repair a cross-provider default model (don't hard-fail the boot):
	// AGEZT_MODEL may name a model this provider's catalog doesn't serve because
	// it is resolved per-run through routing / a fallback chain on a DIFFERENT
	// provider (e.g. AGEZT_PROVIDER=minimax-coding-plan + AGEZT_MODEL=gpt-5.4,
	// where gpt-5.4 rides @new-chain). compat.Build only needs a concrete,
	// catalog-valid id to construct the wire, so fall back to the inert
	// placeholder for CONSTRUCTION while keeping runModel as the override — the
	// governor still resolves the real model per run (or fails that one run with
	// an actionable error), instead of the whole daemon refusing to start.
	if modelOverride != "" {
		if _, ok := entry.Models[modelOverride]; !ok {
			placeholder := compat.FirstModelID(entry)
			fmt.Fprintf(d.stderr(),
				"%s: %sMODEL %q is not in provider %q's catalog — treating it as a routing/fallback-chain model and constructing %q with placeholder %q (set %sMODEL to one of this provider's models to silence)\n",
				brand.Binary, brand.EnvPrefix, modelOverride, entry.ID, entry.ID, placeholder, brand.EnvPrefix)
			constructModel = placeholder
		}
	}
	prov, _, err := compat.Build(entry, constructModel, lookup)
	if err != nil {
		return nil, "", "", "", err
	}
	auth := governor.AuthAPIKey
	if len(entry.Env) == 0 {
		auth = governor.AuthLocal
	}
	modelDesc := runModel
	if modelDesc == "" {
		modelDesc = "(unset — resolved from routing/fallback chain per run)"
	}
	desc := fmt.Sprintf("%s(catalog; family=%s, model=%s)", entry.ID, entry.Family(), modelDesc)
	return prov, desc, runModel, auth, nil
}
