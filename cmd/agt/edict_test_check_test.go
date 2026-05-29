// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdEdictTest_HelpDocsExitCodes — the unusual exit-3-on-deny
// behavior is the key contract for CI scripts. Pin the help text
// so refactors don't lose the documentation.
func TestCmdEdictTest_HelpDocsExitCodes(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdictTest([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"capability", "--json", "exit 0", "3 = deny"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdEdictTest_RejectsMissingCapability — the parser must
// catch the absent required arg before dialing the daemon, with
// a message that points at the expected vocabulary.
func TestCmdEdictTest_RejectsMissingCapability(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdictTest(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "capability required") {
		t.Errorf("stderr should say capability required; got %q", errOut.String())
	}
}

// TestCmdEdictTest_RejectsTooManyPositionals — `agt edict test
// shell "echo" "extra"` should fail loudly. Without this guard
// a typo would be silently dropped.
func TestCmdEdictTest_RejectsTooManyPositionals(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdEdictTest([]string{"shell", "echo hi", "extra"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject extra positional; got %q", errOut.String())
	}
}
