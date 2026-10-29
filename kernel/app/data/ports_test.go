// SPDX-License-Identifier: MIT

package data

import (
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/modules/artifacts/datalake"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The data family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins the FILLED half: a daemon that was given a lake hands exactly
// that lake over. A port bound to nothing would hand the service no lake at
// all, and every operation would report that the lake is unavailable — so a
// port that always answers "no lake" passes an EMPTY daemon's test and is
// wrong.
func TestTheDataPortsReachTheKernelLake(t *testing.T) {
	lake, err := datalake.Open(t.TempDir(), func() int64 { return time.Now().UnixMilli() })
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New(), Lake: lake})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	if ports.Lake() == nil {
		t.Fatal("the data port hands over no lake — the lake never reached the family")
	}
	if ports.Lake() != k.DataLake() {
		t.Fatal("the data port hands over a different lake than the kernel holds")
	}
}

// And the EMPTY half: a daemon that was given no lake holds none, and the port
// says so instead of inventing one.
func TestTheDataPortsHoldNoLakeWhenTheDaemonHasNoLake(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	if got := PortsFromKernel(k).Lake(); got != nil {
		t.Fatal("a daemon with no lake hands over a lake")
	}
}

// A host with no kernel holds no lake.
//
// The port answers for that case with nothing at all: there is no lake to read
// from, and the surfaces that read it refuse instead of reading from nothing.
func TestTheDataPortsSurviveAHostWithNoKernel(t *testing.T) {
	if PortsFromKernel(nil).Lake() != nil {
		t.Fatal("a host with no kernel hands over a lake")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheDataPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the data port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the data port has no methods — the family cannot answer anything")
	}
}
