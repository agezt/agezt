// SPDX-License-Identifier: MIT

package skill

// Package documentation lives in doc.go.
//
// The lifecycle ROWS — the status, the record, the create input — and the
// rules that govern them now live in the contract layer
// (kernel/contract/skillapi): the agent's skill tool is a PLUGIN, and a
// plugin may reach the contract and the platform layers and nothing else.
// The names below stay as aliases, so every existing caller is unchanged and
// there is one row and one spelling of it.

import "github.com/agezt/agezt/kernel/contract/skillapi"

// Status is the lifecycle state of a skill (SPEC-05 §5.2).
type Status = skillapi.Status

const (
	// StatusDraft is a freshly authored skill (by Forge or an operator);
	// never used in production.
	StatusDraft = skillapi.StatusDraft
	// StatusShadow runs alongside real execution for evaluation; not yet in
	// the retrieval pool. (v1: a manual gate; auto shadow-testing deferred.)
	StatusShadow = skillapi.StatusShadow
	// StatusActive is in the retrieval/injection pool.
	StatusActive = skillapi.StatusActive
	// StatusQuarantined was pulled from production by a regression or repeated
	// failure — operator-driven (`agt skill quarantine`) or automatic once an
	// active skill crosses the failure threshold (M387, Forge.RecordOutcome).
	// Quarantined skills are excluded from the retrieval pool (retrieve.go).
	StatusQuarantined = skillapi.StatusQuarantined
	// StatusArchived is retired; retained for lineage/audit.
	StatusArchived = skillapi.StatusArchived
)

// DefaultVersion is the semver a freshly proposed skill starts at.
const DefaultVersion = skillapi.DefaultVersion

// CanTransition reports whether from→to is a legal lifecycle edge.
func CanTransition(from, to Status) bool { return skillapi.CanTransition(from, to) }

// ValidStatus reports whether st is one of the persisted lifecycle states.
func ValidStatus(st Status) bool { return skillapi.ValidStatus(st) }

// PromoteTarget returns the next status `promote` advances to from the given
// state (draft→shadow→active), and whether a promotion is defined there.
func PromoteTarget(from Status) (Status, bool) { return skillapi.PromoteTarget(from) }
