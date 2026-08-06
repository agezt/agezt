// SPDX-License-Identifier: MIT

// Package providerboot owns provider bootstrap for the daemon: primary
// selection, alternate registration, the governor's construction, and the
// hot-reload path — Boot and Reload share ONE registration path
// (registerAlternates), retiring the boot-vs-reload drift class (M928/M816
// and the 2026-08 survey's live drifts: middleware dropped on reload, the
// cross-provider down-route eligibility set frozen at boot).
//
// The package is deliberately concrete (imports compat/mock/openairesponses/
// chatgptauth); it lives under plugins/ so the kernel never grows a
// kernel→plugins edge.
package providerboot

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// Deps bundles everything Boot/Reload need from the daemon. Get and Stderr
// are nil-defaulted (os.Getenv / io.Discard) so tests can inject a map-backed
// environment and capture warnings without touching the process env.
type Deps struct {
	// Catalog is the loaded provider catalog snapshot.
	Catalog *catalog.Catalog
	// Lookup is the chained credential resolver (vault → env → AWS chain).
	Lookup func(string) string
	// BaseDir is the daemon base dir (ChatGPT token store lives under it).
	BaseDir string
	// Get reads configuration environment variables. nil → os.Getenv.
	Get func(string) string
	// Stderr receives non-fatal boot warnings. nil → io.Discard.
	Stderr io.Writer
}

func (d Deps) get(name string) string {
	if d.Get != nil {
		return d.Get(name)
	}
	return os.Getenv(name)
}

func (d Deps) stderr() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return io.Discard
}

// Result is what Boot hands back to the daemon.
type Result struct {
	// Governor is the constructed routing layer (also the agent.Provider the
	// kernel runs against).
	Governor *governor.Governor
	// Primary is the primary provider's registry name. Equal to
	// UnconfiguredName when no provider is configured — the daemon's
	// first-run nudge keys off exactly this (NOT the model id; the survey
	// found the old `model == "mock"` check fired exactly backwards).
	Primary string
	// Model is the run model for the kernel config ("" when none configured).
	Model string
	// Desc is the human-readable banner description.
	Desc string
	// AuthMode is the primary provider's auth classification.
	AuthMode governor.AuthMode
	// Eligible reads the LIVE cross-provider down-route eligibility set
	// (catalog provider id → registered). Refreshed by Reload; the governor's
	// cross-provider altFinder closure reads the same set.
	Eligible func(providerID string) bool
}

// eligibleSet is the mutex-guarded live eligibility map behind the governor's
// cross-provider down-route altFinder (drift fix: the old implementation
// closed over a plain map built once in buildGovernor, so a reload mutated
// the registry but the down-route search kept the boot-time snapshot).
type eligibleSet struct {
	mu sync.RWMutex
	m  map[string]bool
}

func (s *eligibleSet) has(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[id]
}

func (s *eligibleSet) set(m map[string]bool) {
	s.mu.Lock()
	s.m = m
	s.mu.Unlock()
}

// liveEligible maps a governor's *Registry to its eligibleSet so Reload —
// which only receives the *governor.Governor — can refresh the set Boot's
// altFinder closure reads. Entries live as long as the process (one governor
// per daemon; test governors leak a map entry each, which is fine).
var liveEligible sync.Map // *governor.Registry → *eligibleSet

// UnconfiguredName is the Name() of the sentinel primary registered when no
// LLM provider is configured. The reload path keys off it to swap in a real
// provider once the operator configures one, and the daemon's first-run nudge
// compares Result.Primary against it.
const UnconfiguredName = "unconfigured"

// unconfiguredProvider is the daemon's primary when NO LLM provider is
// configured (AGEZT_PROVIDER unset). The daemon ships with no default provider
// or model (owner rule: "hiçbir default provider/model"), so a fresh install
// boots with this sentinel: the daemon, Web UI, and Setup all run, but any LLM
// call fails fast with an actionable message telling the operator to add a
// provider + key and a model (via AGEZT_MODEL or a routing/fallback chain). It
// is swapped for a real provider by the reload path once one is configured.
type unconfiguredProvider struct{}

