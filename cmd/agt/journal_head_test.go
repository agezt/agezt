// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdJournalHead_HelpExitsCleanly — pure flag parsing.
func TestCmdJournalHead_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalHead([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "head seq") {
		t.Errorf("--help missing 'head seq'; got %q", out.String())
	}
}

// TestCmdJournalHead_RejectsExtraArg — silent-drop guard.
func TestCmdJournalHead_RejectsExtraArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdJournalHead([]string{"--bogus"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject; got %q", errOut.String())
	}
}
