// SPDX-License-Identifier: MIT

package workflowapi

// Store is the port through which the stored workflows are read.
//
// The implementation — the on-disk registry in the extensions module —
// satisfies it; the agent-facing tool never names the implementation, because
// it is a plugin.
type Store interface {
	// Get returns one workflow by id or name; the bool is false if absent.
	Get(ref string) (Workflow, bool)
	// List returns every stored workflow.
	List() []Workflow
}
