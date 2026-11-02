// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the WORKFLOW ENGINE — the ratchet of W3.76u.
//
// The workflows module owns the durable store, the graph engine, the trigger
// runner and the copilot. The daemon used to hold EIGHT forwarders over the
// engine's own operations — `SaveWorkflow`, `RestoreWorkflow`,
// `SetWorkflowEnabled`, `RemoveWorkflow`, `RunWorkflow`, `TestWorkflowNode`,
// `DraftWorkflow` and `RefineWorkflow` — a second spelling of every verb the
// contract's engine port already names, and a second copy of the whole
// engine's surface for the canvas, the agent-facing tool and the daemon's own
// trigger path to call through.
//
// The engine's vocabulary is the CONTRACT's (`workflowapi.Engine` — save,
// restore, arm, remove, run, test a node, draft, refine), so the module
// answers in those names and the daemon spells none of them.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the engine's lifecycle any more;
//   - the daemon still holds the library's FILE, the engine's WIRING and the
//     engine's FACE.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"testing"
)

// workflowForwardersInTheDaemon are the daemon's spellings of the engine's
// operations.
var workflowForwardersInTheDaemon = []string{
	"SaveWorkflow",
	"RestoreWorkflow",
	"SetWorkflowEnabled",
	"RemoveWorkflow",
	"RunWorkflow",
	"TestWorkflowNode",
	"DraftWorkflow",
	"RefineWorkflow",
}

// TestTheDaemonDoesNotSpellOutTheWorkflowEngine pins the move: the engine's
// lifecycle belongs to the workflows module, and the daemon keeps the
// library's file, the engine's wiring and the engine's face.
func TestTheDaemonDoesNotSpellOutTheWorkflowEngine(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range workflowForwardersInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the engine's lifecycle is the workflows module's, and one operation has one name", name)
		}
	}
}

// TestTheDaemonKeepsTheWorkflowFileTheWiringAndTheFace pins what stayed: the
// library's FILE is the host's business (W3.48 — the host opens it), the
// WIRING is the host's own governed paths, and the FACE is the module's
// engine.
func TestTheDaemonKeepsTheWorkflowFileTheWiringAndTheFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"Workflows", "WorkflowPorts", "SetWorkflowEngine", "WorkflowEngine", "FireWorkflow"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the library's file, the engine's wiring and the module's face", name)
		}
	}
}
