// SPDX-License-Identifier: MIT

package workflowapi

// Store is the port through which the stored workflows are read.
//
// The implementation — the on-disk registry in the workflows module —
// satisfies it; the agent-facing tool never names the implementation, because
// it is a plugin.
type Store interface {
	// Get returns one workflow by id or name; the bool is false if absent.
	Get(ref string) (Workflow, bool)
	// List returns every stored workflow.
	List() []Workflow
}

// HostStore is the port through which the HOST drives the workflow library:
// the read side plus the four lifecycle verbs the engine drives.
//
// The width is the host's, and the read port stays narrow: the agent-facing
// tool reads (Get, List) and nothing more — a tool that could remove or
// disable a stored workflow would be editing the daemon's library.
type HostStore interface {
	Store
	// Save upserts a workflow by name; the bool is false when the reference
	// already names another workflow.
	Save(w Workflow) (Workflow, bool, error)
	// Restore restores a checkpointed workflow.
	Restore(w Workflow) (Workflow, bool, error)
	// SetEnabled arms or disarms a workflow's triggers.
	SetEnabled(ref string, enabled bool) (Workflow, error)
	// Remove deletes a workflow; it answers the workflow that was removed and
	// whether there was one.
	Remove(ref string) (Workflow, bool, error)
}