func (unconfiguredProvider) Name() string { return UnconfiguredName }
func (unconfiguredProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("no LLM provider configured — add a provider and API key (Setup → Providers, or set %sPROVIDER) and a model (%sMODEL, a per-task route, or a fallback chain)", brand.EnvPrefix, brand.EnvPrefix)
}

// Eligible reports whether a catalog provider can serve requests: a supported
// compat family AND resolvable credentials. This is THE eligibility predicate —
// registration (Boot/Reload), the vision sidecar picker, the keyed-model
// delegation predicate, and the council membership all share it (it used to be
// copy-pasted at each site).
func Eligible(entry *catalog.Provider, lookup func(string) string) bool {
	return entry != nil && compat.IsSupportedFamily(entry.Family()) && entry.HasCredentials(lookup)
}

// Middleware builds the opt-in provider middleware stack from the
// environment (M997). It is empty by default, so every provider is registered
// unwrapped and behaviour is unchanged. Operators opt in to:
//   - DefaultParams: AGEZT_GEN_TEMPERATURE / AGEZT_GEN_TOP_P / AGEZT_GEN_REASONING_EFFORT
//     supply per-call sampling defaults filled in only where a request left them unset.
//   - ExtractReasoning: AGEZT_EXTRACT_REASONING=on pulls inline <think>…</think> out of
//     the answer into ReasoningContent (for inline-reasoning models on OpenAI-compatible /
//     Ollama gateways that don't use a dedicated reasoning field).
//   - SimulateStreaming: AGEZT_SIMULATE_STREAMING=on lets non-streaming providers present
//     a single-chunk stream for a uniform UI.
//
// get is nil-defaulted to os.Getenv.
func Middleware(get func(string) string) []agent.Middleware {
	if get == nil {
		get = os.Getenv
	}
	envOn := func(suffix string) bool {
		v := strings.ToLower(strings.TrimSpace(get(brand.EnvPrefix + suffix)))
		return v == "1" || v == "on" || v == "true" || v == "yes"
	}
	var mws []agent.Middleware

	var defaults agent.Params
	if s := strings.TrimSpace(get(brand.EnvPrefix + "GEN_TEMPERATURE")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			defaults.Temperature = &f
		}
	}
	if s := strings.TrimSpace(get(brand.EnvPrefix + "GEN_TOP_P")); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			defaults.TopP = &f
		}
	}
	defaults.ReasoningEffort = strings.TrimSpace(get(brand.EnvPrefix + "GEN_REASONING_EFFORT"))
	if !defaults.IsZero() {
		mws = append(mws, agent.DefaultParamsMiddleware(defaults))
	}
	if envOn("EXTRACT_REASONING") {
		mws = append(mws, agent.ExtractReasoningMiddleware("<think>", "</think>"))
	}
	if envOn("SIMULATE_STREAMING") {
		mws = append(mws, agent.SimulateStreamingMiddleware())
	}
	return mws
}

// govEnvConfig holds the governor knobs parsed from the environment.
// Split from Boot as its own function so a later daemonconfig phase (2.5)
// can adopt it wholesale.
type govEnvConfig struct {
	ratePerMin      int
	taskRoutes      governor.TaskRoutes
	taskRequires    governor.TaskRouteRequires
	taskModels      governor.TaskModelOverrides
	taskModelChains governor.TaskModelChains
	fallbackChains  map[string][]string
	defaultChain    string
	taskBudgets     map[string]int64
	strictCaps      bool
	strictPricing   bool
	downRoute       bool
	crossDownRoute  bool
	respCacheTTL    time.Duration
}

