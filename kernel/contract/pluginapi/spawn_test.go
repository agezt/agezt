// SPDX-License-Identifier: MIT

package pluginapi

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// TestTheSpawnPortHasExactlyOneMethod: the spawn port is the whole of what a
// plugin may ask of the host's process handling — launch it. A second method
// would be a new reach into the host's module, and it would have to be
// earned here.
func TestTheSpawnPortHasExactlyOneMethod(t *testing.T) {
	typ := reflect.TypeOf((*Spawner)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Spawner must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 1 {
		t.Fatalf("the port has %d methods, want 1", typ.NumMethod())
	}
	spawn, ok := typ.MethodByName("Spawn")
	if !ok {
		t.Fatal("the port must have a Spawn method")
	}
	if spawn.Type.NumIn() != 2 {
		t.Errorf("Spawn must take (ctx, request), got %v", spawn.Type)
	}
	if spawn.Type.In(1) != reflect.TypeOf(SpawnRequest{}) {
		t.Errorf("Spawn must take a SpawnRequest, got %v", spawn.Type.In(1))
	}
	if spawn.Type.NumOut() != 2 {
		t.Fatalf("Spawn must answer (Spawned, error), got %v", spawn.Type)
	}
	if spawn.Type.Out(0) != reflect.TypeOf((*Spawned)(nil)).Elem() {
		t.Errorf("Spawn must answer a Spawned, got %v", spawn.Type.Out(0))
	}
	if spawn.Type.Out(1).Kind() != reflect.Interface {
		t.Errorf("Spawn must answer an error, got %v", spawn.Type.Out(1))
	}
}

// TestTheSpawnedPortHasExactlyThreeVerbs: the running plugin answers three
// verbs — its tools, their declared capabilities, and the close. The host's
// own verbs (invoke, reload, liveness) stay with the host: a plugin that
// could name them would be able to drive the host's process directly.
func TestTheSpawnedPortHasExactlyThreeVerbs(t *testing.T) {
	typ := reflect.TypeOf((*Spawned)(nil)).Elem()
	if typ.NumMethod() != 3 {
		t.Fatalf("the port has %d methods, want 3", typ.NumMethod())
	}
	for _, name := range []string{"Tools", "ToolCapabilities", "Close"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheRequestCarriesNoConcreteHost: the request is a ROW — it may not
// carry a concrete host type, or the port would drag the host's module in
// through the back door. Its one structured field is typed with the
// contract's own tool vocabulary.
func TestTheRequestCarriesNoConcreteHost(t *testing.T) {
	typ := reflect.TypeOf(SpawnRequest{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.Ptr {
			t.Errorf("the request field %q carries a pointer %v — a row must stay a row", f.Name, f.Type)
		}
		if f.Type.Kind() == reflect.Struct {
			t.Errorf("the request field %q carries a struct %v — a row must stay a row", f.Name, f.Type)
		}
		if f.Type.Kind() == reflect.Map && f.Type != reflect.TypeOf(map[string]toolapi.Tool(nil)) {
			t.Errorf("the request field %q carries a map %v that is not the contract's tool vocabulary", f.Name, f.Type)
		}
	}
}
