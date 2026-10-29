// SPDX-License-Identifier: MIT

package introspect

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/types"
	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The introspector's ports are filled by the APPLICATION from the kernel
// handle — the composition root hands over the ports, never the handle.
//
// The test pins the FILLED half: the port really READS the daemon's own
// libraries. A port bound to nothing would answer "no daemon" for every
// question, and the gather would report an empty daemon while looking
// perfectly wired.
func TestTheIntrospectPortsReachTheDaemon(t *testing.T) {
	dir := t.TempDir()
	sched, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	standingStore, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	k, err := kernelruntime.Open(withMemory(t, withWorld(t, kernelruntime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		Tools:     map[string]toolapi.Tool{"zebra": nil, "alpha": nil},
		Schedules: sched,
		Standing:  standingStore,
	})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := sched.Add("morning brief", time.Hour, "mock-model", "operator", time.Now()); err != nil {
		t.Fatalf("Add schedule: %v", err)
	}
	if _, err := standingStore.Add(standingapi.Order{
		Name:     "watch the deploys",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	}); err != nil {
		t.Fatalf("Add standing order: %v", err)
	}
	// The world model and the memory hold something, so the port's counts for
	// them are a MEASUREMENT and not a pair of zeros that agree with anything.
	if _, _, err := k.World().Upsert("corr", worldmodelapi.UpsertSpec{
		Kind: worldmodelapi.KindProject, Name: "the repo",
	}); err != nil {
		t.Fatalf("Upsert world entity: %v", err)
	}
	if _, _, err := k.Memory().Remember("seed", memory.RememberSpec{
		Subject: "the run", Content: "a fact the daemon holds",
	}); err != nil {
		t.Fatalf("remember: %v", err)
	}

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the gather would report an empty daemon")
	}

	// The journal is the daemon's own.
	head, _ := ports.JournalHead()
	wantHead, _ := k.Journal().Head()
	if head != wantHead {
		t.Errorf("the port's journal head = %d, want the daemon's %d", head, wantHead)
	}

	// The tool names are the daemon's own.
	names := ports.ToolNames()
	if len(names) != 2 {
		t.Fatalf("the port listed %d tool names, want 2: %v", len(names), names)
	}

	// The schedules and the standing orders are the daemon's own.
	if got := ports.ScheduleEntries(); len(got) != 1 || got[0].Intent != "morning brief" {
		t.Fatalf("the port listed the wrong schedules: %+v", got)
	}
	if got := ports.StandingOrders(); len(got) != 1 || got[0].Name != "watch the deploys" {
		t.Fatalf("the port listed the wrong standing orders: %+v", got)
	}

	// The daemon's own state answers as the daemon answers it.
	if got, want := ports.DefaultModel(), k.Model(); got != want {
		t.Errorf("the port's default model = %q, want the daemon's %q", got, want)
	}
	if got, want := ports.MemoryRecordCount(), k.MemoryCount(); got != want {
		t.Errorf("the port's memory records = %d, want the daemon's %d", got, want)
	}
	if got, want := ports.Delegation(), k.SubAgentLimits(); got != want {
		t.Errorf("the port's delegation limits = %+v, want the daemon's %+v", got, want)
	}
	if got, want := ports.ActiveRunCount(), k.ActiveRuns(); got != want {
		t.Errorf("the port's active runs = %d, want the daemon's %d", got, want)
	}
	if got, want := ports.Stopped(), k.IsHalted(); got != want {
		t.Errorf("the port's stopped = %v, want the daemon's %v", got, want)
	}
	if got, want := ports.WorldEntities(), k.World().Count(); got != want || want == 0 {
		t.Errorf("the port's world entities = %d, want the daemon's %d", got, want)
	}
	if got, want := ports.MemoryRecordCount(), k.MemoryCount(); got != want || want == 0 {
		t.Errorf("the port's memory records = %d, want the daemon's %d", got, want)
	}
	if got, want := ports.PendingApprovals(), k.Approvals().PendingCount(); got != want {
		t.Errorf("the port's pending approvals = %d, want the daemon's %d", got, want)
	}

	// The reaper's scan is asked of the reaper family: a daemon that was given
	// no reaper module holds no scanner, and the port answers an empty report.
	if got := ports.FleetScan(time.Now().Add(-time.Hour).UnixMilli()); !got.Empty() {
		t.Errorf("a daemon with no reaper module answered a scan: %+v", got)
	}
}

