// SPDX-License-Identifier: MIT

package skillapi

// Provenance: the skill PORTS — the store the file lives behind, the publisher
//             the Forge journals through, and the Curator that is the module's
//             whole public surface. New in the W3.55 slice.

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/llm"
)

// Publisher is the port through which the Forge journals a skill transition.
//
// The bus answers with the durable ENVELOPE, and the envelope carries the hash
// chain — the contract layer may not name it. The port answers with the
// event's ID, which is exactly what a skill records as its provenance
// (SourceEvent) and what `agt why` reads.
type Publisher interface {
	// Publish writes one event and answers its id.
	Publish(spec eventapi.Spec) (id string, err error)
}

// Store is the pure skill store: skills persisted by id, safe for concurrent
// use. The file behind it is the composition root's (W3.55) — the kernel holds
// this port and never opens a file of its own.
type Store interface {
	// Put upserts s by its ID.
	Put(sk Skill) error
	// Get returns the skill at id; the bool is false if absent.
	Get(id string) (Skill, bool, error)
	// All returns every skill, sorted deterministically by CreatedMS then ID.
	All() ([]Skill, error)
	// Count returns the number of active skills (the retrieval-pool size).
	Count() int
	// Close releases any held resources.
	Close() error
}

// Lifecycle is the port for the reversible half of the state machine: retire a
// skill, walk it back to its previous status, restore it, or hand it to another
// roster agent.
type Lifecycle interface {
	// Archive retires a skill.
	Archive(corr, id, reason string) error
	// Revert walks a skill back to its previous status and answers the status
	// it landed on.
	Revert(corr, id string) (string, error)
	// RestoreStatus moves a skill to an explicit status and answers the status
	// it was in and the one it landed on.
	RestoreStatus(corr, id string, target Status, reason string) (Status, Status, error)
	// Reassign hands a skill to another roster agent.
	Reassign(corr, id, newAgent string) (Skill, bool, error)
}

// Activator is the port through which a run pulls the skills that match it.
type Activator interface {
	// ActivateFor activates the active skills that match the intent for one
	// agent (its own private ones plus the shared pool).
	ActivateFor(corr, agentSlug, intent string, limit int) ([]Scored, error)
	// ActivateExplicitFor activates the named skills; the second answer is the
	// refs that could not be resolved.
	ActivateExplicitFor(corr, agentSlug, intent string, refs []string, limit int) ([]Scored, []string, error)
}

// Outcome is the port through which a finished run reports whether the skills
// it used actually helped.
type Outcome interface {
	// RecordOutcome journals the outcome of the given skill ids.
	RecordOutcome(corr string, ids []string, success bool)
}

// Proposer is the port for the self-improvement half: draft a new skill from a
// finished run, and judge the shadow skills against it.
type Proposer interface {
	// Propose runs one best-effort provider call over a task transcript and
	// stores a draft skill when the approach is reusable. It answers the
	// created draft ids.
	Propose(ctx context.Context, corr string, provider llm.Provider, model, intent, transcript string) ([]string, error)
	// ShadowEvaluate judges the shadow skills that match a finished run
	// against it.
	ShadowEvaluate(ctx context.Context, corr string, provider llm.Provider, model, intent, outcome string, limit int) error
}

// Observer is the port for the read-only views of the pool: its size and its
// health.
type Observer interface {
	// Count returns the number of active skills.
	Count() int
	// Hygiene reports which active skills look idle, so an operator can prune
	// dead weight from the retrieval pool.
	Hygiene(idleCutoffMs int64) (HygieneReport, error)
}

// Bundler is the port for a skill's on-disk bundle — the reference files and
// scripts that travel with it.
type Bundler interface {
	// Bundles returns the wired bundle store (nil when none is wired).
	Bundles() BundleStore
}

// Tuner is the port for the automatic lifecycle thresholds: the failure-driven
// auto-quarantine, the draft→shadow auto-staging and the shadow→active
// auto-promotion.
type Tuner interface {
	// SetAutoQuarantine tunes (or, with minFailures <= 0, disables) the
	// failure-driven auto-quarantine.
	SetAutoQuarantine(minFailures int, rate float64)
	// SetAutoShadow enables or disables draft→shadow auto-staging.
	SetAutoShadow(on bool)
	// SetAutoPromote tunes (or, with minWins <= 0, disables) shadow→active
	// auto-promotion.
	SetAutoPromote(minWins int, rate float64)
}

// Curator is the module's whole public surface — the Forge's verbs, as the
// layers above the module see them. It is composed of the narrow ports above
// so each one keeps a single meaning, and it is the face the kernel holds.
//
// It is deliberately NOT a widening of Forge: Forge stays the five-verb seam
// the agent-facing tool (a plugin) drives, and the plugin's own fakes keep
// working. The Curator is the wider face the kernel, the control plane and the
// app layer need.
type Curator interface {
	Forge
	Lifecycle
	Activator
	Outcome
	Proposer
	Observer
	Bundler
	Tuner
}
