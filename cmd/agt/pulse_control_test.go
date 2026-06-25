// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// Daemon-free paths: help + arg validation. The dial-and-call paths are
// covered by the control-plane integration tests.

func TestCmdPulseControl_Help(t *testing.T) {
	for _, sub := range []string{"status", "pause", "resume"} {
		var out, errOut bytes.Buffer
		if code := cmdPulseControl(sub, []string{"--help"}, &out, &errOut); code != 0 {
			t.Fatalf("%s --help exit=%d", sub, code)
		}
		if !strings.Contains(out.String(), "pulse "+sub) {
			t.Errorf("%s help missing usage; got %q", sub, out.String())
		}
	}
}

func TestCmdPulseControl_RejectsUnexpectedArg(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdPulseControl("status", []string{"bogus"}, &out, &errOut); code != 2 {
		t.Errorf("unexpected arg should be exit 2, got %d", code)
	}
}

func TestCmdPulseAsks_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdPulseAsks([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("asks --help exit=%d", code)
	}
	if !strings.Contains(out.String(), "pulse asks") {
		t.Errorf("asks help missing usage; got %q", out.String())
	}
}

func TestCmdPulseAsks_ArgValidation(t *testing.T) {
	// approve/reject without an issue_key is a usage error (exit 2), no dial.
	for _, verb := range []string{"approve", "reject"} {
		var out, errOut bytes.Buffer
		if code := cmdPulseAsks([]string{verb}, &out, &errOut); code != 2 {
			t.Errorf("%s without key should be exit 2, got %d", verb, code)
		}
	}
	// An unexpected bare arg (no verb) is a usage error too.
	var out, errOut bytes.Buffer
	if code := cmdPulseAsks([]string{"bogus"}, &out, &errOut); code != 2 {
		t.Errorf("unexpected arg should be exit 2, got %d", code)
	}
}

func TestCmdPulse_RoutesAsksSubcommand(t *testing.T) {
	// `pulse asks approve` (missing key) must hit the asks arg-validation (exit 2),
	// proving the router dispatches "asks" rather than falling through to the tail.
	var out, errOut bytes.Buffer
	if code := cmdPulse([]string{"asks", "approve"}, &out, &errOut); code != 2 {
		t.Errorf("pulse asks approve (no key) should route to asks validation (exit 2), got %d", code)
	}
}

// TestCmdPulse_TailUnaffected — bare `agt pulse --help`-style parsing must NOT
// be intercepted by the new subcommand router. We assert routing only triggers
// for the three control verbs by checking that a flag-only invocation still
// goes down the tail path (which will try to dial and fail without a daemon —
// exit 1, not a subcommand usage error).
func TestCmdPulse_SubcommandRoutingIsScoped(t *testing.T) {
	var out, errOut bytes.Buffer
	// "--subject" is a tail flag, not a subcommand; must not be treated as a
	// control verb. With no daemon it fails to dial (exit 1), proving it took
	// the tail path rather than the control path's arg validation (exit 2).
	code := cmdPulse([]string{"--subject", "agent.>"}, &out, &errOut)
	if code == 2 {
		t.Errorf("tail flags must not hit subcommand arg-validation; got exit 2")
	}
}
