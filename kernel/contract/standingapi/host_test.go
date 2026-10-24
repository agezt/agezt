// SPDX-License-Identifier: MIT

package standingapi

import (
	"reflect"
	"testing"
)

// TestTheStandingHostPortHasExactlyThreeMethods: the standing port is the
// whole of what the standing-orders tool may do to the daemon's orders —
// add, remove, list. A fourth method would be a new reach into the daemon,
// and it would have to be earned here.
func TestTheStandingHostPortHasExactlyThreeMethods(t *testing.T) {
	typ := reflect.TypeOf((*Host)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Host must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 3 {
		t.Fatalf("the port has %d methods, want 3", typ.NumMethod())
	}
	for _, name := range []string{"AddStanding", "RemoveStanding", "StandingList"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheStandingHostNamesOnlyContractRows: every type the port names must
// be the contract's row or a builtin. A module's type in a port's signature
// would drag that module into every plugin that holds the port — the very
// leak the contract layer exists to stop.
func TestTheStandingHostNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Host)(nil)).Elem()
	order := reflect.TypeOf(Order{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")

	add, _ := typ.MethodByName("AddStanding")
	// AddStanding(o Order) (Order, error)
	if add.Type.NumIn() != 1 || add.Type.In(0) != order {
		t.Errorf("AddStanding must take the contract's order, got %v", add.Type)
	}
	if add.Type.NumOut() != 2 || add.Type.Out(0) != order || add.Type.Out(1) != errType {
		t.Errorf("AddStanding must answer (Order, error), got %v", add.Type)
	}

	remove, _ := typ.MethodByName("RemoveStanding")
	// RemoveStanding(id string) (bool, error)
	if remove.Type.NumIn() != 1 || remove.Type.In(0) != str {
		t.Errorf("RemoveStanding must take a string, got %v", remove.Type)
	}
	if remove.Type.NumOut() != 2 || remove.Type.Out(0) != reflect.TypeOf(false) || remove.Type.Out(1) != errType {
		t.Errorf("RemoveStanding must answer (bool, error), got %v", remove.Type)
	}

	list, _ := typ.MethodByName("StandingList")
	// StandingList() []Order
	if list.Type.NumIn() != 0 || list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]Order{}) {
		t.Errorf("StandingList must answer []Order, got %v", list.Type)
	}
}
