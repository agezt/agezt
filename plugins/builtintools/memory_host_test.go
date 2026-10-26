// SPDX-License-Identifier: MIT

package builtintools

// Provenance: the test host helper for the memory (W3.57). The memory's file
//             is the daemon's — the kernel holds the contract's port and
//             never opens a memory of its own — so a test that expects a
//             memory opens one and injects it, the same way the daemon does.
//
// The helper lives here rather than in the kernel's shared test fixtures: a
// plugin may not reach a module — not even in a test, if the import can be
// avoided. (The archcheck gate reads the non-test imports; this file is a
// test, so the edge does not ship.)

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	"github.com/agezt/agezt/kernel/runtime"
)

// withMemory gives the config a memory: a real store under the config's base
// dir, the way the daemon opens memory/memory.json and hands it over.
func withMemory(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	if cfg.MemoryStore != nil {
		return cfg
	}
	base := cfg.BaseDir
	if base == "" {
		base = t.TempDir()
	}
	store, err := memory.Open(filepath.Join(base, "memory"))
	if err != nil {
		t.Fatalf("open the memory store: %v", err)
	}
	cfg.MemoryStore = store
	cfg.MemoryBuilder = func(deps memoryapi.Deps) memoryapi.Manager {
		return memory.NewManager(deps)
	}
	return cfg
}
