// SPDX-License-Identifier: MIT

// Per-run context plumbing: the ctxKey space, the With*/fromCtx helpers
// that carry actor, correlation, model, budgets, agent identity, and
// policy knobs into a run, and the agent-profile application. Split from
// runtime.go (Phase 3.1 A2).

package runtime

import (
	"context"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/memory"
	"github.com/agezt/agezt/kernel/roster"
)

// per-run context keys used by RunWith → policyHook to carry the
// actor/correlation IDs into approval.Submit so audit events stay
// linked to the originating task.
type ctxKey int

const (
	ctxKeyActor ctxKey = iota
	ctxKeyCorrelation
	ctxKeyModel
	ctxKeyImages
	ctxKeySystem
	ctxKeyRunTimeout
	ctxKeyTools
	ctxKeyMaxCost
	ctxKeyJSONMode
	ctxKeyTrustCeiling
	ctxKeyRoot
	ctxKeyModelChain
	ctxKeyAgentIdent
	ctxKeySystemAgent
	ctxKeyAgentLifecycle
	ctxKeyAgentRetryPolicy
	ctxKeyAgentToolPolicy
	ctxKeyAgentConfigOverrides
	ctxKeyAgentNoisePolicy
	ctxKeyWakeContext
	ctxKeyAutoApproveCaps
	ctxKeyTrustedObservations
	ctxKeyResumeOwned // M1002: a resume ticket for this corr is already owned by an outer frame
	ctxKeyResumeSeed  // M1002: prior conversation + iter to seed a resumed run
)

// agentIdent carries a named agent's identity + daily ceiling for the
// Governor's per-agent ledger (M793).
type agentIdent struct {
	slug    string
	dailyMc int64
}

type agentToolPolicy struct {
	allow []string
	deny  []string
}

const agentNoiseStateNS = "agent_noise"

type agentNoisePolicy struct {
	silentOnSuccess      bool
	disableMemoryWrites  bool
	minNotifySeverity    string
	minNotifyIntervalSec int
}

type agentNoiseState struct {
	LastNotifyMS    int64 `json:"last_notify_ms"`
	PendingNotifyMS int64 `json:"pending_notify_ms,omitempty"`
}

const agentNoisePendingNotifyTTL = 5 * time.Minute

// WakeContext is durable provenance for why a run exists. It is stamped on
// task.received by agent.Run and intentionally kept separate from the prompt.
type WakeContext struct {
	Source            string
	Reason            string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
}

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

// WithAutoApproveCapabilities marks a set of capabilities to auto-grant when
// the policy would otherwise prompt for HITL approval, for THIS run and every
// sub-agent it spawns (the context value rides the delegation tree). caps is a
// set of edict capability strings (e.g. {"tool.forge","code.exec"}). This is a
// session-scoped operator grant — e.g. the chat "auto-approve Tool Forge for
// this session" toggle when standing up an agent army — NOT a daemon-wide policy
// change. It never overrides a hard-deny (those resolve to deny, not approval).
// Empty leaves the context unchanged.
func WithAutoApproveCapabilities(ctx context.Context, caps map[string]bool) context.Context {
	if len(caps) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyAutoApproveCaps, caps)
}

func mergeAutoApproveCapabilities(ctx context.Context, caps map[string]bool) map[string]bool {
	if len(caps) == 0 {
		return nil
	}
	out := make(map[string]bool, len(caps))
	if existing, ok := ctx.Value(ctxKeyAutoApproveCaps).(map[string]bool); ok {
		for c, on := range existing {
			if on {
				out[c] = true
			}
		}
	}
	for c, on := range caps {
		if on {
			out[c] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// autoApproveCap reports whether capability c is in this run's auto-approve set.
func autoApproveCap(ctx context.Context, c string) bool {
	if v, ok := ctx.Value(ctxKeyAutoApproveCaps).(map[string]bool); ok {
		return v[c]
	}
	return false
}

// PromptInjectionMode selects how the prompt-injection guard handles an
// effectful action downstream of directive-like untrusted content.
type PromptInjectionMode int

const (
	// PromptInjectionOn routes the action to HITL approval.
	PromptInjectionOn PromptInjectionMode = iota
	// PromptInjectionWarn allows the action but journals a warning (default).
	PromptInjectionWarn
	// PromptInjectionOff disables the active intervention entirely.
	PromptInjectionOff
)

// ParsePromptInjectionMode maps an operator string (AGEZT_PROMPT_INJECTION_GUARD)
// to a mode. "off"/"0"/"false" → Off, "warn"/"warning"/"audit" → Warn, anything
// else (including empty) → Warn. Operators who want a live approval stop on
// directive-like untrusted content set "on"/"block"/"prompt".
func ParsePromptInjectionMode(s string) PromptInjectionMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "off", "0", "false", "no":
		return PromptInjectionOff
	case "on", "1", "true", "yes", "block", "prompt":
		return PromptInjectionOn
	case "", "warn", "warning", "audit":
		return PromptInjectionWarn
	default:
		return PromptInjectionWarn
	}
}

// WithTrustedObservations marks a run as one whose untrusted-observation content
// the operator has chosen to trust (e.g. the chat "trust this run's web content"
// toggle). It downgrades the prompt-injection guard from blocking to warn FOR
// THIS RUN and its sub-agents, so a deliberately operator-driven agentic task
// isn't interrupted for every action — without changing the daemon-wide posture
// or touching any hard-deny. Never affects the F4 floor or SSRF/budget guards.
func WithTrustedObservations(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyTrustedObservations, true)
}

