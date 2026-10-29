// SPDX-License-Identifier: MIT

package guardians

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The guardian seeder's ports are filled by the APPLICATION from the kernel
// handle — the composition root hands over the ports, never the handle.
//
// The test pins the FILLED half: the port really READS and really WRITES the
// daemon's three libraries. A port bound to nothing would answer "no daemon"
// for every question, and the seeder would plant nothing while looking
// perfectly wired.
func TestTheGuardianPortsReachTheDaemon(t *testing.T) {
	dir := t.TempDir()
	standingStore, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("standing.Open: %v", err)
	}
	scheduleStore, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("cadence.OpenStore: %v", err)
	}
	k, err := runtime.Open(withRoster(t, runtime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		Standing:  standingStore,
		Schedules: scheduleStore,
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the seeder would plant nothing")
	}

	// The roster half.
	added, err := ports.AddAgent(agentapi.Profile{Slug: "guardian-health", System: true})
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	if added.Slug != "guardian-health" {
		t.Fatalf("the added agent = %+v", added)
	}
	if len(ports.Agents()) != 1 {
		t.Fatalf("the live roster holds %d agents, want 1", len(ports.Agents()))
	}
	_, found, err := ports.EditAgent("guardian-health", func(p *agentapi.Profile) {
		p.Description = "edited through the port"
	})
	if err != nil {
		t.Fatalf("EditAgent: %v", err)
	}
	if !found {
		t.Fatal("the port must find the agent the roster holds")
	}
	stored, ok := k.Roster().Get("guardian-health")
	if !ok || stored.Description != "edited through the port" {
		t.Fatalf("the edit was not stored in the roster: %+v ok=%v", stored, ok)
	}

	// The standing half.
	order, err := ports.AddOrder(standingapi.Order{Name: "watch the repo", Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}}})
	if err != nil {
		t.Fatalf("AddOrder: %v", err)
	}
	if order.ID == "" {
		t.Fatal("the standing library gave the order no id")
	}
	if len(ports.Orders()) != 1 {
		t.Fatalf("the live standing library holds %d orders, want 1", len(ports.Orders()))
	}
	if _, _, err := ports.EditOrder(order.ID, func(o *standingapi.Order) { o.Name = "edited" }); err != nil {
		t.Fatalf("EditOrder: %v", err)
	}
	if _, err := ports.SetOrderEnabled(order.ID, false); err != nil {
		t.Fatalf("SetOrderEnabled: %v", err)
	}

	// The schedule half.
	entry, err := ports.AddInterval("cycle repo-watch", 90*time.Second, "mock-model", "")
	if err != nil {
		t.Fatalf("AddInterval: %v", err)
	}
	if entry.Intent != "cycle repo-watch" {
		t.Fatalf("the schedule = %+v", entry)
	}
	if len(ports.Entries()) != 1 {
		t.Fatalf("the daemon holds %d schedules, want 1", len(ports.Entries()))
	}
	if _, err := ports.Reschedule(entry.ID, "interval", 120*time.Second, 0, 0); err != nil {
		t.Fatalf("Reschedule: %v", err)
	}
	if _, err := ports.AddDaily("cycle repo-daily", 30, "mock-model", ""); err != nil {
		t.Fatalf("AddDaily: %v", err)
	}
}

