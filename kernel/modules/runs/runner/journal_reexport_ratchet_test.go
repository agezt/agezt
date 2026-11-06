// SPDX-License-Identifier: MIT

package runner

// The ratchet of W3.76ad: the run engine does not RE-EXPORT the journal's
// own operations.
//
// The event log is the host's own file, and it answers four provenance
// operations in its own names: `Why`, `Causes`, `ParentOf` and `Verify`. The
// engine used to answer all four as well — every one of them was a
// one-line passthrough to `r.k.Journal().…` — and the daemon copied them
// again, so one family had three spellings.
//
// The engine asks the host's journal for what it needs (the port carries the
// journal itself), and it holds no verb of its own for the provenance walks.
//
// A ratchet that is never probed never fires (the lesson W3.76i paid for),
// so the probe is part of the slice: re-exporting one of the four has to
// make this file fail.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theJournalProvenanceWalks are the journal's own operations — the ones the
// engine must not re-export.
var theJournalProvenanceWalks = []string{
	"Why",
	"Causes",
	"ParentOf",
	"Verify",
}

// TestTheRunEngineDoesNotReExportTheJournalWalks pins the ENGINE: it holds
// no method per journal operation. The check reads the PRODUCTION files — a
// test file may legitimately mention a name.
func TestTheRunEngineDoesNotReExportTheJournalWalks(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the run engine package: %v", err)
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
			declaration := regexp.MustCompile(`(?m)^func \(r \*Runner\) ` + name + `\(`)
			if declaration.Match(src) {
				t.Errorf("%s re-exports the journal's %s — the event log is the host's file, and one operation has one name (W3.76ad)", e.Name(), name)
			}
		}
	}
}

// TestTheRunEngineAsksTheJournalForItsWalks pins the other side of the
// shape: the engine still REACHES the provenance walks — through the host's
// journal, which is the host's own file — and it does not ask the host for a
// verb of its own.
func TestTheRunEngineAsksTheJournalForItsWalks(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the run engine package: %v", err)
	}
	ask := regexp.MustCompile(`r\.k\.(Why|Causes|ParentOf|Verify)\(`)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if ask.Match(src) {
			t.Errorf("%s asks the host for a provenance walk — the daemon holds no verb for one; the engine asks the journal (W3.76ad)", e.Name())
		}
	}
}
