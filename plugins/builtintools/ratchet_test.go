// SPDX-License-Identifier: MIT

package builtintools

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/toolreg"
)

// bootSpecNames is the RATCHET for the boot tool registry (Phase 2.2 of
// docs/REFACTORING-SCAN-2026-08.md, mirror of builtinchannels'
// factory_ratchet_test.go): the EXPECTED, ORDERED set of spec names
// RegisterAll contributes. The order is load-bearing — Set.Descs() follows
// registration order and feeds the daemon's "tools :" boot banner, and the
// "plugins" spec must stay LAST so its YieldOnConflict semantics always lose
// to in-process names.
//
// If this test fails you either added a spec (append/insert its name here —
// consciously, in banner position), removed one (delete its entry), or
// reordered RegisterAll (restore the order or update this list on purpose).
// The point is that the boot tool surface never changes silently.
var bootSpecNames = []string{
	// workspace pair
	"shell",
	"file",
	// netguard slice (PR 2)
	"http",
	"browser.read",
	"browser.action",
	"web_search",
	"fetch",
	// Set*-injection batch (PR 3)
	"config",
	"artifacts",
	"db",
	"council",
	"conductor",
	"research",
	"code_exec",
	// kernel-bound zero-arg tools (PR 4)
	"schedule",
	"runs",
	"standing",
	"skill",
	"introspect",
	"overseer",
	"tool_forge",
	"mcp",
	"workflow",
	"workboard",
	// late-bound trio (PR 5)
	"notify",
	"send_media",
	"board",
	// env-gated externals (PR 6)
	"coding",
	"acp_agent",
	"homeassistant",
	"remote_run",
	// external plugin host — must stay last
	"plugins",
}

// TestRegisterAllMatchesPinnedSpecList is the drift alarm: every spec
// RegisterAll registers must be pinned above, every pinned name must be
// registered, and the registration ORDER must match (it drives the boot
// banner and the plugins-last conflict rule).
func TestRegisterAllMatchesPinnedSpecList(t *testing.T) {
	RegisterAll()
	got := toolreg.Names()

	want := map[string]bool{}
	for _, name := range bootSpecNames {
		if want[name] {
			t.Fatalf("bootSpecNames lists %q twice — fix the pinned list", name)
		}
		want[name] = true
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("spec %q is registered but not in bootSpecNames — a boot tool shipped without a conscious ratchet update; add it to the pinned list in its banner position", name)
		}
	}
	have := map[string]bool{}
	for _, name := range got {
		have[name] = true
	}
	for _, name := range bootSpecNames {
		if !have[name] {
			t.Errorf("bootSpecNames lists %q but RegisterAll never registers it — remove the stale entry or restore the spec", name)
		}
	}
	if t.Failed() {
		return // membership already broken; an order diff would be noise
	}
	if strings.Join(got, ",") != strings.Join(bootSpecNames, ",") {
		t.Errorf("registration order drifted (it drives the boot banner and plugins-last conflict semantics)\n got: %s\nwant: %s",
			strings.Join(got, ", "), strings.Join(bootSpecNames, ", "))
	}
}
