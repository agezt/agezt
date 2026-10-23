// SPDX-License-Identifier: MIT

package cadenceapi

// Provenance: Cadence store port: the slice of the schedule store the agent's
//             schedule tool drives. The port lives in the contract layer
//             because the tool is a PLUGIN: a plugin may reach the contract
//             and the platform layers and nothing else, so a store a plugin has
//             to name cannot live in a module's implementation package
//             (kernel/cadence). The store itself stays in the module — it is
//             the implementation, and the module keeps it.
//
// The tool does not need the whole store: it adds schedules, retargets them
// and lists them. The port says exactly that, so the module's store is free to
// grow methods the tool never calls.

import (
	"encoding/json"
	"time"
)

// Store is the schedule-store port the agent-facing schedule tool drives. The
// module's *cadence.Store implements it; the tool takes the port, so the
// plugin names the contract and never the module that holds the
// implementation.
type Store interface {
	// Add records an interval schedule (fires every interval).
	Add(intent string, interval time.Duration, model, source string, now time.Time) (Entry, error)
	// AddDaily records a schedule that fires at a wall-clock time on the given
	// weekday mask.
	AddDaily(intent string, atMinutes, days int, tz, model, source string, now time.Time) (Entry, error)
	// AddOnce records a schedule that fires exactly once.
	AddOnce(intent string, at time.Time, model, source string, now time.Time) (Entry, error)
	// AddContinuous records a never-ending loop, re-anchored after each run.
	AddContinuous(intent string, cooldown time.Duration, model, source string, now time.Time) (Entry, error)
	// SetAssure stamps the do-it-for-sure budget onto an existing entry.
	SetAssure(id string, n int) (bool, error)
	// SetAgent binds the entry to the roster slug it fires as.
	SetAgent(id, agent string) (bool, error)
	// SetWorkflowTarget retargets the entry at a stored workflow.
	SetWorkflowTarget(id, ref string, payload json.RawMessage) (bool, error)
	// SetSystemTaskTarget retargets the entry at a daemon maintenance task.
	SetSystemTaskTarget(id, task string) (bool, error)
	// SetToolTarget retargets the entry at a registered tool.
	SetToolTarget(id, tool string, payload json.RawMessage) (bool, error)
	// Remove deletes an entry.
	Remove(id string) (bool, error)
	// List returns every stored entry.
	List() []Entry
}
