// SPDX-License-Identifier: MIT

package tasteapi

import (
	"reflect"
	"testing"
)

// TestTheStorePortIsExactlyFiveVerbsWide: the store port is what the layers
// ABOVE the module drive — five verbs, no more.
//
// The width is pinned because a WIDER port would let a caller reach past the
// library's own rules (the store validates, assigns ids and timestamps, and
// keeps its own on-disk shape), and a NARROWER one would force a second shape
// to be kept in step with this one. The module's own store has exactly these
// five; nothing more is part of the contract, because no layer above the
// module drives anything else.
func TestTheStorePortIsExactlyFiveVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 5 {
		t.Fatalf("the port has %d methods, want 5", typ.NumMethod())
	}
	for _, name := range []string{"Create", "Get", "List", "ForScope", "Delete"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheStorePortNamesOnlyContractRows: every type the port names must be
// the contract's row or a builtin. A module's type in a port's signature
// would drag that module into every layer that holds the port.
func TestTheStorePortNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	exemplar := reflect.TypeOf(Exemplar{})
	spec := reflect.TypeOf(CreateSpec{})
	filter := reflect.TypeOf(Filter{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	boolean := reflect.TypeOf(false)
	int64Type := reflect.TypeOf(int64(0))
	intType := reflect.TypeOf(0)
	slice := reflect.TypeOf([]Exemplar{})

	create, _ := typ.MethodByName("Create")
	// Create(spec CreateSpec, nowMS int64) (Exemplar, error)
	if create.Type.NumIn() != 2 || create.Type.In(0) != spec || create.Type.In(1) != int64Type {
		t.Errorf("Create must take (CreateSpec, int64), got %v", create.Type)
	}
	if create.Type.NumOut() != 2 || create.Type.Out(0) != exemplar || create.Type.Out(1) != errType {
		t.Errorf("Create must answer (Exemplar, error), got %v", create.Type)
	}

	get, _ := typ.MethodByName("Get")
	// Get(id string) (Exemplar, bool)
	if get.Type.NumIn() != 1 || get.Type.In(0) != str {
		t.Errorf("Get must take a string, got %v", get.Type)
	}
	if get.Type.NumOut() != 2 || get.Type.Out(0) != exemplar || get.Type.Out(1) != boolean {
		t.Errorf("Get must answer (Exemplar, bool), got %v", get.Type)
	}

	list, _ := typ.MethodByName("List")
	// List(f Filter) []Exemplar
	if list.Type.NumIn() != 1 || list.Type.In(0) != filter {
		t.Errorf("List must take a Filter, got %v", list.Type)
	}
	if list.Type.NumOut() != 1 || list.Type.Out(0) != slice {
		t.Errorf("List must answer []Exemplar, got %v", list.Type)
	}

	forScope, _ := typ.MethodByName("ForScope")
	// ForScope(scope string, limit int) []Exemplar
	if forScope.Type.NumIn() != 2 || forScope.Type.In(0) != str || forScope.Type.In(1) != intType {
		t.Errorf("ForScope must take (string, int), got %v", forScope.Type)
	}
	if forScope.Type.NumOut() != 1 || forScope.Type.Out(0) != slice {
		t.Errorf("ForScope must answer []Exemplar, got %v", forScope.Type)
	}

	del, _ := typ.MethodByName("Delete")
	// Delete(id string) error
	if del.Type.NumIn() != 1 || del.Type.In(0) != str {
		t.Errorf("Delete must take a string, got %v", del.Type)
	}
	if del.Type.NumOut() != 1 || del.Type.Out(0) != errType {
		t.Errorf("Delete must answer an error, got %v", del.Type)
	}
}

// TestThePortCarriesNoClock: the port takes the CALLER's clock (nowMS) — it
// must not read one itself.
//
// A port that reads the clock cannot be tested deterministically, and the
// contract layer may not name one (archcheck's principle P4).
func TestThePortCarriesNoClock(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		for j := 0; j < m.Type.NumIn(); j++ {
			if m.Type.In(j).String() == "time.Time" {
				t.Errorf("%s must not take a time.Time — the clock is the caller's", m.Name)
			}
		}
	}
}

// TestTheNotFoundSentinelIsTheContracts: the error the layers above the
// module compare against must be the contract's, so the module never has to
// be named to answer "does this exemplar exist?".
func TestTheNotFoundSentinelIsTheContracts(t *testing.T) {
	if ErrNotFound == nil {
		t.Fatal("the contract must own the not-found sentinel")
	}
	if ErrNotFound.Error() != "taste: not found" {
		t.Fatalf("the sentinel reads %q", ErrNotFound.Error())
	}
}

// TestTheRowsAreTheContractsOwn: the exemplar, its create input and its
// filter are rows — plain data with no behaviour and no clock.
func TestTheRowsAreTheContractsOwn(t *testing.T) {
	ex := Exemplar{ID: "e", Title: "t", Body: "b", Scope: "s", Tags: []string{"x"}, CreatedMS: 1, UpdatedMS: 2}
	if ex.ID != "e" || ex.Title != "t" || ex.Body != "b" || ex.Scope != "s" || len(ex.Tags) != 1 || ex.CreatedMS != 1 || ex.UpdatedMS != 2 {
		t.Fatalf("the exemplar row lost a field: %+v", ex)
	}
	spec := CreateSpec{Title: "t", Body: "b", Scope: "s", Tags: []string{"x"}}
	if spec.Title != "t" || spec.Body != "b" || spec.Scope != "s" || len(spec.Tags) != 1 {
		t.Fatalf("the create row lost a field: %+v", spec)
	}
	f := Filter{Scope: "s", Tag: "x", Limit: 3}
	if f.Scope != "s" || f.Tag != "x" || f.Limit != 3 {
		t.Fatalf("the filter row lost a field: %+v", f)
	}
}
