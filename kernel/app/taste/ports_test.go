// SPDX-License-Identifier: MIT
package taste_test

import (
	"path/filepath"
	"reflect"
	"testing"

	apptaste "github.com/agezt/agezt/kernel/app/taste"
	"github.com/agezt/agezt/kernel/modules/knowledge/taste"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// withTaste opens the curated exemplar library a daemon would open and hands
// the kernel the contract's store port.
func withTaste(t *testing.T, cfg runtime.Config) runtime.Config {
	t.Helper()
	store, err := taste.OpenStore(filepath.Join(cfg.BaseDir, "taste"))
	if err != nil {
		t.Fatalf("open the taste store: %v", err)
	}
	cfg.TasteStore = store
	return cfg
}

// The taste family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the library the port hands over is the
// daemon's own. A port bound to nothing would hand the service no store at all,
// and the service would report an empty library for a daemon that has one.
func TestTheTastePortsReachTheKernelStore(t *testing.T) {
	k, err := runtime.Open(withTaste(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := apptaste.PortsFromKernel(k)

	if ports.Store() == nil {
		t.Fatal("the taste port hands over no store — the daemon holds one")
	}
	if ports.Store() != k.Taste() {
		t.Fatal("the taste port hands over a different store than the kernel holds")
	}
}

// A daemon that was given no taste library holds none, and the port says so
// instead of inventing one.
//
// The library is the DAEMON's file: the kernel holds the contract's port and
// the daemon opens the file, so a daemon that was given none holds a nil store
// — and the port's empty case is a real case, not a theoretical one.
func TestTheTastePortsSurviveAHostWithNoLibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := apptaste.PortsFromKernel(k)

	if ports.Store() != nil {
		t.Fatal("a daemon with no taste library hands over a store")
	}
}

// A host with no kernel holds no taste library.
//
// The port answers for that case with nothing at all: there is no library to
// read from, and the surfaces that read it report the empty library instead of
// reading from nothing.
func TestTheTastePortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := apptaste.PortsFromKernel(nil)

	if ports.Store() != nil {
		t.Fatal("a host with no kernel hands over a taste store")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheTastePortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(apptaste.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the taste port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the taste port has no methods — the family cannot answer anything")
	}
}
