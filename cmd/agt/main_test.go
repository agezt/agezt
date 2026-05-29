// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
)

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), brand.CLI) || !strings.Contains(out.String(), brand.Version) {
		t.Errorf("stdout missing identity; got %q", out.String())
	}
}

func TestRunHelpDefault(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("stdout missing usage; got %q", out.String())
	}
}

func TestRunNeedsIntent(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"run"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "intent required") {
		t.Errorf("stderr missing intent-required note; got %q", errOut.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"bogus"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Errorf("stderr missing unknown-command note; got %q", errOut.String())
	}
}

// TestJournalRejectsUnknownSubcommand — `agt journal <unknown>`
// must surface the supported set so operators discover what's
// available. After M0.5 the set is verify|tail (cmdJournal gained
// `tail` for snapshot-of-last-N reads); update this test if the
// set changes again.
func TestJournalRejectsUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"journal", "list"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "verify|tail") {
		t.Errorf("stderr missing supported-subcommands note; got %q", errOut.String())
	}
}
