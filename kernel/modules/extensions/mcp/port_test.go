// SPDX-License-Identifier: MIT

package mcp

// Provenance: the module's own test for the contract PORTS (W3.58). The
//
//	shapes the layers above speak live in the contract, and this module keeps
//	their NAMES as aliases — an alias, not a copy. A copy would satisfy every
//	assignment and still be a second shape: a Conn that is not the contract's
//	Conn cannot be handed to a caller that holds the contract's port.
//
//	The identity is asserted as a function type — an assignment would also
//	accept a copy with the same fields, while a function type requires
//	identity.

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/mcpapi"
)

// The module's ToolDef IS the contract's.
var _ func(mcpapi.ToolDef) = func(ToolDef) {}

// The module's Conn IS the contract's.
var _ func(mcpapi.Conn) = func(Conn) {}

// The module's Dialer IS the contract's.
var _ func(context.Context, string, []string, map[string]string) (mcpapi.Conn, error) = Dialer(nil)

// The module's HTTPDialer IS the contract's.
var _ func(context.Context, string, map[string]string) (mcpapi.Conn, error) = HTTPDialer(nil)

// TestTheShapesAreTheContractsShapes: the module's shapes are the CONTRACT's
// — by type identity, not by a copy that happens to have the same fields.
//
// A copy would satisfy every assignment and still be a second shape: a Conn
// that is not the contract's Conn cannot be handed to a caller that holds the
// contract's port, and a ToolDef that is not the contract's ToolDef cannot be
// put in a slice the contract's Conn returns.
func TestTheShapesAreTheContractsShapes(t *testing.T) {
	for _, shape := range []struct {
		name string
		got  reflect.Type
		want reflect.Type
	}{
		{"ToolDef", reflect.TypeFor[ToolDef](), reflect.TypeFor[mcpapi.ToolDef]()},
		{"Conn", reflect.TypeFor[Conn](), reflect.TypeFor[mcpapi.Conn]()},
		{"Dialer", reflect.TypeFor[Dialer](), reflect.TypeFor[mcpapi.Dialer]()},
		{"HTTPDialer", reflect.TypeFor[HTTPDialer](), reflect.TypeFor[mcpapi.HTTPDialer]()},
	} {
		if shape.got != shape.want {
			t.Fatalf("the module's %s is %v, want the contract's %v", shape.name, shape.got, shape.want)
		}
	}
}

// TestErrNotFoundIsTheContractsSentinel: the sentinel the module raises is
// the CONTRACT's — the app layer compares against it, and a look-alike would
// leave every errors.Is comparison silently false.
func TestErrNotFoundIsTheContractsSentinel(t *testing.T) {
	if ErrNotFound != mcpapi.ErrNotFound {
		t.Fatal("the module's not-found sentinel is not the contract's")
	}
	if !errors.Is(ErrNotFound, mcpapi.ErrNotFound) {
		t.Fatal("the module's sentinel does not match the contract's")
	}
}

// TestStoreIsTheContractsRegistry: the store satisfies the contract's port —
// and it answers through it, sentinel included.
func TestStoreIsTheContractsRegistry(t *testing.T) {
	store, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	var port mcpapi.Registry = store
	if port.Count() != 0 {
		t.Fatalf("a fresh registry holds %d servers", port.Count())
	}
	saved, err := port.Add(Server{Name: "owned", Command: "never-executed"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if saved.Name != "owned" || port.Count() != 1 {
		t.Fatalf("the registry did not keep the row: %+v / %d", saved, port.Count())
	}
	got, found := port.Get("owned")
	if !found || got.ID != saved.ID {
		t.Fatalf("the registry did not answer the row: %+v / %v", got, found)
	}
	if _, found := port.Get("ghost"); found {
		t.Fatal("the registry invented a row")
	}
	// The missing row is the CONTRACT's sentinel, reachable through the port.
	if _, err := port.SetEnabled("ghost", true); !errors.Is(err, mcpapi.ErrNotFound) {
		t.Fatalf("a missing row must raise the contract's sentinel, got %v", err)
	}
	if _, _, err := port.Remove("ghost"); err != nil {
		t.Fatalf("removing a missing row must not fail, got %v", err)
	}
}
