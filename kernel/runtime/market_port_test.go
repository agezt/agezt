// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// stubMarketPort is a marketplace port that answers nothing: the kernel never
// drives the marketplace itself (the daemon's tool does), so the port only has
// to be a port.
type stubMarketPort struct{}

func (stubMarketPort) List(string) ([]marketapi.Listing, error) { return nil, nil }
func (stubMarketPort) Show(string, string) (marketapi.Pack, marketapi.InstalledPack, bool, error) {
	return marketapi.Pack{}, marketapi.InstalledPack{}, false, nil
}
func (stubMarketPort) Sources() ([]marketapi.Source, error) { return nil, nil }
func (stubMarketPort) InstallContext(context.Context, string, string, string, string, func(marketapi.Event) error) (marketapi.InstalledPack, error) {
	return marketapi.InstalledPack{}, nil
}
func (stubMarketPort) UninstallContext(context.Context, string, string, func(marketapi.Event) error) error {
	return nil
}
func (stubMarketPort) AddSource(string, string, string) (marketapi.Source, error) {
	return marketapi.Source{}, nil
}
func (stubMarketPort) RemoveSource(string) (bool, error) { return false, nil }
func (stubMarketPort) Sync(context.Context, string) ([]marketapi.SyncResult, error) {
	return nil, nil
}

// TestTheKernelHoldsTheMarketPortNotTheManager: the kernel holds the
// CONTRACT's marketplace port (W3.46), and it holds NOTHING until the daemon
// wires one.
//
// The nil half matters: the kernel used to reach into the market module and
// build the manager itself, so the field was never nil and the code never had
// to say what happens when it is. Now a host may open a kernel without a
// marketplace — and the answer is a nil port, not a panic.
func TestTheKernelHoldsTheMarketPortNotTheManager(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Market() != nil {
		t.Fatal("the kernel must hold no marketplace until the daemon wires one")
	}

	k.SetMarket(stubMarketPort{})
	if k.Market() == nil {
		t.Fatal("the daemon's marketplace port did not reach the kernel")
	}
}
