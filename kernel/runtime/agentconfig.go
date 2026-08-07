// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// agentConfigIssue describes one malformed runtime override on an agent
// profile. Unknown AGEZT_* keys are allowed as generic config overlays; only
// the runtime knobs this process actually consumes are validated here.
type agentConfigIssue struct {
	Key   string
	Value string
	Issue string
}

// agentOverride is one runtime knob a named agent may retune for its own runs
// via Profile.ConfigOverrides.
//
// The table below is the SINGLE source of truth for the override surface: the
// doctor validates against it (agentRuntimeConfigIssues) and effectiveConfig
// applies it, so "which keys exist", "what shape is a valid value", and "which
// field does it move" can no longer disagree. Before this table that knowledge
// lived in three places — a key list, a validation switch, and a hand-written
// lookup at each point of use — which meant a key could pass validation (the
// doctor reporting the profile healthy) and then be silently ignored at run
// time because nobody had added the third copy.
type agentOverride struct {
	// Key is the AGEZT_* name, matched case-insensitively.
	Key string
	// Issue is what the doctor tells the operator when a value doesn't parse.
	// Phrased as the complete message so it reads naturally next to the key
	// ("AGEZT_MAX_ITER: must be an integer").
	Issue string
	// Apply parses raw and assigns it to cfg, reporting whether raw was
	// well-formed. Validation and application share this one function, so the
	// doctor can never call a value valid that the run then refuses (or the
	// reverse).
	Apply func(cfg *Config, raw string) bool
}

// agentOverrides is the complete set of per-agent runtime overrides.
//
// Model is included, but note it lands on Config.Model whose meaning
// effectiveConfig has already upgraded from "the boot seed" to "the live
// default" — see effectiveConfig.
var agentOverrides = []agentOverride{
	{"AGEZT_MODEL", "value is blank",
		overrideString(func(c *Config, v string) { c.Model = v })},
	{"AGEZT_MAX_ITER", issueInt,
		overrideInt(func(c *Config, v int) { c.MaxIter = v })},
	{"AGEZT_MAX_AUTO_CONTINUE", issueInt,
		overrideInt(func(c *Config, v int) { c.MaxAutoContinue = v })},
	{"AGEZT_AUTO_CONTINUE_WAIT", "must be a Go duration like 250ms, 2s, or 1m30s",
		overrideDuration(func(c *Config, v time.Duration) { c.AutoContinueWait = v })},
	{"AGEZT_PARALLEL_TOOLS", issueInt,
		overrideInt(func(c *Config, v int) { c.MaxParallelTools = v })},
	{"AGEZT_TOOL_DISCOVERY_MAX", issueInt,
		overrideInt(func(c *Config, v int) { c.ToolDiscoveryMax = v })},
	{"AGEZT_CONTEXT_BUDGET", issueInt,
		overrideInt(func(c *Config, v int) { c.ContextBudget = v })},
	{"AGEZT_OBSERVATION_DELTAS", issueBool,
		overrideBool(func(c *Config, v bool) { c.ObservationDeltas = v })},
	{"AGEZT_DISABLE_HEURISTIC_BYPASS", issueBool,
		overrideBool(func(c *Config, v bool) { c.DisableHeuristicBypass = v })},
}

const (
	issueInt  = "must be an integer"
	issueBool = "must be a boolean like true/false or 1/0"
)

// The overrideX helpers turn a typed setter into an Apply: parse once, assign
// only on success, so a malformed value leaves the config at its inherited
// value rather than zeroing the knob.

