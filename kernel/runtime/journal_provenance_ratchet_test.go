// SPDX-License-Identifier: MIT

package runtime

// The ratchet of W3.76ad: the daemon holds no verb over the JOURNAL's own
// provenance walks.
//
// The event log is the host's own file (kernel/journal), and it answers four
// provenance operations in its own names: `Why` (the events of a correlation),
// `Causes` (the causation ancestry), `ParentOf` (a sub-agent's lead run) and
// `Verify` (the BLAKE3 hash-chain check).
//
// One family had THREE spellings: the journal's methods, the run engine's
// re-exports (`Runner.Why`, `Runner.Causes`, `Runner.ParentOf`,
// `Runner.Verify` — all four were one-line passthroughs to
// `r.k.Journal().…`) and the daemon's copies (`k.Why`, `k.Causes`,
// `k.ParentOf`, `k.Verify` — one-line passthroughs to the engine's).
//
// Three of the daemon's four had **no production caller at all** (only the
// daemon's own tests), and the fourth — the hash-chain check — had exactly
// one: the daemon-overview's gate. The gate asks the journal now, through
// the daemon's own journal door.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: putting one of the four back has to
// make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theJournalProvenanceWalks are the journal's own operations — the ones the
// daemon must not answer for a second time.
var theJournalProvenanceWalks = []string{
	"Why",
	"Causes",
	"ParentOf",
	"Verify",
}

// TestTheDaemonHoldsNoJournalProvenanceWalk pins the daemon's HALF: it holds
// no verb per journal operation.
func TestTheDaemonHoldsNoJournalProvenanceWalk(t *testing.T) {
	methods := daemonMethodSet(t)
	for _, name := range theJournalProvenanceWalks {
		if methods[name] {
			t.Errorf("*Kernel has the %s verb again — the provenance walk is the journal's, and one operation has one name (W3.76ad)", name)
		}
	}
}

// TestTheDaemonDoesNotDeclareAJournalProvenanceWalk pins the daemon's SHAPE:
// the daemon asks the journal for the provenance walks, it does not declare
// its own verb for them. The check reads the PRODUCTION files — a test file
// may legitimately mention a name.
func TestTheDaemonDoesNotDeclareAJournalProvenanceWalk(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the daemon package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		for _, name := range theJournalProvenanceWalks {
			declaration := regexp.MustCompile(`(?m)^func \(k \*Kernel\) ` + name + `\(`)
			if declaration.Match(src) {
				t.Errorf("%s declares the daemon's %s walk — the provenance walks live in the journal, and the daemon asks it (W3.76ad)", e.Name(), name)
			}
		}
	}
}

// TestTheDaemonSaysWhereTheProvenanceWalksLive pins the daemon's own word:
// the daemon says where the walks live, so the next reader does not have to
// guess. The file that lost the four verbs (runner_delegates.go) is gone
// since W3.76aj — the daemon holds no delegate wrapper at all — so the note
// lives in the package's own doc.
func TestTheDaemonSaysWhereTheProvenanceWalksLive(t *testing.T) {
	src, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("read doc.go: %v", err)
	}
	if !strings.Contains(strings.ToLower(string(src)), "the journal's provenance walks are the journal's own") {
		t.Error("the daemon must say where the provenance walks live")
	}
}
