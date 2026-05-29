// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdWhy_RejectsMissingID covers the argument validation
// path; no daemon is needed because we never reach the dial.
func TestCmdWhy_RejectsMissingID(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdWhy(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "event_id required") {
		t.Errorf("stderr missing event_id-required note; got %q", errOut.String())
	}
}

// TestCmdWhy_RejectsExtraPositional ensures the parser doesn't
// silently drop a second positional arg (which a previous version
// of cmdWhy did, leading to confusing UX where typos became
// no-ops).
func TestCmdWhy_RejectsExtraPositional(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdWhy([]string{"evt-abc", "evt-def"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should flag the second positional; got %q", errOut.String())
	}
}

// TestCmdWhy_HelpExitsCleanly — `agt why --help` mustn't try to
// dial the daemon or require an event_id; operators discover the
// new --json/--payload flags through this path.
func TestCmdWhy_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdWhy([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	stdout := out.String()
	for _, want := range []string{"--json", "--payload", "correlation chain"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("--help missing %q; got %q", want, stdout)
		}
	}
}
