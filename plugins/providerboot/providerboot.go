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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/compat"
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