// trustedObservations reports whether this run carries the operator's
// trust-this-run grant.
func trustedObservations(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeyTrustedObservations).(bool)
	return v
}

// WithAgentProfile applies a roster profile to a run's context (M790): the
// soul becomes the system override, the model + ordered fallbacks become the
// run's model chain, and the memory scope follows the identity (M786). The
// per-run cost ceiling is NOT applied here — callers layer it so their own
// explicit budget wins (mirrors handleRun's precedence). Used by the standing
// runner so an order can fire AS a named agent; handleRun keeps its inline
// application (its model resolves before the vision gate).
func WithAgentProfile(ctx context.Context, p roster.Profile) context.Context {
	noise := effectiveAgentNoisePolicy(p)
	if p.System {
		ctx = context.WithValue(ctx, ctxKeySystemAgent, true)
	}
	if sys := agentProfileSystem(p); sys != "" {
		ctx = WithSystem(ctx, sys)
	}
	if p.Lifecycle.Mode != "" || p.Lifecycle.RetireOnComplete {
		ctx = context.WithValue(ctx, ctxKeyAgentLifecycle, p.Lifecycle)
	}
	if p.RetryPolicy != nil {
		cp := *p.RetryPolicy
		cp.RetryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		ctx = context.WithValue(ctx, ctxKeyAgentRetryPolicy, cp)
	}
	if len(p.ToolAllow) > 0 || len(p.ToolDeny) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentToolPolicy, agentToolPolicy{
			allow: append([]string(nil), p.ToolAllow...),
			deny:  append([]string(nil), p.ToolDeny...),
		})
	}
	if noise != (agentNoisePolicy{}) {
		ctx = context.WithValue(ctx, ctxKeyAgentNoisePolicy, noise)
	}
	if len(p.ConfigOverrides) > 0 {
		ctx = context.WithValue(ctx, ctxKeyAgentConfigOverrides, cloneStringMap(p.ConfigOverrides))
	}
	if ceiling := strings.TrimSpace(p.TrustCeiling); ceiling != "" {
		if lvl, err := edict.ParseTrustLevel(ceiling); err == nil {
			ctx = WithTrustCeiling(ctx, lvl)
		}
	}
	primary := strings.TrimSpace(p.Model)
	if primary != "" {
		ctx = WithModel(ctx, primary)
	}
	if len(p.Fallbacks) > 0 {
		chain := []string{primary}
		if primary == "" {
			chain = nil
		}
		for _, m := range p.Fallbacks {
			if m = strings.TrimSpace(m); m != "" && m != primary {
				chain = append(chain, m)
			}
		}
		ctx = WithModelChain(ctx, chain)
	}
	scope := strings.TrimSpace(p.MemoryScope)
	if scope == "" {
		scope = p.Slug
	}
	ctx = memory.WithScope(ctx, scope)
	// The agent's working directory (M792): file/shell tools operate inside
	// this workspace subdirectory. Escape-proofed by the setter.
	ctx = agent.WithWorkdir(ctx, p.Workdir)
	// And its identity + daily ceiling for the Governor's ledger (M793).
	return WithAgentIdent(ctx, p.Slug, p.MaxDailyMc)
}

func effectiveAgentNoisePolicy(p roster.Profile) agentNoisePolicy {
	var out agentNoisePolicy
	if p.NoisePolicy != nil {
		out.silentOnSuccess = p.NoisePolicy.SilentOnSuccess
		out.disableMemoryWrites = p.NoisePolicy.DisableMemoryWrites
		out.minNotifySeverity = strings.ToLower(strings.TrimSpace(p.NoisePolicy.MinNotifySeverity))
		out.minNotifyIntervalSec = p.NoisePolicy.MinNotifyIntervalSec
	}
	if out.silentOnSuccess && notifySeverityRank(out.minNotifySeverity) < notifySeverityRank("warning") {
		out.minNotifySeverity = "warning"
	}
	if p.System {
		out.silentOnSuccess = true
		out.disableMemoryWrites = true
		if notifySeverityRank(out.minNotifySeverity) < notifySeverityRank("warning") {
			out.minNotifySeverity = "warning"
		}
		if out.minNotifyIntervalSec < 8*3600 {
			out.minNotifyIntervalSec = 8 * 3600
		}
	}
	return out
}

