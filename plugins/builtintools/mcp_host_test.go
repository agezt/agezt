// SPDX-License-Identifier: MIT

package builtintools

// Provenance: the test host helper for the MCP REGISTRY (W3.58). The
//             registry's file is the daemon's — the kernel holds the
//             contract's port and never opens a registry of its own — so a
//             test that expects a registry opens one and injects it, the same
//             way the daemon does.
//
// The helper lives here rather than in the kernel's shared test fixtures: a
// plugin may not reach a module — not even in a test, if the import can be
// avoided. (The archcheck gate reads the non-test imports; this file is a
// test, so the edge does not ship.)

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/extensions/mcp"
	"github.com/agezt/agezt/kernel/runtime"
)

// withMCP gives the config a registry: a real store under the config's base
// dir, the way the daemon opens mcp/servers.json and hands it over.
//
// The store is opened ONCE per config: a test that reopens the same config —
// a restart — keeps the same registry, so a server registered before the
// restart is still there after it. That is the whole point of the file.
func withMCP(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	if cfg.MCPStore != nil {
		return cfg
	}
	base := cfg.BaseDir
	if base == "" {
		base = t.TempDir()
	}
	store, err := mcp.OpenStore(filepath.Join(base, "mcp"))
	if err != nil {
		t.Fatalf("open the MCP registry: %v", err)
	}
	cfg.MCPStore = store
	return cfg
}
