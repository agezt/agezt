// SPDX-License-Identifier: MIT

package systemtasks

// Provenance: the test host helper for the memory (W3.57). The memory's file
//             is the daemon's — the kernel holds the contract's port and
//             never opens a memory of its own — so a test that expects a
//             memory opens one and injects it, the same way the daemon does.

import (
	"testing"

	"github.com/agezt/agezt/kernel/internal/testfixtures"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// withMemory gives the config a memory: a real store under the config's base
// dir, the way the daemon opens memory/memory.json and hands it over.
//
// The store is opened ONCE per config: a test that reopens the same config —
// a restart — keeps the same store, so a note written before the restart is
// still there after it. That is the whole point of the file.
func withMemory(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	if cfg.MemoryStore != nil {
		return cfg
	}
	base := cfg.BaseDir
	if base == "" {
		base = t.TempDir()
	}
	store, builder, err := testfixtures.MemoryPorts(base)
	if err != nil {
		t.Fatalf("open the memory store: %v", err)
	}
	cfg.MemoryStore = store
	cfg.MemoryBuilder = builder
	return cfg
}
