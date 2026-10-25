// SPDX-License-Identifier: MIT

package roster_test

// Provenance: the agents module's contract test for the W3.56 slice — that
//             the module's names ARE the contract's rows, so a layer above
//             the module compares against the contract's sentinels and gets
//             the same answers.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
)

// TestTheModuleAnswersTheContractsPorts: the module's store IS the contract's
// store port — not a look-alike. A second shape would mean a second set of
// fakes and a second set of drift.
func TestTheModuleAnswersTheContractsPorts(t *testing.T) {
	storeType := reflect.TypeOf((*roster.Store)(nil))
	if !storeType.Implements(reflect.TypeOf((*agentapi.Store)(nil)).Elem()) {
		t.Fatal("the module's store must satisfy the contract's store port")
	}
	// The reader is the store's read-only half — the same rows, fewer verbs.
	if !storeType.Implements(reflect.TypeOf((*agentapi.Reader)(nil)).Elem()) {
		t.Fatal("the module's store must satisfy the contract's reader port")
	}
}

// TestTheSentinelsAreTheContractsValues: the module's aliases point at the
// CONTRACT's sentinel values, so `errors.Is` matches on both sides of the
// module boundary.
//
// The identity is compared by the error's own address: a look-alike error the
// module raised itself would have the same message and a different address,
// and `errors.Is` against it would be silently false.
func TestTheSentinelsAreTheContractsValues(t *testing.T) {
	if reflect.ValueOf(roster.ErrNotFound).Pointer() != reflect.ValueOf(agentapi.ErrNotFound).Pointer() {
		t.Error("the module's ErrNotFound must BE the contract's ErrNotFound")
	}
	if reflect.ValueOf(roster.ErrRetired).Pointer() != reflect.ValueOf(agentapi.ErrRetired).Pointer() {
		t.Error("the module's ErrRetired must BE the contract's ErrRetired")
	}
	if !errors.Is(agentapi.ErrNotFound, agentapi.ErrNotFound) {
		t.Error("the contract's sentinel must match itself")
	}
}

// TestTheRowsAreTheContractsRows: the module's names are ALIASES of the
// contract's types, so the two are the same type — not two shapes that have
// to be converted at every boundary.
func TestTheRowsAreTheContractsRows(t *testing.T) {
	if reflect.TypeOf(roster.Profile{}) != reflect.TypeOf(agentapi.Profile{}) {
		t.Error("the module's Profile must BE the contract's Profile")
	}
	if reflect.TypeOf(roster.RepairResult{}) != reflect.TypeOf(agentapi.RepairResult{}) {
		t.Error("the module's RepairResult must BE the contract's RepairResult")
	}
	// The repair surface is an alias too, so a caller may hold either name.
	if reflect.TypeOf((*roster.RepairSource)(nil)).Elem() != reflect.TypeOf((*agentapi.RepairSource)(nil)).Elem() {
		t.Error("the module's RepairSource must BE the contract's RepairSource")
	}
}

// TestTheStoreRaisesTheContractsSentinels: the store's refusals ARE the
// contract's sentinels. A layer above the module compares against
// `agentapi.ErrNotFound`; a module that raised a look-alike error of its own
// would leave that comparison silently false — the layer would report "no
// such agent" as "some other failure".
func TestTheStoreRaisesTheContractsSentinels(t *testing.T) {
	store, err := roster.Open(t.TempDir())
	if err != nil {
		t.Fatalf("roster.Open: %v", err)
	}
	if _, err := store.SetEnabled("missing", false); !errors.Is(err, agentapi.ErrNotFound) {
		t.Fatalf("an unknown agent must raise the contract's ErrNotFound, got %v", err)
	}
	if _, err := store.Update("missing", func(*agentapi.Profile) {}); !errors.Is(err, agentapi.ErrNotFound) {
		t.Fatalf("an unknown agent must raise the contract's ErrNotFound, got %v", err)
	}
	if _, err := store.SetRetired("missing", true); !errors.Is(err, agentapi.ErrNotFound) {
		t.Fatalf("an unknown agent must raise the contract's ErrNotFound, got %v", err)
	}
	if _, err := store.Add(roster.Profile{Slug: "old", Name: "Old"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := store.SetRetired("old", true); err != nil {
		t.Fatalf("SetRetired: %v", err)
	}
	if _, err := store.SetEnabled("old", true); !errors.Is(err, agentapi.ErrRetired) {
		t.Fatalf("resuming a graveyard agent must raise the contract's ErrRetired, got %v", err)
	}
}

// TestTheRunbookRuleTravelsWithTheRow: the module's runbook builder answers
// the contract's rule — the same payload, not a second derivation of the
// wake contract.
func TestTheRunbookRuleTravelsWithTheRow(t *testing.T) {
	no := false
	p := roster.Profile{Slug: "worker", Enabled: true, DirectCallable: &no, ParentAgent: "lead"}
	fromModule := roster.AutonomyRunbook(p)
	fromContract := agentapi.AutonomyRunbook(p)
	if !reflect.DeepEqual(fromModule, fromContract) {
		t.Fatalf("the module's runbook differs from the contract's: %+v vs %+v", fromModule, fromContract)
	}
	if got := fromModule["route_contract"]; got != "leader:lead" {
		t.Fatalf("a managed agent routes to its parent, got %v", got)
	}
}
