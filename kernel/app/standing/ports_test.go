// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The standing application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The test pins what that wiring buys: the standing orders the kernel holds
// are the orders the application reads, edits, pauses and removes. A port
// bound to another library would leave the operator managing orders the daemon
// does not fire.
func TestTheStandingPortsReachTheKernelOrders(t *testing.T) {
	dir := t.TempDir()
	store, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("standing.Open: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Standing: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	svc := New(ports.Reader, ports.Writer, Host{})
	ctx := context.Background()

	added, err := svc.Add(ctx, AddInput{Order: standingapi.Order{
		Name:     "watch",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "x.y"}},
	}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.Order.ID == "" {
		t.Fatal("Add returned an order with no id")
	}

	// The ports' own listing sees the order the application added.
	if orders := ports.Orders(); len(orders) != 1 || orders[0].ID != added.Order.ID {
		t.Fatalf("Orders = %+v, want the one order the application added", orders)
	}

	// The service reads the same library.
	list, err := svc.List(ctx, ListInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Count != 1 {
		t.Fatalf("List count = %d, want 1", list.Count)
	}

	// Pausing one goes through the port and lands.
	if err := ports.SetEnabled(added.Order.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	orders := ports.Orders()
	if len(orders) != 1 || orders[0].Enabled {
		t.Fatalf("Orders = %+v, want the paused order", orders)
	}

	// Removing one goes through the port and lands.
	if err := ports.SetEnabled(added.Order.ID, true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	removed, err := ports.Remove(added.Order.ID)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Error("Remove = false, want the order that was there to be removed")
	}
	if orders := ports.Orders(); len(orders) != 0 {
		t.Errorf("Orders = %+v, want none after the removal", orders)
	}
}

// A daemon with NO standing library holds a nil port (W3.50): the
// application sees no standing orders rather than calling into nothing.
//
// The port carries that discipline now: the adapter used to spell the
// nil-check itself, once per surface, and a surface that forgot it would have
// called into nothing.
func TestTheStandingPortsSurviveADaemonWithNoStandingLibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Reader != nil {
		t.Error("the reader port is not nil — the daemon holds no standing library")
	}
	if orders := ports.Orders(); orders != nil {
		t.Errorf("Orders = %+v, want none from a daemon with no standing library", orders)
	}
}
