// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the ROSTER — the ratchet of W3.76n.
//
// The roster is the agents module's (the target architecture's module
// catalogue says it in one line). Its DATA was already the module's — the
// store, the normalization and the validation live in
// kernel/modules/agents/roster — and its OPERATIONS were the daemon's:
// kernel/runtime/accessors_roster.go held the store, applied the write,
// published the event and knew that a shipped guardian may be paused or
// retired but never removed.
//
// A module that keeps the data and gives away the meaning of the data is the
// split this program removes, so the WRITES moved down. The daemon kept the
// names (AddProfile, UpdateProfile, RemoveProfile, SetProfileEnabled,
// SetProfileRetired) and forwards to the module's service.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the lifecycle any more (no roster event
//     kinds, no guardian rule, no direct store writes);
//   - the daemon still answers every name the layers above call.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting the lifecycle back into the
// daemon has to make this file fail.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rosterLifecycleInTheDaemon are the marks of the daemon spelling out the
// roster's lifecycle itself.
//
// The event kinds are the roster's vocabulary: the module decides what a
// roster change SAYS, and the host only carries it. The guardian message is
// the roster's rule about its own rows. The store calls are the roster's
// operations: the daemon holds the PORT and asks the module, it does not
// write the rows itself.
var rosterLifecycleInTheDaemon = []string{
	"event.KindRosterCreated",
	"event.KindRosterUpdated",
	"event.KindRosterRemoved",
	"protected system guardian",
	"k.roster.Add(",
	"k.roster.SetEnabled(",
	"k.roster.SetRetired(",
	"k.roster.Update(",
	"k.roster.Remove(",
}

// TestTheDaemonDoesNotSpellOutTheRosterLifecycle pins the move: the roster's
// lifecycle belongs to the agents module, and the daemon keeps the names.
func TestTheDaemonDoesNotSpellOutTheRosterLifecycle(t *testing.T) {
	for _, file := range daemonProductionFiles(t) {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, needle := range rosterLifecycleInTheDaemon {
			if strings.Contains(string(src), needle) {
				t.Errorf("%s holds %q — the roster's lifecycle is the agents module's (W3.76n); the daemon keeps the names and forwards", file, needle)
			}
		}
	}
}

// TestTheDaemonKeepsTheRosterNames pins the other half: the names did not
// move. Every caller above the daemon — the control plane, the application
// layer, the plugins and the composition root — still calls the same verbs.
func TestTheDaemonKeepsTheRosterNames(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{
		"Roster", "RosterGet", "RosterList",
		"AddProfile", "UpdateProfile", "RemoveProfile",
		"SetProfileEnabled", "SetProfileRetired",
		"AgentImpact",
	} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon keeps the names the layers above call", name)
		}
	}
}

// daemonProductionFiles lists the daemon package's own production files.
func daemonProductionFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		files = append(files, filepath.Join(".", e.Name()))
	}
	return files
}
