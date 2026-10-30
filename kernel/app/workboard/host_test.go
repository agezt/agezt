// SPDX-License-Identifier: MIT

package workboard_test

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/kernel/runtime"
)

// withWorkboard opens the task board the composition root would open (W3.47)
// and hands the daemon the board: the store port AND the module's service.
//
// The board's file is the DAEMON's: the kernel holds the port, and the
// lifecycle is driven through the WORK MODULE's service, which the
// composition root builds over that port (W5.6). A daemon opened without a
// board has none at all — the board operations refuse instead of calling into
// nothing. This test is such a host.
func withWorkboard(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	store, err := workboard.OpenStore(filepath.Join(cfg.BaseDir, "workboard"))
	if err != nil {
		t.Fatalf("open the workboard store: %v", err)
	}
	cfg.Workboard = store
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("open the daemon: %v", err)
	}
	k.SetWorkboard(workboard.NewService(k.Workboard(), k.WorkboardDependencies()))
	return k
}