// governorConfigFromEnv parses every governor env knob. A malformed value is a
// hard error — Boot fails fast so the operator gets loud feedback at startup.
// Reload deliberately does NOT call this (see Reload).
func governorConfigFromEnv(get func(string) string) (govEnvConfig, error) {
	if get == nil {
		get = os.Getenv
	}
	var ec govEnvConfig

	// Optional primary call-rate cap (M106): AGEZT_RATE_PER_MIN=<n> bounds how
	// many completion calls the PRIMARY governor admits per minute (tenants have
	// AGEZT_TENANT_RATE_PER_MIN). 0 / unset = unlimited. A throttled call is
	// journaled as rate.limited and surfaced by `agt ratelimit log`. Malformed =
	// hard startup error (fast feedback, mirrors the other numeric knobs).
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "RATE_PER_MIN")); spec != "" {
		n, perr := strconv.Atoi(spec)
		if perr != nil || n < 0 {
			return ec, fmt.Errorf("AGEZT_RATE_PER_MIN: want a non-negative integer, got %q", spec)
		}
		ec.ratePerMin = n
	}

	// Optional per-task-type routing override (M1.cc). Operators set
	// AGEZT_TASK_ROUTES="plan=anthropic;code=anthropic,openai;..." to
	// pin specific task types to specific providers. Unrecognised
	// provider names degrade silently to the default chain (see the
	// TaskRoutes doc), so a typo doesn't take down the daemon — but
	// a syntactically-malformed entry IS a hard startup error so the
	// operator gets fast feedback instead of silent misrouting.
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TASK_ROUTES")); spec != "" {
		parsed, err := governor.ParseTaskRoutesEnv(spec)
		if err != nil {
			return ec, fmt.Errorf("AGEZT_TASK_ROUTES: %w", err)
		}
		ec.taskRoutes = parsed
	}
	// Hard-pin routes (M1.kk). Same env-var syntax; restrictive
	// rather than preferential semantics.
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TASK_ROUTE_REQUIRES")); spec != "" {
		parsed, err := governor.ParseTaskRoutesEnv(spec)
		if err != nil {
			return ec, fmt.Errorf("AGEZT_TASK_ROUTE_REQUIRES: %w", err)
		}
		ec.taskRequires = governor.TaskRouteRequires(parsed)
	}

	// Per-task-type model override (M1.ll).
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TASK_MODEL_OVERRIDES")); spec != "" {
		parsed, err := governor.ParseTaskModelOverridesEnv(spec)
		if err != nil {
			return ec, fmt.Errorf("AGEZT_TASK_MODEL_OVERRIDES: %w", err)
		}
		ec.taskModels = parsed
	}

	// Per-task-type model fallback CHAINS (M703): task → ordered model ids tried
	// in turn. Supersedes TASK_MODEL_OVERRIDES for the same task. Editable live
	// via the Routing UI / control plane (persisted back into this env var).
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TASK_MODEL_CHAINS")); spec != "" {
		parsed, err := governor.ParseTaskModelChainsEnv(spec)
		if err != nil {
			return ec, fmt.Errorf("AGEZT_TASK_MODEL_CHAINS: %w", err)
		}
		ec.taskModelChains = parsed
	}

	// Named reusable fallback chains (M963): a registry of "@name → [models]"
	// referenced anywhere a model is chosen, plus an optional default chain for
	// runs that resolve to none. Editable live via the Chains UI (persisted back
	// into these env vars).
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "FALLBACK_CHAINS")); spec != "" {
		parsed, err := governor.ParseFallbackChainsEnv(spec)
		if err != nil {
			return ec, fmt.Errorf("AGEZT_FALLBACK_CHAINS: %w", err)
		}
		ec.fallbackChains = parsed
	}
	ec.defaultChain = strings.TrimSpace(get(brand.EnvPrefix + "DEFAULT_CHAIN"))

	// Per-task-type daily budget caps (M1.zz). Layered on top of
	// DAILY_CEILING; both must pass for a call to proceed.
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "TASK_BUDGETS")); spec != "" {
		parsed, err := governor.ParseTaskBudgetsEnv(spec)
		if err != nil {
			return ec, fmt.Errorf("AGEZT_TASK_BUDGETS: %w", err)
		}
		ec.taskBudgets = parsed
	}

	// Model capability gate (M25). Opt-in via AGEZT_MODEL_STRICT=on: a
	// tools-bearing request to a catalog-known model that lacks tool-use is
	// rejected pre-flight instead of failing deep in the provider call. The
	// catalog backs the lookup; per-tenant governors inherit it via
	// WithLimits (the whole Config is copied).
	ec.strictCaps = strings.EqualFold(get(brand.EnvPrefix+"MODEL_STRICT"), "on")
	// Strict pricing (M193/M194). Opt-in via AGEZT_PRICING_STRICT=on: a request
	// for a model with no known price is refused BEFORE any provider call rather
	// than charged $0 (which would silently bypass the daily/task budget).
	// Known-free models (local/mock) still pass. Off by default.
	ec.strictPricing = strings.EqualFold(get(brand.EnvPrefix+"PRICING_STRICT"), "on")
	// Capability down-routing (M37). Opt-in via AGEZT_MODEL_DOWNROUTE=on: a
	// tools-bearing request to a tool-incapable model is remapped to a
	// tool-capable sibling in the same provider instead of being rejected
	// (M25). Pairs naturally with strict mode (reroute-if-possible, else
	// reject), but works independently too.
	// AGEZT_MODEL_DOWNROUTE_CROSS=on widens the substitute search to OTHER
	// registered+credentialed providers when the model's own provider has no
	// tool-capable sibling (M40). It implies down-routing. Without it, the
	// search stays same-provider only (M37).
	ec.crossDownRoute = strings.EqualFold(get(brand.EnvPrefix+"MODEL_DOWNROUTE_CROSS"), "on")
	ec.downRoute = ec.crossDownRoute || strings.EqualFold(get(brand.EnvPrefix+"MODEL_DOWNROUTE"), "on")

	// Opt-in LLM response cache (M888): AGEZT_LLM_CACHE_TTL=<duration> serves
	// an IDENTICAL completion request from memory within the TTL — no provider
	// call, no spend. Off when unset (an LLM is not a pure function; chat
	// regenerate wants fresh samples). Malformed = hard startup error.
	if spec := strings.TrimSpace(get(brand.EnvPrefix + "LLM_CACHE_TTL")); spec != "" {
		d, derr := time.ParseDuration(spec)
		if derr != nil || d < 0 {
			return ec, fmt.Errorf("%sLLM_CACHE_TTL: want a non-negative Go duration (e.g. 5m), got %q", brand.EnvPrefix, spec)
		}
		ec.respCacheTTL = d
	}
	return ec, nil
}

// registerAlternates is the ONE shared registration path for every non-primary
// provider: every OTHER credentialed + supported catalog provider is registered
// as a model-routable alternate (SPEC-15 §1), plus the ChatGPT subscription
// alternate when signed in. Boot calls it with replace=false (fresh registry,
// Registry.Register); Reload calls it with replace=true (Registry.Replace,
// then a stale-drop sweep removes alternates that lost eligibility — key
// revoked / provider gone from the catalog). Build failures are skipped, never
// fatal — a misconfigured alternate must not stop the daemon (boot) or the
// reload. Fallback entries are never touched by the sweep.
//
// Every registered provider — including ChatGPT — is wrapped in the M997
// middleware stack on BOTH paths (drift fix: the old reload path registered
// raw providers, so GEN_TEMPERATURE / EXTRACT_REASONING / SIMULATE_STREAMING
// silently stopped applying after any provider reload until restart).
//
// Returns the eligible set (catalog provider id → true, primary included) —
// the cross-provider down-route eligibility map.
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
