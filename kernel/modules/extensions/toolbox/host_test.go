// SPDX-License-Identifier: MIT

// Package toolbox_test is the toolbox module's own test for the CONTRACT's
// port (W5.4b).
//
// The module keeps the work (the catalog, the probes, the package-manager
// call); the contract keeps the rows and the port. Two things must hold:
//
//   - the module's rows ARE the contract's rows (aliases, not copies) — a
//     copy would fork the wire schema and the operation's schema would stop
//     matching the row the daemon answers with;
//   - the module's host IS the contract's port — the daemon hands it out and
//     the control plane never learns which package implemented it.
package toolbox_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolboxapi"
	"github.com/agezt/agezt/kernel/modules/extensions/toolbox"
)

// The module's host satisfies the contract's port.
var _ toolboxapi.Host = toolbox.Host{}

// TestTheModuleRowsAreTheContractRows: the rows are aliases, not copies.
func TestTheModuleRowsAreTheContractRows(t *testing.T) {
	for _, row := range []struct {
		name       string
		moduleType reflect.Type
		rowType    reflect.Type
	}{
		{"inventory", reflect.TypeOf(toolbox.Inventory{}), reflect.TypeOf(toolboxapi.Inventory{})},
		{"tool status", reflect.TypeOf(toolbox.ToolStatus{}), reflect.TypeOf(toolboxapi.ToolStatus{})},
		{"install result", reflect.TypeOf(toolbox.InstallResult{}), reflect.TypeOf(toolboxapi.InstallResult{})},
	} {
		if row.moduleType != row.rowType {
			t.Fatalf("the module's %s is %v, want the contract's row %v", row.name, row.moduleType, row.rowType)
		}
	}
}

// TestTheModuleHostIsTheContractPort: the host answers exactly what the
// module's own functions answer, and it never runs a package manager for a
// tool that is not in the catalog.
func TestTheModuleHostIsTheContractPort(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	host := toolbox.NewHost()

	inventory := host.Detect(context.Background())
	if inventory.OS == "" {
		t.Fatal("the host answered no operating system")
	}
	if len(inventory.Tools) != len(toolbox.Catalog) {
		t.Fatalf("the host answered %d tools, want the catalog's %d", len(inventory.Tools), len(toolbox.Catalog))
	}
	if inventory.InstalledCount+inventory.MissingCount != len(toolbox.Catalog) {
		t.Fatalf("the counts do not add up: %+v", inventory)
	}
	for _, status := range inventory.Tools {
		if status.Installable && status.Manager == "" {
			t.Fatalf("an installable tool without a manager: %+v", status)
		}
	}

	if outdated := host.Outdated(context.Background()); outdated == nil {
		t.Fatal("the host answered no outdated set")
	}

	result := host.Install(context.Background(), "__agezt_owned_not_in_catalog__")
	if !result.Skipped || result.OK || result.Error != "unknown tool" {
		t.Fatalf("EXPECTED:an unknown tool is skipped ACTUAL:%+v", result)
	}
}
