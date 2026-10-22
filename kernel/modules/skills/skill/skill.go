// SPDX-License-Identifier: MIT

package skill

// Package documentation lives in doc.go.

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
