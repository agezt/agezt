// SPDX-License-Identifier: MIT

package acpapi

import (
	"reflect"
	"testing"
)

// TestThePortHasExactlyThreeMethods: the catalogue port is the whole of what
// the ACP bridge tool may ask of the catalogue module — is any agent
// installed, which slugs are, and which command a slug maps to. A fourth
// method would be a new reach into the module, and it would have to be earned
// here.
func TestThePortHasExactlyThreeMethods(t *testing.T) {
	typ := reflect.TypeOf((*Catalog)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Catalog must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 3 {
		t.Fatalf("the port has %d methods, want 3: %v", typ.NumMethod(), methodNames(typ))
	}
}

// TestThePortCarriesNoRows: the port answers with plain strings and booleans
// — it carries no module's row. The catalogue's own Agent row stays in the
// module that owns it; a plugin may not name it.
func TestThePortCarriesNoRows(t *testing.T) {
	typ := reflect.TypeOf((*Catalog)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		for j := 0; j < m.Type.NumIn(); j++ {
			if in := m.Type.In(j); in.Kind() == reflect.Struct {
				t.Errorf("%s must not take a struct row, got %v", m.Name, in)
			}
		}
		for j := 0; j < m.Type.NumOut(); j++ {
			if out := m.Type.Out(j); out.Kind() == reflect.Struct {
				t.Errorf("%s must not answer a struct row, got %v", m.Name, out)
			}
		}
	}
}

// TestThePortSignatures: the three signatures are pinned — a bool, a slice of
// slugs, and the (command, ok) pair the bridge tool resolves a selector
// with. A renamed or reshaped method would change what the tool can ask for.
func TestThePortSignatures(t *testing.T) {
	typ := reflect.TypeOf((*Catalog)(nil)).Elem()

	anyInstalled, ok := typ.MethodByName("AnyInstalled")
	if !ok {
		t.Fatal("the port must have an AnyInstalled method")
	}
	if anyInstalled.Type.NumOut() != 1 || anyInstalled.Type.Out(0).Kind() != reflect.Bool {
		t.Errorf("AnyInstalled must answer a bool, got %v", anyInstalled.Type)
	}

	slugs, ok := typ.MethodByName("InstalledSlugs")
	if !ok {
		t.Fatal("the port must have an InstalledSlugs method")
	}
	if slugs.Type.NumOut() != 1 || slugs.Type.Out(0) != reflect.TypeOf([]string(nil)) {
		t.Errorf("InstalledSlugs must answer []string, got %v", slugs.Type)
	}

	resolve, ok := typ.MethodByName("ResolveCommand")
	if !ok {
		t.Fatal("the port must have a ResolveCommand method")
	}
	// An interface method's type carries no receiver.
	if resolve.Type.NumIn() != 2 {
		t.Errorf("ResolveCommand must take (ref, fallback), got %v", resolve.Type)
	}
	if resolve.Type.In(0) != reflect.TypeOf("") || resolve.Type.In(1) != reflect.TypeOf("") {
		t.Errorf("ResolveCommand must take two strings, got %v", resolve.Type)
	}
	if resolve.Type.NumOut() != 2 {
		t.Fatalf("ResolveCommand must answer (cmd, ok), got %v", resolve.Type)
	}
	if resolve.Type.Out(0) != reflect.TypeOf("") || resolve.Type.Out(1).Kind() != reflect.Bool {
		t.Errorf("ResolveCommand must answer (string, bool), got %v", resolve.Type)
	}
}

// methodNames lists the port's methods — for the failure messages.
func methodNames(typ reflect.Type) []string {
	out := make([]string, 0, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		out = append(out, typ.Method(i).Name)
	}
	return out
}
