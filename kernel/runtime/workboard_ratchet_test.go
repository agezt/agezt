// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the TASK BOARD — the ratchet of W3.76s.
//
// The board is the WORK MODULE's: the module keeps the store's rules, the
// lifecycle and the proof. The daemon used to hold SIXTEEN forwarders over the
// module's own operations (CreateWorkboardTask, ClaimWorkboardTask,
// HeartbeatWorkboardTask, CommentWorkboardTask, BlockWorkboardTask,
// FailWorkboardTask, UnblockWorkboardTask, SetWorkboardRetryPolicy,
// CompleteWorkboardTask, ReviewWorkboardTask, ArchiveWorkboardTask,
// LinkWorkboardTask, AddWorkboardDependency, ReclaimStaleWorkboardTask,
// SweepStaleWorkboardClaims and ProveTask) — a second spelling of every board
// verb, and a second copy of the whole board surface for the application layer
// and the plugin to call through.
//
// The MODULE implements the CONTRACT's runner port over its own service now
// (kernel/modules/work/workboard/runner.go), and the daemon holds two things:
// the board's FILE and the module's FACE.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the board's lifecycle any more;
//   - the daemon still holds the file, the wiring and the face.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"testing"
)

// boardForwardersInTheDaemon are the daemon's spellings of the board's
// operations.
var boardForwardersInTheDaemon = []string{
	"CreateWorkboardTask",
	"ClaimWorkboardTask",
	"HeartbeatWorkboardTask",
	"CommentWorkboardTask",
	"BlockWorkboardTask",
	"FailWorkboardTask",
	"UnblockWorkboardTask",
	"SetWorkboardRetryPolicy",
	"CompleteWorkboardTask",
	"ReviewWorkboardTask",
	"ArchiveWorkboardTask",
	"LinkWorkboardTask",
	"AddWorkboardDependency",
	"ReclaimStaleWorkboardTask",
	"SweepStaleWorkboardClaims",
	"ProveTask",
	"SetWorkboard",
}

// TestTheDaemonDoesNotSpellOutTheBoardLifecycle pins the move: the board's
// lifecycle belongs to the work module, and the daemon keeps the file, the
// wiring and the module's face.
func TestTheDaemonDoesNotSpellOutTheBoardLifecycle(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range boardForwardersInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the board's lifecycle is the work module's, and one operation has one name", name)
		}
	}
}

// TestTheDaemonKeepsTheBoardFileTheWiringAndTheFace pins what stayed: the
// board's FILE is the host's business (W3.47), the WIRING is the host's own
// ports, and the FACE is the module's runner.
func TestTheDaemonKeepsTheBoardFileTheWiringAndTheFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"Workboard", "WorkboardDependencies", "BoardRunner"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the board's file, its wiring and the module's face", name)
		}
	}
}
