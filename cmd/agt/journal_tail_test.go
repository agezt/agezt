// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdJournalTail_HelpDiscoversUsage — operators learn about
// the optional N positional through --help. Pin the documentation
// so future edits don't accidentally drop it.
func TestCmdJournalTail_HelpDiscoversUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalTail([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"journal tail", "N", "--json"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdJournalTail_RejectsNegativeN guards against an operator
// accidentally typing `agt journal tail -5` (which would otherwise
// parse as `-5` flag → unknown). N must be a positive integer.
func TestCmdJournalTail_RejectsNegativeN(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalTail([]string{"0"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), ">= 1") {
		t.Errorf("stderr should explain the >=1 constraint; got %q", errOut.String())
	}
}

// TestCmdJournalTail_RejectsNonNumericPositional covers typos
// like `agt journal tail twenty` — fail loudly rather than dial
// the daemon with garbage.
func TestCmdJournalTail_RejectsNonNumericPositional(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalTail([]string{"twenty"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should flag the non-numeric arg; got %q", errOut.String())
	}
}
