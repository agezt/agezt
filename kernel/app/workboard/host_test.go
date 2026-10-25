// SPDX-License-Identifier: MIT

package workboard_test

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
// lifecycle through it. A kernel opened without one has no board at all — the
// board operations refuse instead of calling into nothing. This test is such
// a host.
func withWorkboard(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	cfg.Workboard = store
	return cfg
}
