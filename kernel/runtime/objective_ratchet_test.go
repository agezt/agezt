// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the OBJECTIVE LIBRARY — the ratchet of W3.76r.
//
// The library is the WORK MODULE's: the module keeps the goals, the key
// results and the rollup rule. The daemon used to hold six forwarders over
// the module's own operations (CreateObjective, AddObjectiveKeyResult,
// LinkObjectiveTask, UnlinkObjectiveTask, ArchiveObjective, Rollup) plus the
// two wiring doors (SetObjectiveLibrary, ObjectiveLibraryDependencies) — a
// second spelling of every one of them.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the objective lifecycle any more (no
//     forwarders, no direct library writes);
//   - the daemon still holds the two things that are its own: the library's
//     FILE (the contract's store port, W3.53) and the module's FACE.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"os"
	"strings"
	"testing"
)

// objectiveForwardersInTheDaemon are the daemon's spellings of the module's
// operations, and the wiring doors that used to build the service.
var objectiveForwardersInTheDaemon = []string{
	"CreateObjective",
	"AddObjectiveKeyResult",
	"LinkObjectiveTask",
	"UnlinkObjectiveTask",
	"ArchiveObjective",
	"Rollup",
	"SetObjectiveLibrary",
	"ObjectiveLibraryDependencies",
}

// TestTheDaemonDoesNotSpellOutTheObjectiveLifecycle pins the move: the
// objective lifecycle belongs to the work module, and the daemon keeps only
// the file and the module's face.
func TestTheDaemonDoesNotSpellOutTheObjectiveLifecycle(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range objectiveForwardersInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the objective lifecycle is the work module's; the daemon holds the file and the module's face, and one operation has one name", name)
		}
	}
}

// TestTheDaemonDoesNotWriteTheLibraryItself pins the other half of the move:
// the daemon holds the PORT over the library's file, and it asks the module —
// it does not write the rows itself.
func TestTheDaemonDoesNotWriteTheLibraryItself(t *testing.T) {
	for _, file := range daemonProductionFiles(t) {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, needle := range []string{
			"k.okr.Create(",
			"k.okr.AddKeyResult(",
			"k.okr.LinkTask(",
			"k.okr.UnlinkTask(",
			"k.okr.Archive(",
		} {
			if strings.Contains(string(src), needle) {
				t.Errorf("%s holds %q — the library's rows are the module's to write; the daemon holds the port", file, needle)
			}
		}
	}
}

// TestTheDaemonKeepsTheLibraryFileAndTheModulesFace pins what stayed: the
// library's FILE is the daemon's business (W3.53) and the module's FACE is
// what the composition root's scope hands to the application.
func TestTheDaemonKeepsTheLibraryFileAndTheModulesFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"OKR", "Objectives"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the library's file and the module's face", name)
		}
	}
}
