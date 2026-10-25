// SPDX-License-Identifier: MIT

package worldmodel

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
)

// TestTheRowsAreTheContractsRows: the module keeps the names its own callers
// use as ALIASES of the contract's rows — one row, one spelling of it.
//
// The check is on the TYPE IDENTITY, not on a field-by-field comparison: two
// shapes that happen to have the same fields are still two shapes, and a
// caller that passes one where the other is expected has to convert.
func TestTheRowsAreTheContractsRows(t *testing.T) {
	if reflect.TypeOf(Entity{}) != reflect.TypeOf(worldmodelapi.Entity{}) {
		t.Errorf("the entity row is not the contract's row")
	}
	if reflect.TypeOf(Relation{}) != reflect.TypeOf(worldmodelapi.Relation{}) {
		t.Errorf("the relation row is not the contract's row")
	}
	if reflect.TypeOf(ScoredEntity{}) != reflect.TypeOf(worldmodelapi.ScoredEntity{}) {
		t.Errorf("the scored row is not the contract's row")
	}
	if reflect.TypeOf(Neighbor{}) != reflect.TypeOf(worldmodelapi.Neighbor{}) {
		t.Errorf("the neighbor row is not the contract's row")
	}
	if reflect.TypeOf(UpsertSpec{}) != reflect.TypeOf(worldmodelapi.UpsertSpec{}) {
		t.Errorf("the upsert row is not the contract's row")
	}
	if reflect.TypeOf(DecayOptions{}) != reflect.TypeOf(worldmodelapi.DecayOptions{}) {
		t.Errorf("the decay-options row is not the contract's row")
	}
	if reflect.TypeOf(KindProject) != reflect.TypeOf(worldmodelapi.KindProject) {
		t.Errorf("the kind constants are not the contract's")
	}
	if reflect.TypeOf(VerbOwns) != reflect.TypeOf(worldmodelapi.VerbOwns) {
		t.Errorf("the verb constants are not the contract's")
	}
}

// TestTheActiveRulesAreTheContracts: the rule that decides whether an entity
// still counts travels with the row into the contract.
//
// A Go type alias cannot carry methods — so if the rule had stayed in the
// module, the alias would have lost it.
func TestTheActiveRulesAreTheContracts(t *testing.T) {
	if !(Entity{}).Active() {
		t.Fatal("the aliased entity must keep the contract's active rule")
	}
	if (Entity{Tombstoned: true}).Active() {
		t.Fatal("the aliased entity must keep the contract's active rule")
	}
	if !(Relation{}).Active() {
		t.Fatal("the aliased relation must keep the contract's active rule")
	}
}

// TestTheSentinelIsTheContracts: the module's sentinel is the contract's, so
// a layer above the module can compare against it without naming the module.
func TestTheSentinelIsTheContracts(t *testing.T) {
	if ErrEmptyName != worldmodelapi.ErrEmptyName {
		t.Fatal("the module's empty-name sentinel must be the contract's")
	}
}

// TestTheGraphAndStoreAnswerTheContractsPorts: the module's graph and store
// are the implementations of the contract's ports — the shapes the layers
// above hold.
func TestTheGraphAndStoreAnswerTheContractsPorts(t *testing.T) {
	var _ worldmodelapi.Graph = (*Graph)(nil)
	var _ worldmodelapi.Store = (*FileStore)(nil)
}

// TestTheGraphDoesNotKnowTheBus: the graph journals through the CONTRACT's
// publisher port, not the kernel's bus.
//
// The check is on the module's own files: a module that names the bus would
// drag the journal into every layer that holds the graph's port.
func TestTheGraphDoesNotKnowTheBus(t *testing.T) {
	for _, name := range []string{"manager.go", "manager_query.go", "manager_tool.go", "decay.go"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(body), "kernel/bus") {
			t.Fatalf("%s must not import the kernel bus — the graph journals through the contract's publisher port", name)
		}
	}
}
