// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdApprovals_HelpDocsJSON verifies --json appears in the
// help output — operators discover the flag here, and CI smoke
// tests pin its presence to catch accidental removal.
func TestCmdApprovals_HelpDocsJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdApprovals([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "--json") {
		t.Errorf("--help should mention --json; got %q", out.String())
	}
}

// TestCmdApprovals_RejectsUnknownFlag prevents the parser from
// silently treating a typo'd flag as a positional arg (the old
// version of cmdApprovals took no args at all, so any input was
// ignored).
func TestCmdApprovals_RejectsUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdApprovals([]string{"--josn"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("typo'd flag should be rejected; got %q", errOut.String())
	}
}

// TestCmdCatalogList_HelpDocsJSON — same discoverability pin as
// approvals, for the catalog subcommand.
func TestCmdCatalogList_HelpDocsJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdCatalogList([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "--json") {
		t.Errorf("--help should mention --json; got %q", out.String())
	}
}

// TestCmdCatalogList_RejectsUnknownFlag — same typo-safety pin
// as approvals.
func TestCmdCatalogList_RejectsUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdCatalogList([]string{"--csv"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("unsupported flag should be rejected; got %q", errOut.String())
	}
}
