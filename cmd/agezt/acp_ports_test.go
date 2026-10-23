// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"testing"

	"github.com/agezt/agezt/kernel/modules/extensions/acpcatalog"
)

// TestToolBuildDepsSuppliesTheACPCataloguePort: the acp_agent tool asks the
// ACP catalogue which agents this host has installed; the tool is a plugin and
// may not name the catalogue module, so the daemon hands the CONTRACT's port
// over here, at the composition root. A daemon that dropped it would leave
// the tool unable to resolve an agent on every live box, while every test
// that builds the tool by hand stayed green. This test is the only one that
// sees that wiring, so it asserts it directly.
func TestToolBuildDepsSuppliesTheACPCataloguePort(t *testing.T) {
	deps := toolBuildDeps(t.TempDir(), io.Discard, nil, nil)
	if deps.ACP == nil {
		t.Fatal("the daemon must supply the ACP catalogue port")
	}

	// The port must answer the LIVE catalogue — a port bound to nothing would
	// still be non-nil.
	if got, want := deps.ACP.AnyInstalled(), acpcatalog.AnyInstalled(); got != want {
		t.Errorf("AnyInstalled = %v, want the catalogue's %v", got, want)
	}
	got := deps.ACP.InstalledSlugs()
	want := acpcatalog.InstalledSlugs()
	if len(got) != len(want) {
		t.Fatalf("InstalledSlugs returned %d slugs, want the catalogue's %d: %v vs %v", len(got), len(want), got, want)
	}
	// An unknown slug never resolves — the port keeps the catalogue's rule
	// that a selector is a slug and never a raw shell line.
	if cmd, ok := deps.ACP.ResolveCommand("no-such-agent-slug", ""); ok {
		t.Errorf("an unknown slug must not resolve, got %q", cmd)
	}
}
