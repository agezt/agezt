// SPDX-License-Identifier: MIT
package state_test

import (
	"reflect"
	"testing"

	appstate "github.com/agezt/agezt/kernel/app/state"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The state family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the store the port hands over is the
// daemon's own. A port bound to nothing would hand the service no store at all,
// and the service would read from nothing.
func TestTheStatePortsReachTheKernelStore(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appstate.PortsFromKernel(k)

	if ports.Store() == nil {
		t.Fatal("the state port hands over no store — the daemon holds one")
	}
	if ports.Store() != k.State() {
		t.Fatal("the state port hands over a different store than the kernel holds")
	}
}

// A host with no kernel holds no state store.
//
// The port answers for that case with nothing at all: there is no store to
// read from, and the surfaces that read it refuse instead of reading from
// nothing.
func TestTheStatePortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := appstate.PortsFromKernel(nil)

	if ports.Store() != nil {
		t.Fatal("a host with no kernel hands over a state store")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheStatePortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(appstate.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the state port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the state port has no methods — the family cannot answer anything")
	}
}
