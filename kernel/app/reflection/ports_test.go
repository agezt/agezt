// SPDX-License-Identifier: MIT

package reflection

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The reflection family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins the FILLED half: the engine the port hands over is the
// daemon's own. The reflection engine is not a daemon's file — the composition
// root always builds one — so the daemon that WAS given a kernel always holds
// an engine, and that is the half a port that always answers "nothing" would
// fail.
func TestTheReflectionPortsReachTheKernelEngine(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	if ports.Engine() == nil {
		t.Fatal("the reflection port hands over no engine — the engine never reached the family")
	}
	if ports.Engine() != k.Reflect() {
		t.Fatal("the reflection port hands over a different engine than the kernel holds")
	}
}

// A host with no kernel holds no engine.
//
// The port answers for that case with nothing at all: there is no engine to
// ask, and the surfaces that read it refuse instead of reading from nothing.
func TestTheReflectionPortsSurviveAHostWithNoKernel(t *testing.T) {
	if PortsFromKernel(nil).Engine() != nil {
		t.Fatal("a host with no kernel hands over a reflection engine")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheReflectionPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the reflection port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the reflection port has no methods — the family cannot answer anything")
	}
}