// A host with no daemon holds no daemon to seed — and each of the daemon's
// three libraries has its OWN refusal.
//
// The refusals are written ONCE, in the ports: a daemon with no standing
// library is not a daemon with no schedule library, and the seeder must be
// able to tell them apart.
func TestTheGuardianPortsRefuseForAHostWithNoDaemon(t *testing.T) {
	ports := PortsFromKernel(nil)

	if ports.Ready() {
		t.Fatal("a host with no daemon reports a daemon")
	}
	if got := ports.Agents(); got != nil {
		t.Fatalf("a host with no daemon holds agents: %v", got)
	}
	if _, err := ports.AddAgent(agentapi.Profile{Slug: "nobody"}); !errors.Is(err, errNoDaemon) {
		t.Errorf("the roster write answered %v, want %v", err, errNoDaemon)
	}
	if _, _, err := ports.EditAgent("nobody", func(*agentapi.Profile) {}); !errors.Is(err, errNoDaemon) {
		t.Errorf("the roster edit answered %v, want %v", err, errNoDaemon)
	}
	if got := ports.Orders(); got != nil {
		t.Fatalf("a host with no daemon holds standing orders: %v", got)
	}
	if _, err := ports.AddOrder(standingapi.Order{ID: "order-1", Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}}}); !errors.Is(err, errNoStandingLibrary) {
		t.Errorf("the standing write answered %v, want %v", err, errNoStandingLibrary)
	}
	if _, _, err := ports.EditOrder("order-1", func(*standingapi.Order) {}); !errors.Is(err, errNoStandingLibrary) {
		t.Errorf("the standing edit answered %v, want %v", err, errNoStandingLibrary)
	}
	if _, err := ports.SetOrderEnabled("order-1", false); !errors.Is(err, errNoStandingLibrary) {
		t.Errorf("the standing pause answered %v, want %v", err, errNoStandingLibrary)
	}
	if got := ports.Entries(); got != nil {
		t.Fatalf("a host with no daemon holds schedules: %v", got)
	}
	if _, err := ports.Reschedule("id", "interval", time.Minute, 0, 0); !errors.Is(err, errNoScheduleLibrary) {
		t.Errorf("the reschedule answered %v, want %v", err, errNoScheduleLibrary)
	}
	if _, err := ports.AddInterval("cycle", time.Minute, "mock-model", ""); !errors.Is(err, errNoScheduleLibrary) {
		t.Errorf("the interval schedule answered %v, want %v", err, errNoScheduleLibrary)
	}
	if _, err := ports.AddDaily("cycle", 30, "mock-model", ""); !errors.Is(err, errNoScheduleLibrary) {
		t.Errorf("the daily schedule answered %v, want %v", err, errNoScheduleLibrary)
	}

	if got := NewKernelHost(ports); got != nil {
		t.Fatalf("a host with no daemon hands the seeder a port: %v", got)
	}
}

// A daemon with NO standing library and NO schedule library is still a
// daemon: the roster half works, and each library half refuses with its OWN
// refusal.
//
// This is the third half the programme keeps asking for — the daemon's own
// files may be absent, and "absent" is not "empty": the standing writes
// refuse, the schedule writes refuse, and the reads answer nothing.
func TestTheGuardianPortsRefuseForADaemonWithoutItsLibraries(t *testing.T) {
	k, err := runtime.Open(withRoster(t, runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the daemon is there, its libraries are not")
	}

	// The roster half works.
	if _, err := ports.AddAgent(agentapi.Profile{Slug: "guardian-health", System: true}); err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	if len(ports.Agents()) != 1 {
		t.Fatalf("the live roster holds %d agents, want 1", len(ports.Agents()))
	}

	// The standing half refuses.
	if got := ports.Orders(); got != nil {
		t.Fatalf("a daemon with no standing library holds orders: %v", got)
	}
	if _, err := ports.AddOrder(standingapi.Order{Name: "watch", Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}}}); !errors.Is(err, errNoStandingLibrary) {
		t.Errorf("the standing write answered %v, want %v", err, errNoStandingLibrary)
	}
	if _, _, err := ports.EditOrder("order-1", func(*standingapi.Order) {}); !errors.Is(err, errNoStandingLibrary) {
		t.Errorf("the standing edit answered %v, want %v", err, errNoStandingLibrary)
	}
	if _, err := ports.SetOrderEnabled("order-1", false); !errors.Is(err, errNoStandingLibrary) {
		t.Errorf("the standing pause answered %v, want %v", err, errNoStandingLibrary)
	}

	// The schedule half refuses.
	if got := ports.Entries(); got != nil {
		t.Fatalf("a daemon with no schedule library holds schedules: %v", got)
	}
	if _, err := ports.Reschedule("id", "interval", time.Minute, 0, 0); !errors.Is(err, errNoScheduleLibrary) {
		t.Errorf("the reschedule answered %v, want %v", err, errNoScheduleLibrary)
	}
	if _, err := ports.AddInterval("cycle", time.Minute, "mock-model", ""); !errors.Is(err, errNoScheduleLibrary) {
		t.Errorf("the interval schedule answered %v, want %v", err, errNoScheduleLibrary)
	}
	if _, err := ports.AddDaily("cycle", 30, "mock-model", ""); !errors.Is(err, errNoScheduleLibrary) {
		t.Errorf("the daily schedule answered %v, want %v", err, errNoScheduleLibrary)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheGuardianPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	if port.NumMethod() == 0 {
		t.Fatal("the guardian port has no methods — the seeder cannot ask anything")
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the guardian port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	for i := 0; i < port.NumField(); i++ {
		name := port.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the guardian port field %s is the kernel's own verb — a port field may not carry the name of the verb it stands for", name)
		}
	}
}
