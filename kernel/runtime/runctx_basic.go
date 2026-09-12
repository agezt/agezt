// SPDX-License-Identifier: MIT

// Run context basic: cloneStringMap + With/From for trust/images/json/model/system/runTimeout/maxCost.
// Code extracted from runctx.go during the Day-69 god-file split. Public API unchanged.
package runtime


import (
	"context"
	"github.com/agezt/agezt/kernel/edict"
	"time"
)


func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// rootFromCtx returns the correlation of the ROOT run of a delegation tree (the
// top-level lead), propagated to every descendant so a tree-wide cap can be
// attributed to the whole tree rather than a single spawner. Empty when not in
// a delegated context.
func rootFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRoot).(string); ok {
		return v
	}
	return ""
}

// WithTrustCeiling returns a context capping autonomous tool-use at `ceiling` for
// the run started with it (SPEC-16 §4 initiative.max_trust). The policy hook
// consults it so a normally auto-allowed capability is downgraded to Ask (or
// Deny) within this run. ceiling >= LevelAllow is a no-op (no clamp). Used by the
// standing-order runner to bound an order's autonomy.
//
// The ceiling is monotonically TIGHTENING down a delegation tree: if the context
// already carries a tighter (lower) ceiling, that one is kept. A child run (e.g. a
// delegated sub-agent whose profile declares a looser TrustCeiling) can therefore
// never loosen the bound its parent was started with — only narrow it. Without
// this, WithAgentProfile re-applying a target profile's higher ceiling would
// overwrite a standing-order initiative cap and let delegation escape it
// (CWE-269, security finding VULN-001).
func WithTrustCeiling(ctx context.Context, ceiling edict.TrustLevel) context.Context {
	if ceiling >= edict.LevelAllow {
		// "No clamp" must not erase an existing tighter ceiling: leave ctx as-is so
		// any inherited cap survives.
		return ctx
	}
	if existing, ok := trustCeilingFromCtx(ctx); ok && existing < ceiling {
		ceiling = existing
	}
	return context.WithValue(ctx, ctxKeyTrustCeiling, ceiling)
}

func trustCeilingFromCtx(ctx context.Context) (edict.TrustLevel, bool) {
	v, ok := ctx.Value(ctxKeyTrustCeiling).(edict.TrustLevel)
	return v, ok
}

// WithImages returns a context carrying image-attachment references for the run
// started with it (M93). They flow into the agent loop's initial user message.
// Empty is a no-op. The caller (control plane) only sets this after the M91
// vision gate confirms the active model is vision-capable.
func WithImages(ctx context.Context, images []string) context.Context {
	if len(images) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyImages, images)
}

func imagesFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxKeyImages).([]string); ok {
		return v
	}
	return nil
}

// WithJSONMode returns a context requesting structured (JSON) output for the run
// started with it (M314). It flows into the agent loop's CompletionRequest.JSONMode,
// so a provider with a native JSON mode constrains its output. false is a no-op.
// Used by the OpenAI-compatible API to honour a client's response_format.
func WithJSONMode(ctx context.Context, jsonMode bool) context.Context {
	if !jsonMode {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyJSONMode, true)
}

func jsonModeFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyJSONMode).(bool)
	return v
}

// WithModel returns a context that overrides the model for the run started with
// it. Empty model is a no-op (the kernel's configured Model is used). The
// override flows into the agent loop's CompletionRequest.Model, so the selected
// provider serves exactly the requested model — the basis for per-request model
// selection from the OpenAI-compatible API.
func WithModel(ctx context.Context, model string) context.Context {
	if model == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyModel, model)
}

func modelFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyModel).(string); ok {
		return v
	}
	return ""
}

// WithSystem returns a context that overrides the base system prompt for the run
// started with it (M148-sibling). Empty is a no-op (the kernel's configured System
// is used). The override REPLACES the configured System; memory/world/skill
// injection still layer on top, so a one-off identity/instruction override can
// be set per run without losing what Agezt knows.
func WithSystem(ctx context.Context, system string) context.Context {
	if system == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeySystem, system)
}

func systemFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeySystem).(string); ok {
		return v
	}
	return ""
}

// WithRunTimeout returns a context that overrides the per-run wall-clock budget
// for the run started with it (a per-run counterpart to Config.MaxDuration / M31).
// d <= 0 is a no-op (the configured MaxDuration, if any, applies). Lets a single
// run be bounded without a daemon-wide cap (`agt run --timeout`).
func WithRunTimeout(ctx context.Context, d time.Duration) context.Context {
	if d <= 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRunTimeout, d)
}

func runTimeoutFromCtx(ctx context.Context) time.Duration {
	if v, ok := ctx.Value(ctxKeyRunTimeout).(time.Duration); ok {
		return v
	}
	return 0
}

// WithMaxCost returns a context that caps the cumulative provider spend (in
// USD-microcents) for the run started with it (M166) — the per-run cost analogue
// of WithRunTimeout. mc <= 0 is a no-op (uncapped). Lets a single run be bounded
// by money (`agt run --max-cost`) without a daemon-wide ceiling; the Governor's
// daily ceiling still applies on top.
func WithMaxCost(ctx context.Context, mc int64) context.Context {
	if mc <= 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyMaxCost, mc)
}

func maxCostFromCtx(ctx context.Context) int64 {
	if v, ok := ctx.Value(ctxKeyMaxCost).(int64); ok {
		return v
	}
	return 0
}
