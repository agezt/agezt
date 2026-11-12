// SPDX-License-Identifier: MIT

package runtime

// The daemon's RUN'S TOOL FOLD door (W3.76aj).
//
// Counting the tool results of one run — and collecting the tool names, in
// order — is a walk over the EVENT LOG. The event log is the host's own file
// (kernel/journal), and the journal answers walks over it: the provenance
// walks (Why, Causes, ParentOf, Verify) live there since W3.76ad.
//
// The daemon kept its own copy of that walk (FoldRunTools) and the run engine
// — which lives inside the runs module — asked the HOST for it, only to be
// forwarded to the very package next door. One family, two spellings, and a
// hop the host never had to keep.
//
// These tests pin that: the daemon holds no verb for the fold, no copy of its
// body, and no name for the journal's own fold.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// theFoldVerbIsNotTheDaemons is the daemon's own verb for the fold. The fold
// is the journal's operation, so the daemon answers for no fold.
var theFoldVerbIsNotTheDaemons = regexp.MustCompile(`(?m)^func \(k \*Kernel\) FoldRunTools\(`)

// theFoldBodyIsNotTheDaemons is the fold's own shape: a journal walk that
// keeps THIS correlation's tool.result events. The daemon must not grow a
// copy of it back under another name.
var theFoldBodyIsNotTheDaemons = regexp.MustCompile(`CorrelationID != corr \|\| e\.Kind != event\.KindToolResult`)

// theFoldNameIsNotTheDaemons is the journal's own name for the fold. The
// daemon has no business calling it: the run engine asks the log.
var theFoldNameIsNotTheDaemons = regexp.MustCompile(`\.FoldTools\(`)

// TestTheDaemonHoldsNoVerbPerFold pins the SURFACE: the daemon answers for
// no fold.
func TestTheDaemonHoldsNoVerbPerFold(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theFoldVerbIsNotTheDaemons.Match(src) {
			t.Errorf("%s declares a fold verb — the fold is the journal's (W3.76aj)", f)
		}
	}
}

// TestTheDaemonHoldsNoFoldBody pins the BODY: the daemon does not fold the
// journal itself, under any name.
func TestTheDaemonHoldsNoFoldBody(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theFoldBodyIsNotTheDaemons.Match(src) {
			t.Errorf("%s grew a copy of the fold back — the fold is the journal's (W3.76aj)", f)
		}
	}
}

// TestTheDaemonDoesNotCallTheJournalsFold pins the NAME: the daemon does not
// ask the journal for the run's fold either — the run engine does. The check
// reads the daemon's CODE (the comments are stripped): the daemon's own doc
// may name the fold, but no daemon code may call it.
func TestTheDaemonDoesNotCallTheJournalsFold(t *testing.T) {
	for _, f := range daemonProductionFiles(t) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if theFoldNameIsNotTheDaemons.Match(daemonCode(t, src)) {
			t.Errorf("%s asks the journal for the run's fold — the run engine asks the log (W3.76aj)", f)
		}
	}
}

// daemonCode strips the line comments from the daemon's source, so a ratchet
// that pins a CALL reads the code and not the prose around it.
func daemonCode(t *testing.T, src []byte) []byte {
	t.Helper()
	var out []string
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}
