// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestObservabilityCmds_AcceptTenantFlag locks the M129 wiring: every tenant-
// routable observability CLI must accept `--tenant <id>` (so an operator can
// inspect a tenant's own subsystems and a tenant can read its own via the agt
// CLI — the M128 daemon grant). The flag is parsed BEFORE the command dials, so
// with no daemon the command fails to connect (exit 1) rather than rejecting the
// flag (exit 2 / "unexpected arg"). We assert the flag is accepted, not the
// daemon round-trip. Daemon-free: AGEZT_HOME points at an empty dir so dial fails
// fast.
func TestObservabilityCmds_AcceptTenantFlag(t *testing.T) {
	t.Setenv("AGEZT_HOME", t.TempDir())

	cmds := map[string]func([]string, io.Writer, io.Writer) int{
		"memory log":          cmdMemoryLog,
		"world log":           cmdWorldLog,
		"approvals log":       cmdApprovalsLog,
		"approvals stats":     cmdApprovalsStats,
		"plan history":        cmdPlanHistory,
		"plan stats":          cmdPlanStats,
		"provider log":        cmdProviderLog,
		"provider stats":      cmdProviderStats,
		"provider rejections": cmdProviderRejections,
		"schedule fires":      cmdScheduleFires,
		"schedule stats":      cmdScheduleStats,
		"warden log":          cmdWardenLog,
		"warden stats":        cmdWardenStats,
	}

	for name, fn := range cmds {
		var out, errOut bytes.Buffer
		code := fn([]string{"--tenant", "acme"}, &out, &errOut)
		if code == 2 {
			t.Errorf("%s: --tenant rejected as a usage error (exit 2): %q", name, errOut.String())
		}
		if strings.Contains(errOut.String(), "unexpected") {
			t.Errorf("%s: --tenant reported as unexpected arg: %q", name, errOut.String())
		}
	}
}

// TestObservabilityCmds_TenantFlagInHelp confirms `--tenant` is documented in
// each command's help, so it's discoverable (M129).
func TestObservabilityCmds_TenantFlagInHelp(t *testing.T) {
	cmds := map[string]func([]string, io.Writer, io.Writer) int{
		"memory log":          cmdMemoryLog,
		"world log":           cmdWorldLog,
		"approvals log":       cmdApprovalsLog,
		"approvals stats":     cmdApprovalsStats,
		"plan history":        cmdPlanHistory,
		"plan stats":          cmdPlanStats,
		"provider log":        cmdProviderLog,
		"provider stats":      cmdProviderStats,
		"provider rejections": cmdProviderRejections,
		"schedule fires":      cmdScheduleFires,
		"schedule stats":      cmdScheduleStats,
		"warden log":          cmdWardenLog,
		"warden stats":        cmdWardenStats,
	}
	for name, fn := range cmds {
		var out, errOut bytes.Buffer
		if code := fn([]string{"--help"}, &out, &errOut); code != 0 {
			t.Errorf("%s --help exit = %d want 0", name, code)
		}
		if !strings.Contains(out.String(), "--tenant") {
			t.Errorf("%s help does not document --tenant: %q", name, out.String())
		}
	}
}
