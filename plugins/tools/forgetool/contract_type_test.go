// SPDX-License-Identifier: MIT

package forgetool

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
)

const (
	contractPkg = "github.com/agezt/agezt/kernel/contract/toolforgeapi"
	modulePkg   = "github.com/agezt/agezt/kernel/modules/extensions/toolforge"
)

// TestTheToolNamesTheContractRows: the tool is a PLUGIN, so it may reach the
// contract layer and nothing else. The rows it speaks must therefore be the
// contract's — and the module that keeps them must not appear anywhere in
// the tool's own surface. This walks the kernel interface the tool drives and
// collects every package it names.
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

// TestTheToolReadsThroughTheContractsPort: the read ops (op=list, op=show)
// read the store through the CONTRACT's port — a plugin may not name the
// concrete store, so the port is what the daemon hands over.
func TestTheToolReadsThroughTheContractsPort(t *testing.T) {
	typ := reflect.TypeOf((*Tool)(nil)).Elem()
	field, ok := typ.FieldByName("store")
	if !ok {
		t.Fatal("the tool must keep the store it reads through")
	}
	if field.Type != reflect.TypeOf((*toolforgeapi.Store)(nil)).Elem() {
		t.Fatalf("the store field = %v, want the contract's port", field.Type)
	}

	bind, ok := reflect.TypeOf((*Tool)(nil)).MethodByName("Bind")
	if !ok {
		t.Fatal("the tool must have a Bind")
	}
	seen := map[string]bool{}
	collect(bind.Type, seen)
	if seen[modulePkg] {
		t.Fatalf("Bind must not name the module %s", modulePkg)
	}
	if !seen[contractPkg] {
		t.Fatalf("Bind must take the contract's port, it names %v", seen)
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
