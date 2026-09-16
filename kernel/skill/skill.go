// SPDX-License-Identifier: MIT

// Package skill implements "Forge v1" (SPEC-05 §4–5): the agent learns
// reusable, named procedures ("skills") from what it does, and those skills
// are governed through a journaled, reversible state machine instead of
// straight-to-active markdown. This is the "Curator-killer" — Agezt matches
// Hermes's learning loop and beats it on auditability and reversibility: every
// skill mutation is a content-addressed, hash-chained event, so you can ask
// why a skill exists, when it was promoted, and undo it (`agt skill revert`).
//
// Two layers, mirroring kernel/memory and kernel/worldmodel:
//
//   - Store (this file) is a pure, file-backed record store — no bus, no
//     journaling — owning content-addressing and the legal-transition table.
//     A CobaltDB-class engine can replace it behind the Store interface later.
//   - Forge (forge.go) wraps a Store with the kernel bus so every transition
//     (create/promote/quarantine/revert/activate) is a durable-before-publish
//     event carrying the run's correlation_id (SPEC-05 §5.3).
//
// Content-addressing is versioning: a skill's id is BLAKE3(name\0body), so
// editing the body yields a NEW record (a new version) with Lineage pointing
// at its parent — never a destructive edit (§4.3). Status and metrics are
// mutable metadata on the record; lifecycle transitions don't change the id.
//
// Concurrency: a single Store instance is safe for concurrent use.
package skill


// Status is the lifecycle state of a skill (SPEC-05 §5.2).
type Status string

const (
	// StatusDraft is a freshly authored skill (by Forge or an operator);
	// never used in production.
	StatusDraft Status = "draft"
	// StatusShadow runs alongside real execution for evaluation; not yet in
	// the retrieval pool. (v1: a manual gate; auto shadow-testing deferred.)
	StatusShadow Status = "shadow"
	// StatusActive is in the retrieval/injection pool.
	StatusActive Status = "active"
	// StatusQuarantined was pulled from production by a regression or repeated
	// failure — operator-driven (`agt skill quarantine`) or automatic once an
	// active skill crosses the failure threshold (M387, Forge.RecordOutcome).
	// Quarantined skills are excluded from the retrieval pool (retrieve.go).
	StatusQuarantined Status = "quarantined"
	// StatusArchived is retired; retained for lineage/audit.
	StatusArchived Status = "archived"
)

// DefaultVersion is the semver a freshly proposed skill starts at.
const DefaultVersion = "0.1.0"

// legalTransitions encodes the state machine (SPEC-05 §5.2). promote walks
// draft→shadow→active; quarantine pulls an active/shadow skill; archive retires
// a draft/shadow/quarantined skill; revert is handled specially by Forge
// (it appends a reversal rather than being a plain edge). A skill can always be
// archived from any non-archived state for operator cleanup.
var legalTransitions = map[Status]map[Status]struct{}{
	StatusDraft:       {StatusShadow: {}, StatusArchived: {}},
	StatusShadow:      {StatusActive: {}, StatusQuarantined: {}, StatusArchived: {}},
	StatusActive:      {StatusQuarantined: {}, StatusArchived: {}},
	StatusQuarantined: {StatusActive: {}, StatusArchived: {}},
	StatusArchived:    {},
}

// CanTransition reports whether from→to is a legal lifecycle edge.
func CanTransition(from, to Status) bool {
	dests, ok := legalTransitions[from]
	if !ok {
		return false
	}
	_, ok = dests[to]
	return ok
}

// ValidStatus reports whether st is one of the persisted lifecycle states.
func ValidStatus(st Status) bool {
	switch st {
	case StatusDraft, StatusShadow, StatusActive, StatusQuarantined, StatusArchived:
		return true
	default:
		return false
	}
}

// PromoteTarget returns the next status `promote` advances to from the given
// state (draft→shadow→active), and whether a promotion is defined there.
func PromoteTarget(from Status) (Status, bool) {
	switch from {
	case StatusDraft:
		return StatusShadow, true
	case StatusShadow:
		return StatusActive, true
	case StatusQuarantined:
		return StatusActive, true // un-quarantine back into production
	default:
		return from, false
	}
}
