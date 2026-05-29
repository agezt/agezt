// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdHalt_HelpExitsCleanly — pure flag parsing.
func TestCmdHalt_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdHaltResume("halt", []string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"--reason", "--json"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdResume_HelpExitsCleanly — symmetric to halt.
func TestCmdResume_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdHaltResume("resume", []string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "halt flag") {
		t.Errorf("resume --help should mention halt flag; got %q", out.String())
	}
}

// TestCmdHalt_RejectsUnknownArg — silent-drop guard.
func TestCmdHalt_RejectsUnknownArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdHaltResume("halt", []string{"--bogus"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject; got %q", errOut.String())
	}
}

// TestCmdHalt_ReasonNeedsValue — `agt halt --reason` with no
// value would otherwise silently omit; refuse.
func TestCmdHalt_ReasonNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdHaltResume("halt", []string{"--reason"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "needs a value") {
		t.Errorf("stderr should explain; got %q", errOut.String())
	}
}
