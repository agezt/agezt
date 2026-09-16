// SPDX-License-Identifier: MIT

// Package providerboot owns provider bootstrap for the daemon: primary
// selection, alternate registration, the governor's construction, and the
// hot-reload path. The unconfiguredProvider stub moved to
// providerboot_stub.go; the cross-provider down-route eligibleSet moved to
// providerboot_set.go; governor env parsing moved to
// providerboot_config.go. Day-211 god-file split. Public API unchanged.
package providerboot

import (
	"io"
	"os"
	"strconv"
	"strings"

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
