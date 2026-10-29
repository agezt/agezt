// SPDX-License-Identifier: MIT
package market_test

import (
	"reflect"
	"testing"

	appmarket "github.com/agezt/agezt/kernel/app/market"
	core "github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// marketPortLibrary is a library that serves nothing: the port test is about
// the WIRING, not about what the marketplace happens to carry.
type marketPortLibrary struct{}

func (marketPortLibrary) Marketplaces() []core.Marketplace { return nil }

func (marketPortLibrary) ResolvePack(_, _, _ string) (core.Pack, error) { return core.Pack{}, nil }

// The marketplace family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins what that wiring buys: the manager the port hands over is the
// daemon's own manager. A port bound to nothing would hand the reads and the
// writes no manager at all, and every surface would refuse.
func TestTheMarketPortsReachTheKernelManager(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	manager := core.NewManager(core.Config{Library: marketPortLibrary{}, Store: core.NewStore(t.TempDir())})
	k.SetMarket(manager)

	ports := appmarket.PortsFromKernel(k)

	if ports.Manager() == nil {
		t.Fatal("the marketplace port hands over no manager — the daemon holds one")
	}
	if ports.Manager() != k.Market() {
		t.Fatal("the marketplace port hands over a different manager than the kernel holds")
	}
}

// A host with no kernel holds no marketplace.
//
// The port answers for that case with nothing at all: there is no manager to
// read from, and the surfaces that read it refuse instead of reading from
// nothing.
func TestTheMarketPortsSurviveAHostWithNoKernel(t *testing.T) {
	ports := appmarket.PortsFromKernel(nil)

	if ports.Manager() != nil {
		t.Fatal("a host with no kernel hands over a marketplace manager")
	}
}

// A daemon that holds a kernel but no marketplace holds no manager either: the
// port asks the kernel, and the kernel says there is none.
func TestTheMarketPortsSurviveAHostWithNoManager(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appmarket.PortsFromKernel(k)

	if ports.Manager() != nil {
		t.Fatal("a daemon with no marketplace hands over a marketplace manager")
	}
}

// The port asks the kernel WHEN IT IS ASKED, not when it was filled.
//
// The marketplace is the daemon's own file and the daemon may be given it after
// the ports are filled — a port that kept the answer it was handed at fill time
// would report no marketplace for a daemon that has one.
func TestTheMarketPortsAskTheKernelWhenTheyAreAsked(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := appmarket.PortsFromKernel(k)
	if ports.Manager() != nil {
		t.Fatal("the port hands over a manager before the daemon holds one")
	}

	k.SetMarket(core.NewManager(core.Config{Library: marketPortLibrary{}, Store: core.NewStore(t.TempDir())}))

	if ports.Manager() == nil {
		t.Fatal("the port keeps the answer it was given when it was filled — it must ask the kernel when it is asked")
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheMarketPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(appmarket.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the marketplace port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the marketplace port has no methods — the family cannot answer anything")
	}
}
