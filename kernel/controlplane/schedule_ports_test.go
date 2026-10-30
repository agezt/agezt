// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"github.com/agezt/agezt/kernel/app/binding"
	"path/filepath"
	"testing"
	"time"

	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The host does not bind the schedule application's ports — the APPLICATION
// fills them from the kernel handle. What the host keeps is the binding:
// which schedule library answers the daemon's schedule questions.
//
// The test pins that binding: the schedules the kernel holds are the schedules
// the adapter's schedule service serves, and they are the schedules the roster
// teardown's schedule port sees. A binding that answered from another library
// would leave the operator managing schedules the daemon never fires.
func TestTheAdapterBindsTheSchedulePortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	store, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("cadence.OpenStore: %v", err)
	}
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New(), Schedules: store}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	entry, err := k.Schedules().Add("check disks", time.Hour, "", "operator", time.Now())
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	s := NewServer(binding.ScopeFromKernel(k, dir))

	// The daemon's schedule service lists the kernel's schedules.
	list, err := s.scheduleReads().List(context.Background(), appschedule.ListInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Count != 1 {
		t.Fatalf("List = %+v, want the schedule the kernel holds", list)
	}

	// And the roster teardown's schedule port — bound from the application's
	// port, not from the handle — sees the same library.
	teardown := s.teardownPorts()
	if entries := teardown.Schedules(); len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("the teardown's schedule port sees %+v, want the schedule the kernel holds", entries)
	}

	// The teardown pauses through the application's port, and the pause lands.
	paused, err := teardown.SetScheduleEnabled(entry.ID, false)
	if err != nil {
		t.Fatalf("SetScheduleEnabled: %v", err)
	}
	if !paused {
		t.Error("SetScheduleEnabled = false, want the pause to land")
	}
	if held := k.Schedules().List(); len(held) != 1 || held[0].Enabled {
		t.Fatalf("the kernel holds %+v, want the paused schedule", held)
	}

	// And it removes through the same port.
	removed, err := teardown.RemoveSchedule(entry.ID)
	if err != nil {
		t.Fatalf("RemoveSchedule: %v", err)
	}
	if !removed {
		t.Error("RemoveSchedule = false, want the schedule that was there to be removed")
	}
	if held := k.Schedules().List(); len(held) != 0 {
		t.Errorf("the kernel holds %+v, want none after the removal", held)
	}
}
