// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"path/filepath"
	"testing"

	appstanding "github.com/agezt/agezt/kernel/app/standing"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the standing application's ports — the APPLICATION
// fills them from the kernel handle. What the host keeps is the binding:
// which standing library answers the daemon's standing questions.
//
// The test pins that binding: the orders the kernel holds are the orders the
// adapter's standing service serves, and they are the orders the roster
// teardown's standing port sees. A binding that answered from another library
// would leave the operator managing orders the daemon never fires.
func TestTheAdapterBindsTheStandingPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	store, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("standing.Open: %v", err)
	}
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New(), Standing: store}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	s := NewServer(binding.ScopeFromKernel(k, dir))
	svc := s.standingService()
	ctx := context.Background()

	added, err := svc.Add(ctx, appstanding.AddInput{Order: standingapi.Order{
		Name:     "watch",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "x.y"}},
	}})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The kernel holds the order the daemon's service added.
	held := k.Standing().List()
	if len(held) != 1 || held[0].ID != added.Order.ID {
		t.Fatalf("the kernel holds %+v, want the order the daemon's service added", held)
	}

	// The service lists it back.
	list, err := svc.List(ctx, appstanding.ListInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Count != 1 || list.Orders[0].ID != added.Order.ID {
		t.Fatalf("List = %+v, want the order the kernel holds", list)
	}

	// And the roster teardown's standing port — bound from the application's
	// port, not from the handle — sees the same library.
	teardown := s.teardownPorts()
	if orders := teardown.Standing(); len(orders) != 1 || orders[0].ID != added.Order.ID {
		t.Fatalf("the teardown's standing port sees %+v, want the order the kernel holds", orders)
	}

	// The teardown pauses through the application's port, and the pause lands.
	if err := teardown.SetStandingEnabled(added.Order.ID, false); err != nil {
		t.Fatalf("SetStandingEnabled: %v", err)
	}
	if held := k.Standing().List(); len(held) != 1 || held[0].Enabled {
		t.Fatalf("the kernel holds %+v, want the paused order", held)
	}

	// And it removes through the same port.
	removed, err := teardown.RemoveStanding(added.Order.ID)
	if err != nil {
		t.Fatalf("RemoveStanding: %v", err)
	}
	if !removed {
		t.Error("RemoveStanding = false, want the order that was there to be removed")
	}
	if held := k.Standing().List(); len(held) != 0 {
		t.Errorf("the kernel holds %+v, want none after the removal", held)
	}
}
