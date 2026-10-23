// SPDX-License-Identifier: MIT

package db

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/datalakeapi"
)

const (
	contractPkg = "github.com/agezt/agezt/kernel/contract/datalakeapi"
	modulePkg   = "github.com/agezt/agezt/kernel/modules/artifacts/datalake"
)

// TestTheToolNamesTheContractRows: the tool is a PLUGIN, so it may reach the
// contract layer and nothing else. The rows it speaks must therefore be the
// contract's — and the module that keeps them must not appear anywhere in
// the tool's own surface.
func TestTheToolNamesTheContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
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

// TestTheStoreIsTheContractsPort: the tool's store is the CONTRACT's port —
// one port, one spelling. The plugin used to carry its own interface with
// the module's rows in it.
func TestTheStoreIsTheContractsPort(t *testing.T) {
	if reflect.TypeOf((*Store)(nil)).Elem() != reflect.TypeOf((*datalakeapi.Lake)(nil)).Elem() {
		t.Fatal("the tool's store must be the contract's port")
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
