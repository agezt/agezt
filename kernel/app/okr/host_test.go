// SPDX-License-Identifier: MIT
package okr_test

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
)

// withWorkboard opens the task board the composition root would open (W3.47)
// and hands the kernel the contract's host port.
//
// The board's file is the DAEMON's: the kernel holds the port and drives the
// lifecycle through it. The OKR rollup reads the board — a linked task counts
// toward a key result once it is done — so a fixture that exercises the
// rollup needs one. This test is such a host.
func withWorkboard(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	cfg.Workboard = store
	return cfg
}
