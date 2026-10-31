// SPDX-License-Identifier: MIT

package resume

// Provenance: The ticket row's CAPTURE rules, moved out of
//             kernel/runtime/resume.go in W3.76a (the durable run tickets
//             move into the runs module). The daemon reads the run's context;
//             this package decides what the ticket records.

import "github.com/agezt/agezt/kernel/contract/agentapi"

// Setting is one resolved dispatch value and WHERE it came from.
//
// The source is what decides reconstructibility: the agent profile's own
// default is reloaded on boot and can be reproduced; a per-run override
// cannot.
type Setting struct {
	// Set reports whether the run used a value for this axis at all.
	Set bool
	// SourceProfile is the agent profile that supplied the value. Empty means
	// the value came from a PER-RUN override (or from nothing).
	SourceProfile string
}

// Reconstructible reports whether this resolved value can be reproduced on
// resume.
//
// A value the run did not set is reconstructible (the profile supplies the
// default). A value the profile supplied is reconstructible only when THAT
// profile is the run's agent — the resumer reloads the agent named by the
// ticket, so a value from any other profile would come back different.
func (s Setting) Reconstructible(agentSlug string) bool {
	if !s.Set {
		return true
	}
	return s.SourceProfile != "" && s.SourceProfile == agentSlug
}

// Facts is what the daemon RESOLVED about a run when it writes the ticket.
//
// The daemon reads its own run context; this package decides what the ticket
// records. Every field is a fact about the run's dispatch, and each one is
// either reconstructible from the ticket plus the agent's profile (the agent's
// own defaults) or not (a per-run override).
type Facts struct {
	// Corr is the run's correlation id — the ticket's identity.
	Corr string

	// Intent is the run's intent, replayed by a resume that cannot continue
	// the conversation.
	Intent string

	// Kind classifies HOW the run was dispatched (plain, assured, retry), so
	// the resumer re-enters through the matching governed entry point.
	Kind string

	// AssureBudget is the assurance budget the run was dispatched with.
	AssureBudget int

	// AgentSlug is the named agent the run executed as. Empty means the run
	// ran under no named agent, so nothing about it is profile-derived.
	AgentSlug string

	// MaxCostMc is the run's resolved USD-microcent ceiling.
	MaxCostMc int64

	// TrustCeiling is the run's resolved trust ceiling, or nil when none was
	// ever set. A set value MUST be re-applied on resume so authority is
	// never silently regained.
	TrustCeiling *int

	// RunTimeoutMs is the run's per-run wall-clock budget in milliseconds,
	// or 0 when unbounded.
	RunTimeoutMs int64

	// Wake is the run's durable provenance — why the run exists. It is
	// flattened into the ticket so the row stays a flat record.
	Wake agentapi.WakeContext

	// System is the run's resolved system prompt.
	System Setting

	// Model is the run's resolved model.
	Model Setting

	// ToolsOverridden reports that the run carried a PER-RUN tool allowlist.
	// The ticket does not record which tools the run was allowed, so the
	// allowlist cannot be reconstructed.
	ToolsOverridden bool
}

// Build assembles the durable ticket for a run from the facts the daemon
// resolved, and decides whether the run may be resumed from it.
//
// The ticket records the run's dispatch context so a restart can continue it:
// the resolved governance (trust ceiling, cost cap, run timeout) is captured
// so resume neither loses a tightened ceiling nor guesses a cap, and the wake
// provenance is captured so the resumed run still knows why it exists.
//
// The Resumable flag is the module's verdict (JudgeResumability): a run whose
// dispatch context cannot be reconstructed faithfully is marked non-resumable,
// so the boot-time resumer cleans its ticket up instead of re-dispatching it
// under different constraints.
func Build(f Facts) *Ticket {
	t := &Ticket{
		Corr:         f.Corr,
		Intent:       f.Intent,
		Kind:         f.Kind,
		AssureBudget: f.AssureBudget,
		AgentSlug:    f.AgentSlug,
		MaxCostMc:    f.MaxCostMc,
		TrustCeiling: f.TrustCeiling,
		RunTimeoutMs: f.RunTimeoutMs,
		WakeSource:   f.Wake.Source,
		WakeReason:   f.Wake.Reason,

		WakeScheduleID:     f.Wake.ScheduleID,
		WakeStandingID:     f.Wake.StandingID,
		WakeStandingName:   f.Wake.StandingName,
		WakeTriggerSubject: f.Wake.TriggerSubject,

		Resumable: JudgeResumability(DispatchFacts{
			AgentSlug:       f.AgentSlug,
			System:          f.System,
			Model:           f.Model,
			ToolsOverridden: f.ToolsOverridden,
		}) == Resumable,

		Status: StatusActive,
	}
	return t
}
