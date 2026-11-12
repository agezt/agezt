// SPDX-License-Identifier: MIT

package runner

// The run engine's RUN'S TOOL FOLD door (W3.76aj).
//
// Counting the tool results of one run — and collecting the tool names, in
// order — is a walk over the EVENT LOG. The event log is the host's own file
// (kernel/journal), and the journal answers walks over it: the provenance
// walks (Why, Causes, ParentOf, Verify) live there since W3.76ad.
//
// The daemon held its own copy of that walk (FoldRunTools), and the engine
// asked the HOST for the fold — one family, two spellings, and a hop the host
// never had to keep. The port already carries the journal itself, so the
// engine asks the log and the hop is gone.
//
// These tests pin that: the port carries no fold entry, and the engine calls
// the journal's fold.

import (
	"os"
	"regexp"
	"testing"
)

// theFoldEntriesAreNotTheHosts are the fold operations the run engine's host
// port must NOT carry: the fold is the journal's operation, and a port entry
// for it is a hop the host never had to keep.
var theFoldEntriesAreNotTheHosts = regexp.MustCompile(`(?m)^\tFoldRunTools\(`)

// TestTheRunEnginePortCarriesNoFoldEntry pins the PORT: it promises the
// host's capabilities, not the journal's own walks.
func TestTheRunEnginePortCarriesNoFoldEntry(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read api.go: %v", err)
	}
	if theFoldEntriesAreNotTheHosts.Match(src) {
		t.Error("the run engine's port carries a fold entry — the fold is the journal's (W3.76aj)")
	}
}

// TestTheRunEngineAsksTheJournalForItsFold pins the WIRING: the engine folds
// the log itself, through the journal the port already carries.
func TestTheRunEngineAsksTheJournalForItsFold(t *testing.T) {
	if !runnerPackageContains(t, "Journal().FoldTools(") {
		t.Error("the run engine must ask the journal for the run's fold — the fold is the journal's (W3.76aj)")
	}
}

// TestTheRunEngineHoldsNoFoldOfItsOwn pins the BODY: the engine keeps no
// copy of the fold either — one walk, one spelling.
func TestTheRunEngineHoldsNoFoldOfItsOwn(t *testing.T) {
	if runnerPackageContains(t, "CorrelationID != corr || e.Kind != event.KindToolResult") {
		t.Error("the run engine grew a copy of the fold — the journal answers it (W3.76aj)")
	}
}
