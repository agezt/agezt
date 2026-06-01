// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdScheduleFires_HelpExitsCleanly — `--help` prints usage and exits 0
// without needing a daemon (M54).
func TestCmdScheduleFires_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdScheduleFires([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"fires", "outcomes", "runs show"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdScheduleFires_RejectsBadArg — a non-numeric, non-flag positional is a
// usage error (exit 2) before any daemon dial (M54).
func TestCmdScheduleFires_RejectsBadArg(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdScheduleFires([]string{"notanumber"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject the arg; got %q", errOut.String())
	}
}

// TestCmdSchedule_DispatchesFires — the `fires` subcommand (and its `history`
// alias) reach cmdScheduleFires via the dispatcher (M54).
func TestCmdSchedule_DispatchesFires(t *testing.T) {
	for _, sub := range []string{"fires", "history"} {
		var out, errOut bytes.Buffer
		code := cmdSchedule([]string{sub, "--help"}, &out, &errOut)
		if code != 0 {
			t.Errorf("schedule %s --help exit=%d want 0; stderr=%q", sub, code, errOut.String())
		}
		if !strings.Contains(out.String(), "firings") {
			t.Errorf("schedule %s --help should render fires usage; got %q", sub, out.String())
		}
	}
}
