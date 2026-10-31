// SPDX-License-Identifier: MIT

package resume

// Provenance: The ticket RULES, moved out of kernel/runtime/resume.go in
//             W3.76a (the durable run tickets move into the runs module).
//             The daemon reads the run's context; this package decides.

import (
	"context"
	"errors"
)

// The ticket RULES — the decisions about a run's durable ticket. They live
// here, with the ticket rows they judge, because a rule that owns a store is a
// MODULE's rule (the counterpart of W3.73's "a rule that owns no store is the
// platform's"). The daemon keeps the doors: it reads the run's context, hands
// the facts over, and publishes the events.
//
// Three rules decide a ticket's fate:
//
//  1. SCOPE — which runs carry a ticket at all. A ROOT run does; a delegated
//     (sub-agent) run does not, because its work is re-driven by resuming the
//     root rather than persisted separately.
//
//  2. RESUMABILITY — which of those may be RESUMED. A ticket is resumable
//     when the run's dispatch context can be RECONSTRUCTED faithfully from
//     the ticket plus the agent's profile. The profile's own defaults (its
//     system prompt, its model) are reloaded on boot, so they are fine. A
//     PER-RUN override (an explicit system prompt, an explicit model, a tool
//     allowlist) is recorded nowhere the resumer reads, so a run that used
//     one cannot be resumed faithfully — and resuming it under different
//     constraints is worse than not resuming it.
//
//  3. TERMINATION — when a run ends, does its ticket survive? A run cancelled
//     while the daemon is SUSPENDING was interrupted by a restart, not
//     finished, so its ticket is kept (the next boot re-dispatches it). Every
//     other terminal outcome — a clean finish, a genuine failure, an operator
//     cancel, a per-run timeout — deletes the ticket: none of those should
//     re-run on the next boot.

// CarriesTicket reports whether a run carries its own durable ticket.
//
// Only a ROOT run does. A delegated run's work is re-driven by resuming the
// root — the root's ticket already holds the conversation that led to the
// delegation — so persisting the child's ticket separately would duplicate the
// work and double-dispatch it.
//
// The parent correlation is empty for a root run and names the root for every
// descendant, so it is what decides the scope.
func CarriesTicket(parentCorrelation string) bool {
	return parentCorrelation == ""
}

// Disposition is what happens to a ticket when a run terminates.
type Disposition int

const (
	// DispositionDelete removes the ticket: the run reached a terminal outcome
	// that must not re-run.
	DispositionDelete Disposition = iota
	// DispositionKeep preserves the ticket: the run was interrupted by a
	// restart, so it is a resume candidate.
	DispositionKeep
)

// TerminalOutcome reports whether a finished run's ticket survives.
//
// A run cancelled while the daemon is suspending was INTERRUPTED (its ticket
// is kept for the restart); every other terminal outcome deletes the ticket —
// a clean finish, a genuine failure, an operator cancel, a per-run timeout —
// because none of those should re-run on the next boot.
//
// The caller says whether the daemon is suspending; this function says what
// the ticket's fate is. The rule is here because the ticket is the module's.
func TerminalOutcome(runErr error, suspending bool) Disposition {
	if suspending && (errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)) {
		return DispositionKeep
	}
	return DispositionDelete
}

// Resumability is the verdict on whether a run's dispatch context can be
// reconstructed from its ticket, and therefore whether the run may resume.
type Resumability int

const (
	// Resumable means the ticket faithfully records the run's dispatch
	// context: the run may be re-dispatched from it.
	Resumable Resumability = iota
	// NotResumable means the run used a per-run override the ticket cannot
	// reconstruct. The ticket is still written (it records what ran), but it
	// is cleaned up on boot rather than re-dispatched.
	NotResumable
)

// DispatchFacts is what the daemon knows about a run when it asks whether the
// run may resume. The daemon READS the run's context; this package DECIDES.
type DispatchFacts struct {
	// AgentSlug is the named agent the run executed as. Empty means the run
	// ran under no named agent, so nothing about it is profile-derived.
	AgentSlug string

	// System is the run's resolved system prompt.
	System Setting

	// Model is the run's resolved model.
	Model Setting

	// ToolsOverridden reports that the run carried a PER-RUN tool allowlist.
	// The ticket does not record which tools the run was allowed, so the
	// allowlist cannot be reconstructed.
	ToolsOverridden bool
}

// JudgeResumability answers whether the run may resume.
//
// The rule: a run may resume when every part of its dispatch context can be
// reconstructed from the ticket plus the agent's profile. The profile's own
// defaults (its system prompt, its model) are reloaded on boot, so they are
// fine. A per-run override is recorded nowhere the resumer reads, so a run
// that used one cannot be resumed faithfully — and resuming it under
// different constraints is worse than not resuming it.
func JudgeResumability(facts DispatchFacts) Resumability {
	if facts.ToolsOverridden {
		return NotResumable
	}
	if !facts.System.Reconstructible(facts.AgentSlug) {
		return NotResumable
	}
	if !facts.Model.Reconstructible(facts.AgentSlug) {
		return NotResumable
	}
	return Resumable
}
