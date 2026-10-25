// SPDX-License-Identifier: MIT

package standingapi

import (
	"errors"
	"reflect"
	"testing"
)

// TestTheStandingStorePortHasExactlySevenMethods: the store port is the whole
// of what the layers above the module may do to the daemon's standing orders.
//
// The width is pinned because a WIDER port would let a caller reach past the
// library's own rules (the store validates, assigns ids and protects the
// identity fields), and a NARROWER one would force a second shape to be kept
// in step with this one.
func TestTheStandingStorePortHasExactlySevenMethods(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 7 {
		t.Fatalf("the port has %d methods, want 7", typ.NumMethod())
	}
	for _, name := range []string{"Add", "SetEnabled", "Update", "Remove", "Get", "List", "Count"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheStandingStoreNamesOnlyContractRows: every type the port names must be
// the contract's row or a builtin. A module's type in a port's signature would
// drag that module into every layer that holds the port — the very leak the
// contract layer exists to stop.
func TestTheStandingStoreNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	order := reflect.TypeOf(Order{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	boolean := reflect.TypeOf(false)
	integer := reflect.TypeOf(0)
	mutator := reflect.TypeOf(func(*Order) {})

	add, _ := typ.MethodByName("Add")
	// Add(o Order) (Order, error)
	if add.Type.NumIn() != 1 || add.Type.In(0) != order {
		t.Errorf("Add must take the contract's order, got %v", add.Type)
	}
	if add.Type.NumOut() != 2 || add.Type.Out(0) != order || add.Type.Out(1) != errType {
		t.Errorf("Add must answer (Order, error), got %v", add.Type)
	}

	setEnabled, _ := typ.MethodByName("SetEnabled")
	// SetEnabled(id string, enabled bool) (Order, error)
	if setEnabled.Type.NumIn() != 2 || setEnabled.Type.In(0) != str || setEnabled.Type.In(1) != boolean {
		t.Errorf("SetEnabled must take (string, bool), got %v", setEnabled.Type)
	}
	if setEnabled.Type.NumOut() != 2 || setEnabled.Type.Out(0) != order || setEnabled.Type.Out(1) != errType {
		t.Errorf("SetEnabled must answer (Order, error), got %v", setEnabled.Type)
	}

	update, _ := typ.MethodByName("Update")
	// Update(id string, mutate func(*Order)) (Order, error)
	if update.Type.NumIn() != 2 || update.Type.In(0) != str || update.Type.In(1) != mutator {
		t.Errorf("Update must take (string, func(*Order)), got %v", update.Type)
	}
	if update.Type.NumOut() != 2 || update.Type.Out(0) != order || update.Type.Out(1) != errType {
		t.Errorf("Update must answer (Order, error), got %v", update.Type)
	}

	remove, _ := typ.MethodByName("Remove")
	// Remove(id string) (bool, error)
	if remove.Type.NumIn() != 1 || remove.Type.In(0) != str {
		t.Errorf("Remove must take a string, got %v", remove.Type)
	}
	if remove.Type.NumOut() != 2 || remove.Type.Out(0) != boolean || remove.Type.Out(1) != errType {
		t.Errorf("Remove must answer (bool, error), got %v", remove.Type)
	}

	get, _ := typ.MethodByName("Get")
	// Get(id string) (Order, bool)
	if get.Type.NumIn() != 1 || get.Type.In(0) != str {
		t.Errorf("Get must take a string, got %v", get.Type)
	}
	if get.Type.NumOut() != 2 || get.Type.Out(0) != order || get.Type.Out(1) != boolean {
		t.Errorf("Get must answer (Order, bool), got %v", get.Type)
	}

	list, _ := typ.MethodByName("List")
	// List() []Order
	if list.Type.NumIn() != 0 || list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]Order{}) {
		t.Errorf("List must answer []Order, got %v", list.Type)
	}

	count, _ := typ.MethodByName("Count")
	// Count() int
	if count.Type.NumIn() != 0 || count.Type.NumOut() != 1 || count.Type.Out(0) != integer {
		t.Errorf("Count must answer an int, got %v", count.Type)
	}
}

// TestTheNotFoundErrorNamesItsSubject: the error the layers above compare
// against must say WHAT was not found. A bare "not found" would leave an
// operator guessing between a missing order and a missing library.
func TestTheNotFoundErrorNamesItsSubject(t *testing.T) {
	if ErrNotFound.Error() != "standing: order not found" {
		t.Fatalf("the error says %q", ErrNotFound.Error())
	}
}

// TestTheNotFoundErrorIsComparable: the kernel turns "not found" into "the
// order does not exist" — the comparison must work with errors.Is, or every
// edit of an unknown id would look like a failure.
func TestTheNotFoundErrorIsComparable(t *testing.T) {
	wrapped := errors.Join(ErrNotFound, errors.New("context"))
	if !errors.Is(wrapped, ErrNotFound) {
		t.Fatal("the error must survive wrapping")
	}
}
