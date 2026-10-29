// SPDX-License-Identifier: MIT
package seats_test

import (
	"path/filepath"
	"reflect"
	"testing"

	appseats "github.com/agezt/agezt/kernel/app/seats"
	"github.com/agezt/agezt/kernel/modules/work/seat"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The seats family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins the FILLED half: a daemon that was given a seat catalog hands
// exactly that catalog over. A port bound to nothing would hand the service no
// store at all, and the service would report that the catalog is not
// available — so a port that always answers "no catalog" passes an EMPTY
// daemon's test and is wrong.
func TestTheSeatPortsReachTheKernelStore(t *testing.T) {
	base := t.TempDir()
	catalog, err := seat.OpenStore(filepath.Join(base, "seats"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: base, Provider: mock.New(), Seats: catalog})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appseats.PortsFromKernel(k)

	if ports.Store() == nil {
		t.Fatal("the seat port hands over no store — the catalog never reached the family")
	}
	if ports.Store() != k.Seats() {
		t.Fatal("the seat port hands over a different store than the kernel holds")
	}
}

// And the EMPTY half: a daemon that was given no seat catalog holds none, and
// the port says so instead of inventing one.
func TestTheSeatPortsHoldNoStoreWhenTheDaemonHasNoCatalog(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appseats.PortsFromKernel(k)

	if ports.Store() != nil {
		t.Fatal("a daemon with no seat catalog hands over a store")
	}
}

// A host with no kernel holds no seat catalog.
//
// The port answers for that case with nothing at all: there is no catalog to
// read from, and the surfaces that read it refuse instead of reading from
// nothing.
func TestTheSeatPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := appseats.PortsFromKernel(nil)

	if ports.Store() != nil {
		t.Fatal("a host with no kernel hands over a seat store")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheSeatPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(appseats.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the seat port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the seat port has no methods — the family cannot answer anything")
	}
}
