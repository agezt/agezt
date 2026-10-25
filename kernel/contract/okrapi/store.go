// SPDX-License-Identifier: MIT

package okrapi

// Store is the port the daemon's OKR library implements: the durable set of
// objectives and their key results, one JSON file rewritten atomically on
// change.
//
// The port lives in the contract layer because three layers have to speak it
// and none of them may name the module's implementation:
//
//   - the kernel holds it (cfg.OKRStore), creates goals, links tasks to key
//     results and flips a goal's status when its rollup crosses the target;
//   - the app layer's service reads it (list, show) and reaches the kernel's
//     writer for the mutations;
//   - the control plane reaches it through the kernel's accessor.
//
// The daemon opens the file and hands the kernel this port; a host that was
// given none holds a nil port and reports that the library is unavailable
// rather than calling into nothing.
//
// The port is the module's whole public surface — nine verbs, no more. A
// wider surface would let a caller reach past the library's own rules (the
// store validates, assigns ids and protects the linked-task set); a narrower
// one would force a second shape to be kept in step with this one.
//
// The verbs that stamp a row take the CALLER's clock as nowMS: the contract
// layer may not name a clock (archcheck's principle P4), and a port that
// reads one cannot be tested deterministically.
type Store interface {
	// Create inserts a new objective and returns it.
	Create(spec CreateSpec, nowMS int64) (Objective, error)
	// Get returns one objective by id; the bool says whether it was there.
	Get(id string) (Objective, bool)
	// List returns the objectives matching the filter, newest first.
	List(f Filter) []Objective
	// AddKeyResult appends a key result to an objective.
	AddKeyResult(objID, title string, target int, nowMS int64) (Objective, error)
	// LinkTask attaches a workboard task id to a key result (idempotent).
	LinkTask(objID, krID, taskID string, nowMS int64) (Objective, error)
	// UnlinkTask removes a task id from a key result (idempotent).
	UnlinkTask(objID, krID, taskID string, nowMS int64) (Objective, error)
	// SetStatus transitions an objective's status (achieved/active/archived).
	SetStatus(id string, status Status, nowMS int64) (Objective, error)
	// Archive marks an objective archived.
	Archive(id string, nowMS int64) (Objective, error)
	// ObjectivesForTask returns the ids of the objectives that link taskID
	// under any key result — the set to recompute when a task is proven.
	ObjectivesForTask(taskID string) []string
}
