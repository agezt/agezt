// SPDX-License-Identifier: MIT

package seatapi

import (
	"reflect"
	"testing"
)

// TestTheSeatRowIsTheContractRow pins the seat row and the port it is read
// through.
//
// The seat catalog moved into the contract layer in W3.41: the row is pure
// data, and the host holds the port rather than a module's concrete store.
// The module keeps its name as an ALIAS of this row, so there is one seat and
// one spelling of it — a copy with the same shape would be two types, and the
// host would be holding the wrong one without any compiler saying so.
func TestTheSeatRowIsTheContractRow(t *testing.T) {
	// A row may not grow a field: every field is a seat axis somebody sets,
	// and a field nobody names is a row waiting to drift.
	if n := reflect.TypeOf(Seat{}).NumField(); n != 9 {
		t.Errorf("Seat has %d fields, want 9 — a row may not grow a field", n)
	}

	// The wire shape is the contract: the seat is read from disk and written
	// over the app's JSON, so the tags are the row's public shape.
	seatType := reflect.TypeOf(Seat{})
	for name, want := range map[string]string{
		"ID":               "id",
		"Name":             "name",
		"Description":      "description",
		"ExecutionProfile": "execution_profile,omitempty",
		"ModelChain":       "model_chain,omitempty",
		"Tools":            "tools,omitempty",
		"RestrictTools":    "restrict_tools,omitempty",
		"Icon":             "icon,omitempty",
		"Builtin":          "builtin,omitempty",
	} {
		f, ok := seatType.FieldByName(name)
		if !ok {
			t.Fatalf("Seat has no %s field", name)
		}
		if got := f.Tag.Get("json"); got != want {
			t.Errorf("Seat.%s json tag is %q, want %q", name, got, want)
		}
	}
}

// TestTheStorePortHasTheVerbsTheHostUses pins the port's shape.
//
// The host asks the catalog five things: the list, one seat, whether a seat
// may be dispatched under, and the two operator edits. The port is what the
// kernel holds — a verb nobody calls is a promise the host cannot check, and a
// missing verb is a call the host cannot make.
func TestTheStorePortHasTheVerbsTheHostUses(t *testing.T) {
	storeType := reflect.TypeOf((*Store)(nil)).Elem()
	if storeType.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", storeType.Kind())
	}
	want := map[string][]reflect.Type{
		"List":   {},
		"Get":    {reflect.TypeOf("")},
		"Valid":  {reflect.TypeOf("")},
		"Create": {reflect.TypeOf(Seat{})},
		"Delete": {reflect.TypeOf("")},
	}
	if storeType.NumMethod() != len(want) {
		t.Errorf("Store has %d methods, want %d — a verb nobody calls is a promise nobody can check", storeType.NumMethod(), len(want))
	}
	for name, ins := range want {
		m, ok := storeType.MethodByName(name)
		if !ok {
			t.Errorf("Store has no %s method", name)
			continue
		}
		if m.Type.NumIn() != len(ins) {
			t.Errorf("Store.%s must take %d arguments, got %v", name, len(ins), m.Type)
			continue
		}
		for i, in := range ins {
			if got := m.Type.In(i); got != in {
				t.Errorf("Store.%s argument %d is %v, want %v", name, i+1, got, in)
			}
		}
	}
}
