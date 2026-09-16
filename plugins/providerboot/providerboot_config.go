// SPDX-License-Identifier: MIT

// providerboot_config.go holds Boot-time env parsing for the
// governor knobs (govEnvConfig + governorConfigFromEnv). Carved
// out of providerboot.go during the Day-211 god-file split so
// providerboot.go can stay focused on type definitions and the
// Boot/Reload entry points. The unconfiguredProvider stub lives
// in providerboot_stub.go.
package providerboot

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/governor"
)

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
