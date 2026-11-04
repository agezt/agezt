// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the ROSTER — the ratchet of W3.76n, extended by
// W3.76y.
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
// split this program removes, so the WRITES moved down (W3.76n). The daemon
// kept the names and forwarded — and that left a THIRD spelling of one
// family: the module's service already answered the CONTRACT's names (Add,
// SetEnabled, SetRetired, Update, Remove), so the daemon's five forwarders
// (AddProfile, SetProfileEnabled, SetProfileRetired, UpdateProfile,
// RemoveProfile) said the same five operations a third way.
//
// W3.76y took the names away. The daemon hands over the module's FACE
// (RosterService) and keeps the READS: they are the host's nil-safe view over
// the contract's port, and a read over a port is the host's.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the lifecycle any more (no roster event
//     kinds, no guardian rule, no direct store writes) and holds NO verb per
//     operation;
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
//
// The FORWARDER names are the third spelling: the daemon used to answer the
// module's operations under names of its own, and one operation has one name.
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
	"k.Roster().Add(",
	"k.Roster().SetEnabled(",
	"k.Roster().SetRetired(",
	"k.Roster().Update(",
	"k.Roster().Remove(",
	"func (k *Kernel) AddProfile(",
	"func (k *Kernel) SetProfileEnabled(",
	"func (k *Kernel) SetProfileRetired(",
	"func (k *Kernel) UpdateProfile(",
	"func (k *Kernel) RemoveProfile(",
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
				t.Errorf("%s holds %q — the roster's lifecycle is the agents module's (W3.76n); the daemon hands over the module's face", file, needle)
			}
		}
	}
}

// TestTheDaemonHoldsNoRosterForwarder pins the second half of the move
// (W3.76y): the daemon holds NO verb per roster operation.
//
// The five forwarders were a THIRD spelling of one family: the module's
// service already answered the contract's names (Add, SetEnabled, SetRetired,
// Update, Remove). The daemon hands over the module's face instead.
func TestTheDaemonHoldsNoRosterForwarder(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{
		"AddProfile", "SetProfileEnabled", "SetProfileRetired",
		"UpdateProfile", "RemoveProfile",
	} {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the roster's lifecycle is the agents module's, and one operation has one name (W3.76y)", name)
		}
	}
}

// TestTheDaemonKeepsTheRosterNames pins the other half: the READS did not
// move. Every caller above the daemon — the control plane, the application
// layer, the plugins and the composition root — still reads the roster
// through the host's nil-safe view, and the daemon hands over the module's
// face for the writes.
func TestTheDaemonKeepsTheRosterNames(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{
		"Roster", "RosterGet", "RosterList", "AgentImpact", "RosterService",
	} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon keeps the reads the layers above call and hands over the module's face", name)
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