// A host with no daemon holds no daemon to report on — and the gather answers
// with NOTHING rather than with a lie.
//
// The uptime is the sharp end of it: measured from the zero time, a host with
// no daemon would claim to have been running since the year one.
func TestTheIntrospectPortsAnswerAHostWithNoDaemon(t *testing.T) {
	ports := PortsFromKernel(nil)

	if ports.Ready() {
		t.Fatal("a host with no daemon reports a daemon")
	}
	if head, hash := ports.JournalHead(); head != 0 || hash != "" {
		t.Errorf("a host with no daemon has a journal head: %d %q", head, hash)
	}
	if got, err := ports.JournalTail(10); got != nil || err != nil {
		t.Errorf("a host with no daemon has a journal tail: %v (%v)", got, err)
	}
	if got := ports.Started(); !got.IsZero() {
		t.Errorf("a host with no daemon started at %v", got)
	}
	if got := ports.Delegation(); got != (types.SubAgentLimits{}) {
		t.Errorf("a host with no daemon has delegation limits: %+v", got)
	}
	if got := ports.ScheduleEntries(); got != nil {
		t.Errorf("a host with no daemon holds schedules: %v", got)
	}
	if got := ports.PendingApprovals(); got != 0 {
		t.Errorf("a host with no daemon waits on %d approvals", got)
	}
	if got := ports.WorldEntities(); got != 0 {
		t.Errorf("a host with no daemon holds %d world entities", got)
	}
	if got := ports.ActiveSkills(); got != 0 {
		t.Errorf("a host with no daemon holds %d skills", got)
	}
	if got := ports.ToolNames(); got != nil {
		t.Errorf("a host with no daemon holds tools: %v", got)
	}
	if got := ports.DefaultModel(); got != "" {
		t.Errorf("a host with no daemon has a default model: %q", got)
	}
	if got := ports.Stopped(); got {
		t.Error("a host with no daemon is stopped")
	}
	if got := ports.ActiveRunCount(); got != 0 {
		t.Errorf("a host with no daemon runs %d runs", got)
	}
	if got := ports.MemoryRecordCount(); got != 0 {
		t.Errorf("a host with no daemon holds %d memory records", got)
	}
	if got := ports.FleetScan(0); !got.Empty() {
		t.Errorf("a host with no daemon answered a fleet scan: %+v", got)
	}
	if got := ports.StandingOrders(); got != nil {
		t.Errorf("a host with no daemon holds standing orders: %v", got)
	}

	// And the GATHER answers with an empty overview instead of panicking.
	ov := NewKernelSource(ports).Overview()
	if ov.UptimeSeconds != 0 {
		t.Errorf("a host with no daemon reported an uptime of %d seconds", ov.UptimeSeconds)
	}
	if len(ov.Tools) != 0 || ov.SchedulesTotal != 0 || ov.MemoryRecords != 0 {
		t.Errorf("a host with no daemon reported state: %+v", ov)
	}
}

// A daemon with NO world model, NO skill library, NO schedule library and NO
// standing library is still a daemon.
//
// This is the third half the programme keeps asking for: the daemon's own
// files may be absent, and "absent" is not "empty" — the reads answer zero
// and the gather does not call into nothing.
func TestTheIntrospectPortsAnswerADaemonWithoutItsLibraries(t *testing.T) {
	k, err := kernelruntime.Open(withMemory(t, withWorld(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"probe": nil},
	})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	ports := PortsFromKernel(k)
	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the daemon is there, its libraries are not")
	}

	if got := ports.ScheduleEntries(); got != nil {
		t.Errorf("a daemon with no schedule library holds schedules: %v", got)
	}
	if got := ports.StandingOrders(); got != nil {
		t.Errorf("a daemon with no standing library holds standing orders: %v", got)
	}
	if got := ports.WorldEntities(); got != 0 {
		t.Errorf("a daemon with no world model holds %d entities", got)
	}
	if got := ports.ActiveSkills(); got != 0 {
		t.Errorf("a daemon with no skill library holds %d skills", got)
	}

	// And the GATHER still answers the daemon it does have.
	ov := NewKernelSource(ports).Overview()
	if len(ov.Tools) != 1 || ov.Tools[0] != "probe" {
		t.Errorf("the overview lost the daemon's tools: %v", ov.Tools)
	}
	if ov.SchedulesTotal != 0 || ov.SchedulesEnabled != 0 {
		t.Errorf("the overview counted schedules the daemon does not have: %+v", ov)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheIntrospectPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&kernelruntime.Kernel{})
	if port.NumMethod() == 0 {
		t.Fatal("the introspect port has no methods — the gather cannot ask anything")
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the introspect port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	for i := 0; i < port.NumField(); i++ {
		name := port.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the introspect port field %s is the kernel's own verb — a port field may not carry the name of the verb it stands for", name)
		}
	}
}
