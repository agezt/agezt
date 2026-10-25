// SPDX-License-Identifier: MIT

package system_test

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/modules/knowledge/worldmodel"
	"github.com/agezt/agezt/kernel/runtime"
)

// withWorld opens the world-model file the composition root would open (W3.54)
// and hands the kernel the contract's store port and the BUILDER.
//
// The file is the DAEMON's: the kernel holds the contract's ports, and the
// daemon opens the file and names the module. A host with no world model
// registers no world tool — and the kernel's tool set is part of what the
// status surface reports.
func withWorld(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	store, err := worldmodel.Open(filepath.Join(cfg.BaseDir, "worldmodel"))
	if err != nil {
		t.Fatalf("open the world model store: %v", err)
	}
	cfg.WorldStore = store
	cfg.WorldGraph = func(s worldmodelapi.Store, pub worldmodelapi.Publisher) worldmodelapi.Graph {
		return worldmodel.NewGraph(s, pub)
	}
	return cfg
}
