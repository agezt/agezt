// SPDX-License-Identifier: MIT

package runtime

// The daemon's face over the STANDING LIBRARY — the ratchet of W3.76x.
//
// The standing library's lifecycle is the STANDING PACKAGE's: the package keeps
// the operations and the journal behind them, and the daemon used to hold FOUR
// forwarders over that package's own verbs (`AddStanding`,
// `SetStandingEnabled`, `UpdateStanding`, `RemoveStanding`) plus ONE reader of
// its own (`StandingList`) — a second spelling of every operation the
// contract's lifecycle port already names.
//
// The vocabulary is the CONTRACT's (`standingapi.Lifecycle` — add, pause, edit,
// remove, list), so the library answers in those names and the daemon spells
// none of them.
//
// This file pins the move in both directions:
//
//   - the daemon does not spell out the standing library's operations any more;
//   - the daemon still holds the library's FILE (the contract's store port) and
//     the library's FACE.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting a forwarder back into the daemon
// has to make this file fail.

import (
	"testing"
)

// standingVerbsInTheDaemon are the daemon's spellings of the standing
// library's operations.
var standingVerbsInTheDaemon = []string{
	"AddStanding",
	"SetStandingEnabled",
	"UpdateStanding",
	"RemoveStanding",
	"StandingList",
}

// TestTheDaemonDoesNotSpellOutTheStandingLibrary pins the move: the standing
// library's lifecycle is the standing package's, and one operation has one
// name.
func TestTheDaemonDoesNotSpellOutTheStandingLibrary(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range standingVerbsInTheDaemon {
		if _, ok := methods[name]; ok {
			t.Errorf("*Kernel has the %s verb again — the standing library's lifecycle is the standing package's, and one operation has one name (W3.76x)", name)
		}
	}
}

// TestTheDaemonKeepsTheStandingFileAndTheFace pins what stayed: the library's
// FILE (the daemon opens it and holds the contract's store port) and the
// library's FACE — the one port the standing application, the guardians' orders
// and the agent-facing standing-orders tool bind.
func TestTheDaemonKeepsTheStandingFileAndTheFace(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range []string{"Standing", "StandingService"} {
		if !methods[name] {
			t.Errorf("*Kernel lost the %s verb — the daemon holds the library's file and the library's face", name)
		}
	}
}
