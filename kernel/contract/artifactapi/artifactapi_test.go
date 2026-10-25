// SPDX-License-Identifier: MIT

package artifactapi

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
)

// TestTheIndexPortHasExactlyFourMethods: the artifact port is the whole of
// what a tool may do to the daemon's index — store an entry, list them, read
// one back, delete one. A fifth method would be a new reach into the daemon,
// and it would have to be earned here.
//
// The count is four and not one because the four tools that drive the index
// need four different verbs: the artifacts tool reads and deletes, the three
// writers store. One port, four verbs — the daemon keeps a single wiring
// site instead of four.
func TestTheIndexPortHasExactlyFourMethods(t *testing.T) {
	typ := reflect.TypeOf((*Index)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Index must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 4 {
		t.Fatalf("the port has %d methods, want 4", typ.NumMethod())
	}
	for _, name := range []string{"PutEntry", "List", "Bytes", "Delete"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheIndexPortNamesOnlyContractRows: every type the port names must be a
// contract row or a builtin. A module's type in a port's signature would
// drag that module into every plugin that holds the port — the very leak the
// contract layer exists to stop — and it would break the moment the module
// moved.
func TestTheIndexPortNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Index)(nil)).Elem()
	row := reflect.TypeOf(remoteapi.Artifact{})
	filter := reflect.TypeOf(remoteapi.Filter{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	byteSlice := reflect.TypeOf([]byte{})
	str := reflect.TypeOf("")

	check := func(name string, m reflect.Method, wantIn []reflect.Type, wantOut []reflect.Type) {
		t.Helper()
		// An interface method's type carries no receiver: In(0) is the first
		// ARGUMENT.
		if m.Type.NumIn() != len(wantIn) {
			t.Errorf("%s must take %d arguments, got %v", name, len(wantIn), m.Type)
			return
		}
		for i, want := range wantIn {
			if m.Type.In(i) != want {
				t.Errorf("%s argument %d must be %v, got %v", name, i, want, m.Type.In(i))
			}
		}
		if m.Type.NumOut() != len(wantOut) {
			t.Errorf("%s must answer %d values, got %v", name, len(wantOut), m.Type)
			return
		}
		for i, want := range wantOut {
			if m.Type.Out(i) != want {
				t.Errorf("%s answer %d must be %v, got %v", name, i, want, m.Type.Out(i))
			}
		}
	}

	m, _ := typ.MethodByName("PutEntry")
	// PutEntry(meta remoteapi.Artifact, data []byte, createdMs int64) (remoteapi.Artifact, error)
	check("PutEntry", m, []reflect.Type{row, byteSlice, reflect.TypeOf(int64(0))}, []reflect.Type{row, errType})

	m, _ = typ.MethodByName("List")
	// List(f remoteapi.Filter) []remoteapi.Artifact
	check("List", m, []reflect.Type{filter}, []reflect.Type{reflect.TypeOf([]remoteapi.Artifact{})})

	m, _ = typ.MethodByName("Bytes")
	// Bytes(id string) ([]byte, remoteapi.Artifact, error)
	check("Bytes", m, []reflect.Type{str}, []reflect.Type{byteSlice, row, errType})

	m, _ = typ.MethodByName("Delete")
	// Delete(id string) error
	check("Delete", m, []reflect.Type{str}, []reflect.Type{errType})
}

// TestTheHostPortAddsTheHostsVerbs: the host-facing port is the tool port
// plus the six verbs the daemon and the kernel use to RUN the index — count
// it, find one entry, record an entry for bytes that are already in the blob
// store, count the stale ones, reclaim them, and install the ref guard.
//
// The count is six because each of them is a verb the host asked for by name:
// the control plane counts and reads, the pulse path records a ref, the reaper
// counts stale entries, the collector reclaims them, and the kernel installs
// the journal's guard.
func TestTheHostPortAddsTheHostsVerbs(t *testing.T) {
	typ := reflect.TypeOf((*HostIndex)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("HostIndex must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 10 {
		t.Fatalf("the host port has %d methods, want 10", typ.NumMethod())
	}
	for _, name := range []string{"Get", "Count", "IndexRef", "StaleEntries", "Collect", "SetRefGuard"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the host port must have a %s method", name)
		}
	}
}

// TestTheToolPortCannotReachTheHostsVerbs: the four-verb port is what a
// PLUGIN holds, so the host's verbs must stay off it.
//
// Two of them are not merely unnecessary for a tool but dangerous: Collect
// deletes entries, and SetRefGuard switches off the journal's protection of
// the bytes the journal still names — a plugin that could clear the guard
// would make the collector delete bytes a run's output still points at.
//
// This is the rule the split exists for: the tool port stays exactly as
// narrow as the guard test above says, and the wider verbs live on a port
// only the host holds.
func TestTheToolPortCannotReachTheHostsVerbs(t *testing.T) {
	typ := reflect.TypeOf((*Index)(nil)).Elem()
	for _, name := range []string{"Get", "Count", "IndexRef", "StaleEntries", "Collect", "SetRefGuard"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("the tool-facing port must not have a %s method", name)
		}
	}
}
