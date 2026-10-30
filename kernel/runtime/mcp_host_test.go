// SPDX-License-Identifier: MIT

package runtime_test

// Provenance: the test host helper for the MCP REGISTRY (W3.58). The
//             registry's file is the daemon's — the kernel holds the
//             contract's port and never opens a registry of its own — so a
//             test that expects a registry opens one and injects it, the same
//             way the daemon does.
//
// The registry's LIFECYCLE is the extensions module's (W3.71), so a test that
// expects a working registry also hands the kernel the module's BUILDER —
// the same handover the daemon makes.

import (
	"testing"

	"github.com/agezt/agezt/kernel/internal/testfixtures"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
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
	registry, err := testfixtures.MCPRegistry(base)
	if err != nil {
		t.Fatalf("open the MCP registry: %v", err)
	}
	cfg.MCPStore = registry
	cfg.MCPBuilder = mcpServiceBuilder()
	return cfg
}

// mcpServiceBuilder is the extensions module's builder, handed to the kernel
// the way the daemon hands it over (W3.71): the module keeps the registry's
// lifecycle, the daemon keeps the names.
func mcpServiceBuilder() extapi.MCPRegistryBuilder {
	return func(deps extapi.MCPRegistryDependencies) extapi.MCPRegistryService {
		return mcp.NewService(deps)
	}
}
