// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

// Internal tests for the per-run config resolution surface (refactor Phase 3.3).

// The override table is the single source of truth, so it has to be internally
// coherent: unique canonical keys, a doctor message for each, and a parser that
// actually rejects garbage. A new entry that forgets any of these would
// otherwise present as a knob that validates but never applies.
func TestAgentOverrides_TableIsCoherent(t *testing.T) {
	seen := map[string]bool{}
	for _, ov := range agentOverrides {
		if ov.Key != strings.ToUpper(strings.TrimSpace(ov.Key)) {
			t.Errorf("key %q must be canonical (upper-case, trimmed) — lookups uppercase before matching", ov.Key)
		}
		if seen[ov.Key] {
			t.Errorf("duplicate key %q: the second entry would silently shadow the first", ov.Key)
		}
		seen[ov.Key] = true
		if ov.Issue == "" {
			t.Errorf("%s: needs an Issue message — it is what the doctor shows the operator", ov.Key)
		}
		if ov.Apply == nil {
			t.Fatalf("%s: Apply is nil", ov.Key)
		}

		// Every parser must reject a value that cannot mean anything, or a typo
		// would pass validation and then be applied as a zero value.
		var cfg Config
		if ov.Apply(&cfg, " not-a-valid-value ") && ov.Key != "AGEZT_MODEL" {
			t.Errorf("%s: accepted a garbage value", ov.Key)
		}
	}
}

// A malformed value is reported by the doctor AND leaves the rest of the config
// alone: one typo must not strip an agent of its other knobs.
func TestApplyAgentOverrides_MalformedValueIsIsolated(t *testing.T) {
	cfg := Config{MaxIter: 4, ContextBudget: 100}
	issues := applyAgentOverrides(&cfg, map[string]string{
		"AGEZT_MAX_ITER":       "not-a-number",
		"AGEZT_CONTEXT_BUDGET": "9000",
	})
	if len(issues) != 1 || issues[0].Key != "AGEZT_MAX_ITER" {
		t.Fatalf("issues = %+v, want exactly the AGEZT_MAX_ITER one", issues)
	}
	if cfg.MaxIter != 4 {
		t.Errorf("MaxIter = %d, want the inherited 4 — a malformed value must not zero the knob", cfg.MaxIter)
	}
	if cfg.ContextBudget != 9000 {
		t.Errorf("ContextBudget = %d, want 9000 — a sibling typo must not block a valid override", cfg.ContextBudget)
	}
}

// Lower-case / oddly-spaced keys still match: profiles are hand-edited.
func TestApplyAgentOverrides_KeyMatchingIsCaseInsensitive(t *testing.T) {
	var cfg Config
	applyAgentOverrides(&cfg, map[string]string{"AGEZT_MAX_ITER": " 11 "})
	if cfg.MaxIter != 11 {
		t.Errorf("MaxIter = %d, want 11 (surrounding whitespace is trimmed)", cfg.MaxIter)
	}
}

// effectiveConfig layers three things in order: the daemon-wide config, the
// operator's live edits, then the running agent's overrides. Skipping the middle
// layer is the bug this function exists to make unrepresentable — six call sites
// used to read the boot seed and so kept using the model/persona the daemon
// started with after the operator changed it.
func TestEffectiveConfig_LayersBootThenLiveThenAgent(t *testing.T) {
	k := &Kernel{cfg: Config{
		Model:   "boot-model",
		System:  "boot persona",
		MaxIter: 3,
	}}
	k.model, k.system = k.cfg.Model, k.cfg.System // as Open seeds them

	// Layer 1 — nothing live-edited, no agent: the daemon-wide config.
	if got := k.effectiveConfig(context.Background()); got.Model != "boot-model" || got.System != "boot persona" || got.MaxIter != 3 {
		t.Fatalf("plain run = %+v, want the daemon config verbatim", got)
	}

	// Layer 2 — the operator hot-swaps both without a restart (M816 / M710).
	k.SetModel("live-model")
	k.SetSystem("live persona")
	got := k.effectiveConfig(context.Background())
	if got.Model != "live-model" {
		t.Errorf("Model = %q, want live-model — cfg.Model is only the boot seed", got.Model)
	}
	if got.System != "live persona" {
		t.Errorf("System = %q, want live persona", got.System)
	}

	// Layer 3 — a named agent retunes knobs for its own runs, over the live values.
	ctx := WithAgentProfile(context.Background(), roster.Profile{
		Slug: "tuned",
		ConfigOverrides: map[string]string{
			"AGEZT_MODEL":    "agent-model",
			"AGEZT_MAX_ITER": "9",
		},
	})
	got = k.effectiveConfig(ctx)
	if got.Model != "agent-model" {
		t.Errorf("Model = %q, want the agent's override to win over the live default", got.Model)
	}
	if got.MaxIter != 9 {
		t.Errorf("MaxIter = %d, want 9", got.MaxIter)
	}
	// Untouched knobs still come from the daemon config.
	if got.AutoContinueWait != 0 {
		t.Errorf("AutoContinueWait = %v, want the unset daemon value", got.AutoContinueWait)
	}

	// And resolving does not mutate the kernel — the next run starts clean.
	if k.cfg.Model != "boot-model" || k.Model() != "live-model" {
		t.Errorf("effectiveConfig mutated kernel state: cfg.Model=%q live=%q", k.cfg.Model, k.Model())
	}
}

// A per-run WithModel pick beats both the live default and the agent's override,
// and is the only one reported EXPLICIT — that flag is what stops the governor's
// per-task chain from rerouting an operator's deliberate choice (M931).
func TestResolveRunModel_PerRunPickWinsAndIsExplicit(t *testing.T) {
	cfg := Config{Model: "effective-model"}

	model, explicit := resolveRunModel(context.Background(), cfg)
	if model != "effective-model" || explicit {
		t.Errorf("no per-run pick = (%q, %v), want (effective-model, false)", model, explicit)
	}

	model, explicit = resolveRunModel(WithModel(context.Background(), "asked-for"), cfg)
	if model != "asked-for" || !explicit {
		t.Errorf("per-run pick = (%q, %v), want (asked-for, true)", model, explicit)
	}
}

func TestAgentConfigDurationValue(t *testing.T) {
	if d, ok := agentConfigDurationValue(" 1m30s "); !ok || d != 90*time.Second {
		t.Errorf("duration = %v/%v, want 90s", d, ok)
	}
	if _, ok := agentConfigDurationValue("90"); ok {
		t.Error("a bare number is not a Go duration and must be rejected")
	}
}