func overrideString(set func(*Config, string)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigStringValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func overrideInt(set func(*Config, int)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigIntValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func overrideBool(set func(*Config, bool)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigBoolValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func overrideDuration(set func(*Config, time.Duration)) func(*Config, string) bool {
	return func(c *Config, raw string) bool {
		v, ok := agentConfigDurationValue(raw)
		if ok {
			set(c, v)
		}
		return ok
	}
}

func agentConfigOverrideRaw(overrides map[string]string, key string) (string, bool) {
	if overrides == nil {
		return "", false
	}
	raw, ok := overrides[strings.TrimSpace(strings.ToUpper(key))]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(raw), true
}

func agentConfigStringValue(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	return raw, true
}

func agentConfigBoolValue(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enabled":
		return true, true
	case "0", "false", "no", "off", "disabled":
		return false, true
	default:
		return false, false
	}
}

func agentConfigIntValue(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return n, true
}

func agentConfigDurationValue(raw string) (time.Duration, bool) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return d, true
}

// applyAgentOverrides applies every override present in overrides to cfg and
// returns the ones whose value did not parse. Malformed values are skipped, not
// fatal: a typo in one knob must not strip an agent of the rest of its config,
// and the doctor surfaces the typo to the operator.
func applyAgentOverrides(cfg *Config, overrides map[string]string) []agentConfigIssue {
	if len(overrides) == 0 {
		return nil
	}
	var issues []agentConfigIssue
	for _, ov := range agentOverrides {
		raw, ok := agentConfigOverrideRaw(overrides, ov.Key)
		if !ok {
			continue
		}
		if !ov.Apply(cfg, raw) {
			issues = append(issues, agentConfigIssue{Key: ov.Key, Value: raw, Issue: ov.Issue})
		}
	}
	return issues
}

// agentRuntimeConfigIssues reports the malformed runtime overrides on a profile
// (the agent doctor's config check). It applies them to a throwaway Config so
// validation exercises the exact code path a run would.
func agentRuntimeConfigIssues(overrides map[string]string) []agentConfigIssue {
	var scratch Config
	return applyAgentOverrides(&scratch, overrides)
}

// effectiveConfig is the config ONE run actually sees: the daemon-wide Config,
// updated to the operator's live edits, then retuned by the running agent's own
// overrides. Every consumer inside a run should read this rather than k.cfg.
//
// Two layers sit between k.cfg and the truth, and reading k.cfg directly skips
// both:
//
//   - Live edits. Model and System are hot-swappable without a restart —
//     SetModel on provider reload (M816), SetSystem from the persona surface
//     (M710) — so on k.cfg they are only the BOOT SEED. Six call sites had
//     drifted onto that seed, which meant that after an operator rotated a
//     provider key or switched models, root runs used the new model while
//     delegations, workflow LLM nodes, workflow drafting, and memory
//     distillation kept asking for the old one — typically the model that was
//     just de-keyed, so those paths failed while chat looked fine.
//   - Per-agent overrides. A named agent's ConfigOverrides retune these knobs
//     for its own runs.
//
// The returned Config is a copy, but its maps, slices, and funcs are shared with
// k.cfg — callers must treat those as read-only. Takes configMu (SetCouncilMembers
// writes into k.cfg under it), so never call this while already holding it.
func (k *Kernel) effectiveConfig(ctx context.Context) Config {
	k.configMu.Lock()
	cfg := k.cfg
	cfg.Model = k.model // live default, not the boot seed
	cfg.System = k.system
	k.configMu.Unlock()

	// Malformed values are ignored here; the agent doctor is what tells the
	// operator about them (agentRuntimeConfigIssues), and a run should not be
	// derailed by a typo in one knob.
	applyAgentOverrides(&cfg, AgentConfigOverrides(ctx))
	return cfg
}

// resolveRunModel picks the model a run should ask for, given its effective
// config. A per-run WithModel override wins and is reported as EXPLICIT — the
// caller then carries it as the per-request model chain so the governor's
// per-task chain can't silently reroute an operator's deliberate pick (M931).
// Otherwise the effective config already holds the live daemon default with any
// AGEZT_MODEL profile override applied, so there is nothing left to layer.
func resolveRunModel(ctx context.Context, cfg Config) (model string, explicit bool) {
	if m := modelFromCtx(ctx); m != "" {
		return m, true
	}
	return cfg.Model, false
}
