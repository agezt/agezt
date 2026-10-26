// SPDX-License-Identifier: MIT

package testfixtures

// Provenance: the shared test host helper for the MEMORY (W3.57). The
//             memory's file is the daemon's and the kernel holds the
//             contract's port — so a test that wants a memory opens one and
//             injects it, the same way the daemon does.
//
// The helper returns the TWO ports a host injects: the store (the file) and
// the builder (the daemon names the module; the kernel calls the builder
// with the store and its own publisher). The publisher is the kernel's — the
// host does not hand one over.

import (
	"path/filepath"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
)

// MemoryPorts opens a memory store under dir and returns the store plus the
// builder the kernel calls with it.
func MemoryPorts(dir string) (memoryapi.Store, memoryapi.Builder, error) {
	store, err := memory.Open(filepath.Join(dir, "memory"))
	if err != nil {
		return nil, nil, err
	}
	return store, func(deps memoryapi.Deps) memoryapi.Manager {
		return memory.NewManager(deps)
	}, nil
}
