// SPDX-License-Identifier: MIT

package workboardtool

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
)

const (
	contractPkg = "github.com/agezt/agezt/kernel/contract/workboardapi"
	modulePkg   = "github.com/agezt/agezt/kernel/modules/work/workboard"
)

// TestTheToolNamesTheContractRows: the tool is a PLUGIN, so it may reach the
// contract layer and nothing else. The rows it speaks must therefore be the
// contract's — and the module that keeps them must not appear anywhere in
// the tool's own surface.
func TestTheToolNamesTheContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Kernel)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Kernel must be an interface, got %v", typ.Kind())
	}
	seen := map[string]bool{}
	for i := 0; i < typ.NumMethod(); i++ {
		collect(typ.Method(i).Type, seen)
	}
	if seen[modulePkg] {
		t.Fatalf("the plugin must not name the module %s", modulePkg)
	}
	if !seen[contractPkg] {
		t.Fatalf("the plugin must name the contract %s (it names %v)", contractPkg, seen)
	}
}

// TestTheStorePortIsTheContractsPort: the tool reads the board through the
// CONTRACT's port — one port, one spelling. The plugin used to reach into
// the module for the store.
func TestTheStorePortIsTheContractsPort(t *testing.T) {
	field, ok := reflect.TypeOf(Tool{}).FieldByName("store")
	if !ok {
		t.Fatal("the tool must keep the store it reads through")
	}
	want := reflect.TypeOf((*workboardapi.Store)(nil)).Elem()
	if field.Type != want {
		t.Fatalf("the tool's store must be the contract's port, got %v", field.Type)
	}
}

// TestANilStoreStaysANilPort: a store that was never wired must stay a nil
// port — a port that wraps a nil implementation is not nil, so the tool
// would call into nothing instead of reporting that the board is missing.
func TestANilStoreStaysANilPort(t *testing.T) {
	tool := New()
	tool.Bind(&fakeKernel{}, nil)
	if store := tool.currentStore(); store != nil {
		t.Fatalf("an unwired store must stay a nil port, got %v", store)
	}
}

// collect walks a type and records the package path of every named type it
// mentions (unwrapping pointers, slices, maps and funcs).
func collect(t reflect.Type, seen map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if pkg := t.PkgPath(); pkg != "" {
		seen[pkg] = true
	}
	switch t.Kind() {
	case reflect.Func:
		for i := 0; i < t.NumIn(); i++ {
			collect(t.In(i), seen)
		}
		for i := 0; i < t.NumOut(); i++ {
			collect(t.Out(i), seen)
		}
		return
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			collect(t.Field(i).Type, seen)
		}
		return
	}
}
