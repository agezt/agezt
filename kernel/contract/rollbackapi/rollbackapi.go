// SPDX-License-Identifier: MIT

// Package rollbackapi is the contract the rollback engine and everyone who
// asks "what can I roll back?" agree on.
//
// The rollback catalogue is the daemon's list of **checkpoints**: the moments
// it can return to — a skill's status before it changed, a workflow before it
// ran, a file before it was written, a setting before it was set. The console
// lists them and applies them; the engine writes them.
//
// The rules of the road:
//
//   - Rows only. No store, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The checkpoint row lives HERE, not in the platform's store, because the
// console serves it and the platform's store writes it — and a row two
// surfaces name must be declared once. The platform's own type is an ALIAS of
// this one, so the two can never drift apart.
package rollbackapi

// Checkpoint is one moment the daemon can return to.
//
// The fields are declared in the order the wire speaks them, because the field
// order IS the member order of the JSON the console serves.
type Checkpoint struct {
	ID           string         `json:"id"`
	Kind         string         `json:"kind"`
	Action       string         `json:"action"`
	RunID        string         `json:"run_id,omitempty"`
	SubjectID    string         `json:"subject_id"`
	SubjectName  string         `json:"subject_name,omitempty"`
	BeforeStatus string         `json:"before_status,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Before       map[string]any `json:"before,omitempty"`
	CreatedMS    int64          `json:"created_ms"`
	AppliedMS    int64          `json:"applied_ms,omitempty"`
}

// List is the answer to "what can I roll back?": the checkpoints themselves,
// how many there are, and the run they were filtered by when one was asked
// for.
type List struct {
	Checkpoints []Checkpoint `json:"checkpoints"`
	Count       int          `json:"count"`
	RunID       string       `json:"run_id,omitempty"`
}

// Applied is the answer to "did the rollback happen?": whether it did, the
// checkpoint it was, and — when it did not — why, or what the restore said.
//
// Result carries the restore's own answer, which is another operation's row:
// the contract cannot name it, and the wire does not care how the console
// held it.
type Applied struct {
	Applied    bool           `json:"applied"`
	Checkpoint Checkpoint     `json:"checkpoint"`
	Reason     string         `json:"reason,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
}
