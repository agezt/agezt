// SPDX-License-Identifier: MIT

package system

// The daemon-overview application's BINDING — the ratchet of W3.76ad.
//
// The daemon's GATE is three doors: the halt that stops new runs, the resume
// that lets them start again, and the journal's hash-chain check. The first
// two are the runs module's operations — the gate is an adapter over the
// module's face (`LifecycleService`). The third is the JOURNAL's own
// operation: the event log is the host's file, and the check belongs to it.
//
// The daemon used to answer the check under a verb of its own (`k.Verify`) —
// the run engine re-exported it, so one family had three spellings. The gate
// asks the daemon's own journal door now (`k.Journal().Verify`).
//
// What is pinned here:
//
//   - the gate does not bind a VERB of the daemon's — the daemon holds none
//     over the check, and one that came back would be a second spelling
//     again;
//   - the gate reaches the check through the JOURNAL.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: binding the daemon's own verb again has
// to make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theDaemonsOwnVerify is a journal check called ON THE DAEMON — the shape
// the application must not carry.
//
// The receiver must be the daemon (`k.`): the journal is reached as
// `k.Journal().Verify(…)`, and that call is NOT this shape. The name must be
// followed by a PARENTHESIS, so that the file's own history stays out of the
// ratchet.
var theDaemonsOwnVerify = regexp.MustCompile(`\bk\.Verify\s*\(`)

// theJournalDoor is the binding the application must carry: the daemon's own
// journal door, over the hash-chain check.
var theJournalDoor = regexp.MustCompile(`k\.Journal\(\)\.Verify`)

// TestTheDaemonOverviewGateBindsTheJournal pins the binding: the gate's
// verification reaches the journal.
func TestTheDaemonOverviewGateBindsTheJournal(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon-overview application package: %v", err)
	}
	journal := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if theDaemonsOwnVerify.Match(src) {
			t.Errorf("%s binds the daemon's own Verify — the hash-chain check is the journal's, and one operation has one name (W3.76ad)", e.Name())
		}
		if theJournalDoor.Match(src) {
			journal = true
		}
	}
	if !journal {
		t.Error("the daemon-overview's gate does not reach the journal's hash-chain check — the check is the journal's operation")
	}
}
