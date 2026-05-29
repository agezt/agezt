// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdRunsLast_HelpExitsCleanly — pure flag parsing.
func TestCmdRunsLast_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsLast([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "most-recent run") {
		t.Errorf("--help missing 'most-recent run'; got %q", out.String())
	}
}

// TestCmdRunsLast_RejectsExtraArg — `agt runs last <something>`
// would be ambiguous; if the user wants a specific correlation
// they should be using `runs show`. Hard-reject so the typo
// doesn't get silently swallowed.
func TestCmdRunsLast_RejectsExtraArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsLast([]string{"some-correlation"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject; got %q", errOut.String())
	}
}

// TestCmdRuns_LastDispatch — `agt runs last` is routed to
// cmdRunsLast (not silently treated as an unknown subcommand).
// We can't easily prove the dispatch path without a daemon, but
// we can verify --help via the dispatcher reaches the help text.
func TestCmdRuns_LastDispatchHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRuns([]string{"last", "--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "most-recent run") {
		t.Errorf("dispatcher should reach cmdRunsLast --help; got %q", out.String())
	}
}
