// SPDX-License-Identifier: MIT

package whoami

import (
	"strings"
	"testing"
)

// The package's `Run` is exercised by the integration-style tests
// in cmd/agt/help_test.go (every registered command answers
// -h without panicking) and cmd/agt/main_test.go. The unit
// tests here cover the argument-parsing branches that don't
// need a live daemon: --help, --tenant without value, and
// unknown arg. The "happy path" branches (primary, tenant,
// --json) require a control plane and are out of scope for
// pure unit tests.

func TestRun_HelpPrintsUsageAndExitsZero(t *testing.T) {
	var out, errb strings.Builder
	code := Run([]string{"--help"}, &out, &errb)
	if code != 0 {
		t.Errorf("Run(--help) returned %d, want 0", code)
	}
	if !strings.Contains(out.String(), "usage:") {
		t.Errorf("expected usage line, got: %q", out.String())
	}
}

func TestRun_TenantWithoutValueExitsTwo(t *testing.T) {
	var out, errb strings.Builder
	code := Run([]string{"--tenant"}, &out, &errb)
	if code != 2 {
		t.Errorf("Run(--tenant) returned %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "needs an id") {
		t.Errorf("expected --tenant-needs-id hint, got: %q", errb.String())
	}
}

func TestRun_UnknownArgExitsTwo(t *testing.T) {
	var out, errb strings.Builder
	code := Run([]string{"--bogus"}, &out, &errb)
	if code != 2 {
		t.Errorf("Run(--bogus) returned %d, want 2", code)
	}
	if !strings.Contains(errb.String(), "unexpected arg") {
		t.Errorf("expected unexpected-arg message, got: %q", errb.String())
	}
}