func agentNoisePolicyFromCtx(ctx context.Context) (agentNoisePolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentNoisePolicy).(agentNoisePolicy)
	return v, ok
}

func appendUniqueString(in []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return in
	}
	for _, x := range in {
		if strings.EqualFold(strings.TrimSpace(x), value) {
			return in
		}
	}
	return append(in, value)
}

// WithAgentIdent stamps the run with a named agent's identity and per-day
// spend ceiling (M793): every completion of the run is metered against the
// Governor's per-agent daily ledger and refused past the ceiling.
func WithAgentIdent(ctx context.Context, slug string, dailyMc int64) context.Context {
	if strings.TrimSpace(slug) == "" {
		return ctx
	}
	// Also stamp the agent slug under the kernel/agent key so provenance-aware
	// tools (memory, M851) can read who is acting via agent.AgentFromContext —
	// the runtime key here is private and additionally carries the daily ceiling.
	ctx = agent.WithAgent(ctx, slug)
	return context.WithValue(ctx, ctxKeyAgentIdent, agentIdent{slug: slug, dailyMc: dailyMc})
}

func agentIdentFromCtx(ctx context.Context) (string, int64) {
	if v, ok := ctx.Value(ctxKeyAgentIdent).(agentIdent); ok {
		return v.slug, v.dailyMc
	}
	return "", 0
}

func agentSlugFromCtx(ctx context.Context) string { s, _ := agentIdentFromCtx(ctx); return s }

func agentDailyMcFromCtx(ctx context.Context) int64 { _, d := agentIdentFromCtx(ctx); return d }

// WithWakeContext attaches run provenance to the next agent loop. Empty fields
// are omitted from the journal; callers can layer it with WithAgentProfile.
func WithWakeContext(ctx context.Context, w WakeContext) context.Context {
	w.Source = strings.TrimSpace(w.Source)
	w.Reason = strings.TrimSpace(w.Reason)
	w.ScheduleID = strings.TrimSpace(w.ScheduleID)
	w.StandingID = strings.TrimSpace(w.StandingID)
	w.StandingName = strings.TrimSpace(w.StandingName)
	w.TriggerSubject = strings.TrimSpace(w.TriggerSubject)
	w.ParentCorrelation = strings.TrimSpace(w.ParentCorrelation)
	if w == (WakeContext{}) {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyWakeContext, w)
}

func wakeContextFromCtx(ctx context.Context) WakeContext {
	v, _ := ctx.Value(ctxKeyWakeContext).(WakeContext)
	return v
}

func systemAgentFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySystemAgent).(bool)
	return v
}

func agentToolPolicyFromCtx(ctx context.Context) agentToolPolicy {
	v, _ := ctx.Value(ctxKeyAgentToolPolicy).(agentToolPolicy)
	return v
}

func agentRetryPolicyFromCtx(ctx context.Context) (roster.RetryPolicy, bool) {
	v, ok := ctx.Value(ctxKeyAgentRetryPolicy).(roster.RetryPolicy)
	if !ok {
		return roster.RetryPolicy{}, false
	}
	v.RetryOn = append([]string(nil), v.RetryOn...)
	return v, true
}

// AgentConfigOverrides returns the named agent's config-override map attached to
// a run context by WithAgentProfile. The returned map is a copy and safe for the
// caller to mutate. Nil means the run carries no agent-specific overrides.
func AgentConfigOverrides(ctx context.Context) map[string]string {
	v, _ := ctx.Value(ctxKeyAgentConfigOverrides).(map[string]string)
	return cloneStringMap(v)
}

// WithModelChain sets the run's per-agent ordered model fallback chain (M787):
// the Governor tries these models in order, overriding the task type's
// configured chain. Carries a named agent's own fallbacks (roster M783).
func WithModelChain(ctx context.Context, chain []string) context.Context {
	if len(chain) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyModelChain, chain)
}

func modelChainFromCtx(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxKeyModelChain).([]string); ok {
		return v
	}
	return nil
}

// WithTools restricts the run started with this context to the named tools only
// (a per-run allowlist). A non-nil slice — including an EMPTY one (no tools at all,
// for a pure-reasoning / safe one-off run) — activates the restriction; passing it
// is the only way to override, so an unrestricted run is simply one where this is
// never called. Names not registered are ignored.
func WithTools(ctx context.Context, allow []string) context.Context {
	return context.WithValue(ctx, ctxKeyTools, allow)
}

// toolsFromCtx returns the per-run tool allowlist and whether one was set. ok=false
// means "no restriction" (use all tools); ok=true with an empty/nil slice means
// "no tools".
func toolsFromCtx(ctx context.Context) ([]string, bool) {
	v, ok := ctx.Value(ctxKeyTools).([]string)
	return v, ok
}

func actorFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyActor).(string); ok {
		return v
	}
	return ""
}

func correlationFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}
