// SPDX-License-Identifier: MIT

package overseer

import (
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The overseer's ports are filled by the APPLICATION from the kernel handle —
// the composition root hands over the ports, never the handle.
//
// The test pins the FILLED half: the port really READS and really WRITES the
// daemon's fleet, and it really reaches the daemon's own gate. A port bound to
// nothing would answer "no daemon" for every question, and the supervisor
// would steer nothing while looking perfectly wired.
func TestTheOverseerPortsReachTheDaemon(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the supervisor would steer nothing")
	}

	// The fleet half.
	added, err := ports.CreateProfile(agentapi.Profile{Slug: "scout", Name: "Scout", Enabled: true})
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if added.Slug != "scout" {
		t.Fatalf("the added agent = %+v", added)
	}
	if got := ports.Profiles(); len(got) != 1 || got[0].Slug != "scout" {
		t.Fatalf("the live roster = %+v", got)
	}
	stored, ok := ports.Profile("scout")
	if !ok || stored.Name != "Scout" {
		t.Fatalf("the port did not find the agent the roster holds: %+v ok=%v", stored, ok)
	}
	if _, found, err := ports.EditProfile("scout", func(p *agentapi.Profile) { p.Name = "Renamed" }); err != nil || !found {
		t.Fatalf("EditProfile: %v found=%v", err, found)
	}
	if got, _ := ports.Profile("scout"); got.Name != "Renamed" {
		t.Fatalf("the edit was not stored in the roster: %+v", got)
	}
	if _, err := ports.EnableAgent("scout", false); err != nil {
		t.Fatalf("EnableAgent: %v", err)
	}
	if got, _ := ports.Profile("scout"); got.Enabled {
		t.Error("the port did not pause the agent")
	}
	if _, err := ports.RetireAgent("scout", true, "the fleet is retired"); err != nil {
		t.Fatalf("RetireAgent: %v", err)
	}
	if got, _ := ports.Profile("scout"); !got.Retired {
		t.Error("the port did not retire the agent")
	}
	// The impact is what DEPENDS on the agent: an agent nothing depends on
	// has no impact, and the port answers as the daemon answers it.
	if got, want := ports.ImpactOf("scout"), k.AgentImpact("scout"); len(got) != len(want) {
		t.Errorf("the port's impact = %v, want the daemon's %v", got, want)
	}
	if ok, err := ports.DeleteProfile("scout"); err != nil || !ok {
		t.Fatalf("DeleteProfile: %v ok=%v", err, ok)
	}
	if got := ports.Profiles(); len(got) != 0 {
		t.Errorf("the port did not delete the agent: %+v", got)
	}

	// The correlation and the provider half.
	if got := ports.NewCorrelationID(); got == "" {
		t.Error("the port minted no correlation — a wake would run under nothing")
	}
	if ports.LiveProvider() == nil {
		t.Error("the port holds no live provider — the routing chains would be unreachable")
	}

	// The daemon's own gate half.
	if ports.Stopped() {
		t.Error("a fresh daemon is halted")
	}
	ports.HaltDaemon("the port's own test")
	if !ports.Stopped() {
		t.Error("the port did not halt the daemon")
	}
	ports.ResumeDaemon("the port's own test")
	if ports.Stopped() {
		t.Error("the port did not resume the daemon")
	}

	// The runs half: the two verbs that belong to nobody else.
	if got := ports.RunningIDs(); len(got) != 0 {
		t.Errorf("a fresh daemon runs %v", got)
	}
	if ports.StopRun("no-such-correlation") {
		t.Error("the port cancelled a run that does not exist")
	}
}

// A host with no daemon holds no daemon to supervise.
//
// The READS answer with nothing, and the WRITES refuse — a supervisor that
// answered "edited" about a daemon that does not exist would be lying, and
// the refusal is written ONCE, in the port.
func TestTheOverseerPortsRefuseForAHostWithNoDaemon(t *testing.T) {
	ports := PortsFromKernel(nil)

	if ports.Ready() {
		t.Fatal("a host with no daemon reports a daemon")
	}
	if got := ports.Profiles(); got != nil {
		t.Errorf("a host with no daemon holds a fleet: %v", got)
	}
	if _, ok := ports.Profile("scout"); ok {
		t.Error("a host with no daemon holds an agent")
	}
	if got := ports.ImpactOf("scout"); got != nil {
		t.Errorf("a host with no daemon has an impact: %v", got)
	}
	if got := ports.RunningIDs(); got != nil {
		t.Errorf("a host with no daemon runs %v", got)
	}
	if ports.Stopped() {
		t.Error("a host with no daemon is halted")
	}
	if got := ports.StopRun("corr"); got {
		t.Error("a host with no daemon cancelled a run")
	}
	if got := ports.NewCorrelationID(); got != "" {
		t.Errorf("a host with no daemon minted a correlation: %q", got)
	}
	if got := ports.FleetScan(0); !got.Empty() {
		t.Errorf("a host with no daemon answered a fleet scan: %+v", got)
	}
	if got := ports.LiveProvider(); got != nil {
		t.Errorf("a host with no daemon holds a live provider: %v", got)
	}

	// The WRITES refuse, and they refuse with the ONE refusal.
	if _, err := ports.CreateProfile(agentapi.Profile{Slug: "scout"}); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the agent write answered %v, want %v", err, ErrNoDaemon)
	}
	if _, err := ports.DeleteProfile("scout"); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the agent delete answered %v, want %v", err, ErrNoDaemon)
	}
	if _, err := ports.EnableAgent("scout", false); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the agent pause answered %v, want %v", err, ErrNoDaemon)
	}
	if _, err := ports.RetireAgent("scout", true, "because"); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the agent retirement answered %v, want %v", err, ErrNoDaemon)
	}
	if _, _, err := ports.EditProfile("scout", func(*agentapi.Profile) {}); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the agent edit answered %v, want %v", err, ErrNoDaemon)
	}
	if _, err := ports.Launch(t.Context(), "corr", "do it", nil); !errors.Is(err, ErrNoDaemon) {
		t.Errorf("the run answered %v, want %v", err, ErrNoDaemon)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheOverseerPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	if port.NumMethod() == 0 {
		t.Fatal("the overseer port has no methods — the supervisor cannot ask anything")
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the overseer port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	for i := 0; i < port.NumField(); i++ {
		name := port.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the overseer port field %s is the kernel's own verb — a port field may not carry the name of the verb it stands for", name)
		}
	}
}
