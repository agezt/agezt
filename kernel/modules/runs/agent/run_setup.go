// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
)

// Run's prologue, extracted (refactor Phase 3.2). Every function here is pure or
// closure-local: it reads the config and the intent, and returns a value. None
// of them touch the loop's mutable state or publish events, so the loop body
// they were carved out of reads as "set up, then iterate" instead of 130 lines
// of preamble before the first `for`.

// normalizeLoopConfig resolves the zero-value defaults a caller may leave unset,
// so the loop body's arithmetic never has to re-ask "is this 0 meaningful?".
// Mutates through the pointer because Run owns its cfg copy by value.
//
// MaxAutoContinue is the subtle one (M833): 0 means "use the default budget",
// while NEGATIVE means "disabled" — the old fail-immediately-at-MaxIter
// behaviour — and is normalized to 0 here so the loop can compare against a
// plain count.
func normalizeLoopConfig(cfg *LoopConfig) {
	if cfg.MaxIter <= 0 {
		cfg.MaxIter = DefaultMaxIter
	}
	if cfg.MaxIdenticalToolCalls == 0 {
		cfg.MaxIdenticalToolCalls = DefaultMaxIdenticalToolCalls
	}
	if cfg.MaxAutoContinue == 0 {
		cfg.MaxAutoContinue = DefaultMaxAutoContinue
	}
	if cfg.MaxAutoContinue < 0 {
		cfg.MaxAutoContinue = 0
	}
	if cfg.AutoContinueWait == 0 {
		cfg.AutoContinueWait = DefaultAutoContinueWait
	}
	if cfg.Tools == nil {
		cfg.Tools = map[string]Tool{}
	}
}

// validateLoopConfig rejects a config the loop cannot run with at all. Separate
// from normalize because these are caller bugs (no provider, no bus, no actor)
// that must fail BEFORE task.received — a run that never started must not appear
// in the journal as one that failed.
func validateLoopConfig(cfg LoopConfig) error {
	switch {
	case cfg.Provider == nil:
		return fmt.Errorf("agent: provider required")
	case cfg.Bus == nil:
		return fmt.Errorf("agent: bus required")
	case cfg.Actor == "":
		return fmt.Errorf("agent: actor required")
	}
	return nil
}

// taskReceivedPayload builds the task.received provenance map: the intent plus
// every "where did this run come from" field the config carries (M93 images,
// M854 agent attribution, wake source/reason, the schedule/standing order that
// fired it, the trigger subject, and the parent run for delegations). Absent
// fields are omitted rather than emitted empty, so the journal payload stays
// readable and a consumer can distinguish "not applicable" from "".
func taskReceivedPayload(cfg LoopConfig, userIntent string) map[string]any {
	received := map[string]any{"intent": userIntent}
	if len(cfg.Images) > 0 {
		received["images"] = len(cfg.Images)
	}
	for k, v := range map[string]string{
		"agent":              cfg.Agent,
		"wake_source":        cfg.WakeSource,
		"wake_reason":        cfg.WakeReason,
		"schedule_id":        cfg.ScheduleID,
		"standing_id":        cfg.StandingID,
		"standing_name":      cfg.StandingName,
		"trigger_subject":    cfg.TriggerSubject,
		"parent_correlation": cfg.ParentCorrelation,
	} {
		if v != "" {
			received[k] = v
		}
	}
	return received
}

// initialMessages seeds the conversation: the intent as one user turn, or the
// resumed snapshot when the caller is continuing an interrupted run (M1002).
// The snapshot already opens with the original intent turn, so it is adopted
// as-is rather than prepended to — and it is COPIED so the loop's appends never
// mutate the caller's backing array.
func initialMessages(cfg LoopConfig, userIntent string) []Message {
	if len(cfg.PriorMessages) > 0 {
		return append([]Message(nil), cfg.PriorMessages...)
	}
	return []Message{{Role: RoleUser, Content: userIntent, Images: cfg.Images}}
}

// buildToolDefs lints every configured tool's schema once, up front, and returns
// the definitions to offer the model. Linting here (before task.received's
// sibling work) means a malformed schema fails the run immediately instead of
// surfacing as a provider 400 several iterations in.
func buildToolDefs(tools map[string]Tool) ([]ToolDef, error) {
	defs := make([]ToolDef, 0, len(tools))
	for name, t := range tools {
		def := t.Definition()
		if err := LintToolSchema(def); err != nil {
			return nil, fmt.Errorf("agent: tool %q schema lint failed: %w", name, err)
		}
		defs = append(defs, def)
	}
	return defs, nil
}

// newElisionSummarizer wraps cfg.SummarizeElided (M398) with a per-run cache
// keyed by the output, so each distinct tool output is summarised at most once,
// and swallows ctx/errors into "" so compaction always falls back to the head
// snippet. Returns nil when no summarizer is configured — zero extra provider
// calls on the common path.
func newElisionSummarizer(ctx context.Context, cfg LoopConfig) func(string) string {
	if cfg.SummarizeElided == nil {
		return nil
	}
	cache := map[string]string{}
	return func(output string) string {
		if s, ok := cache[output]; ok {
			return s
		}
		s, err := cfg.SummarizeElided(ctx, output)
		if err != nil {
			s = "" // fall back to the head snippet for this output
		}
		cache[output] = s
		return s
	}
}

// resolveDirectiveWindow returns the iteration span over which a directive-like
// untrusted observation keeps forcing the policy gate (the prompt-injection
// causal window). A non-positive configured value means "use the default".
func resolveDirectiveWindow(cfg LoopConfig) int {
	if cfg.DirectiveTaintWindow > 0 {
		return cfg.DirectiveTaintWindow
	}
	return DefaultDirectiveTaintWindow
}
