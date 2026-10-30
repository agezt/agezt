// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"testing"

	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// withForge opens the script-tool store the composition root would open
// (W3.45) and hands the kernel the contract's port.
//
// The store is the DAEMON's file: the kernel holds the port and drives the
// lifecycle through it. A kernel opened without one has no forge at all —
// drafting, testing and promoting report that the forge is unavailable.
//
// The forge's LIFECYCLE is the extensions module's (W3.72), so the helper also
// hands the kernel the module's BUILDER — the same handover the daemon makes.
func withForge(t *testing.T, cfg kernelruntime.Config) kernelruntime.Config {
	t.Helper()
	store, err := toolforge.Open(filepath.Join(cfg.BaseDir, "toolforge"))
	if err != nil {
		t.Fatalf("open the forge store: %v", err)
	}
	cfg.ToolForge = store
	cfg.ForgeBuilder = func(deps extapi.ForgeDependencies) extapi.ForgeService {
		return toolforge.NewService(deps)
	}
	return cfg
}
