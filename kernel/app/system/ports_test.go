// SPDX-License-Identifier: MIT

package system_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app/system"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// worldCount is the daemon's own count of world entities — zero for a daemon
// that holds no world model.
func worldCount(k *runtime.Kernel) int {
	graph := k.World()
	if graph == nil {
		return 0
	}
	return graph.Count()
}

// skillCount is the daemon's own count of skills — zero for a daemon that
// holds no skill library.
func skillCount(k *runtime.Kernel) int {
	curator := k.Forge()
	if curator == nil {
		return 0
	}
	return curator.Count()
}

// pendingCount is the daemon's own count of waiting approvals — zero for a
// daemon that holds no approval queue.
func pendingCount(k *runtime.Kernel) int {
	registry := k.Approvals()
	if registry == nil {
		return 0
	}
	return registry.PendingCount()
}

// The daemon overview's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins the FILLED half: the port really READS the daemon. A port
// bound to nothing would answer zeros for every question, and the overview
// would report a daemon with no tools, no memory and no world.
func TestTheDaemonOverviewPortsReachTheDaemon(t *testing.T) {
	k, err := runtime.Open(withMemory(t, withWorld(t, runtime.Config{
		BaseDir:                    t.TempDir(),
		Provider:                   mock.New(),
		Tools:                      map[string]toolapi.Tool{},
		SubAgentTool:               true,
		SubAgentMaxDepth:           2,
		SubAgentMaxFanout:          3,
		SubAgentMaxSpendMicrocents: 500,
		SubAgentMaxTotal:           7,
	})))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := system.PortsFromKernel(k)

	if !ports.Ready() {
		t.Fatal("the port reports no daemon — the ports were filled with nothing")
	}
	if ports.Started().IsZero() {
		t.Fatal("the port does not report when the daemon started")
	}
	if !ports.Started().Equal(k.StartTime()) {
		t.Fatalf("the port reports the daemon started at %v, want %v", ports.Started(), k.StartTime())
	}
	if ports.Events() == nil {
		t.Fatal("the port holds no journal — the overview reads the journal for its fallback counts")
	}
	wantSeq, _ := k.Journal().Head()
	gotSeq, _ := ports.Events().Head()
	if gotSeq != wantSeq {
		t.Fatalf("the port's journal is not the daemon's: head %d, want %d", gotSeq, wantSeq)
	}
	if got, want := ports.ToolCount(), len(k.Tools()); got != want {
		t.Errorf("the port reports %d tools, want %d", got, want)
	}
	if got, want := ports.MemoryRecordCount(), k.MemoryCount(); got != want {
		t.Errorf("the port reports %d memory records, want %d", got, want)
	}
	if got, want := ports.DefaultModel(), k.Model(); got != want {
		t.Errorf("the port reports the model %q, want %q", got, want)
	}
	if got, want := ports.Entities(), worldCount(k); got != want {
		t.Errorf("the port reports %d world entities, want %d", got, want)
	}
	if got, want := ports.Skills(), skillCount(k); got != want {
		t.Errorf("the port reports %d skills, want %d", got, want)
	}
	if got, want := ports.Delegation(), k.SubAgentLimits(); got != want {
		t.Errorf("the port reports the delegation ceilings %+v, want %+v", got, want)
	}
	if ports.Stopped() != k.IsHalted() {
		t.Errorf("the port reports stopped=%v, want %v", ports.Stopped(), k.IsHalted())
	}
	if got, want := ports.ActiveRunCount(), k.ActiveRuns(); got != want {
		t.Errorf("the port reports %d active runs, want %d", got, want)
	}
	if got, want := ports.Pending(), pendingCount(k); got != want {
		t.Errorf("the port reports %d pending approvals, want %d", got, want)
	}
	if ports.Gate() == nil {
		t.Fatal("the port holds no gate — the daemon's own halt and resume have nowhere to go")
	}
}

// A host with no daemon holds no daemon to report on.
//
// The port answers the empty ones — no journal, no entries, no skills — and
// the SURFACE REFUSES: a status full of zeros would read as a claim about a
// daemon that does not exist ("the daemon has no tools and no memory"), and
// the refusal is the honest answer.
func TestTheDaemonOverviewPortsRefuseForAHostWithNoDaemon(t *testing.T) {
	ports := system.PortsFromKernel(nil)

	if ports.Ready() {
		t.Fatal("a host with no daemon reports a daemon")
	}
	if ports.Events() != nil {
		t.Fatal("a host with no daemon holds a journal")
	}
	if ports.Gate() != nil {
		t.Fatal("a host with no daemon holds a gate")
	}
	if ports.Entries() != nil || ports.Pending() != 0 || ports.Skills() != 0 || ports.ToolCount() != 0 {
		t.Fatal("a host with no daemon answered a question about one")
	}

	service := system.New(ports, nil, nil, nil, "")
	status, err := service.Status(context.Background(), system.StatusInput{})
	if err == nil {
		t.Fatalf("a host with no daemon reported a status: %+v", status)
	}
	if !errors.Is(err, system.ErrNoDaemon) {
		t.Fatalf("the refusal is %v, want %v", err, system.ErrNoDaemon)
	}
	if status.Daemon != "" || status.Tools != 0 || status.MemoryRecords != 0 {
		t.Fatalf("the refusal answered a status anyway: %+v", status)
	}
}

// The daemon's own gate is a CAPABILITY, and the daemon's lifecycle service
// refuses without one: halting a daemon that is not there is not a halt.
func TestTheDaemonOverviewGateRefusesForAHostWithNoDaemon(t *testing.T) {
	lifecycle := system.NewLifecycle(system.PortsFromKernel(nil).Gate(), func() {})

	if _, err := lifecycle.Halt(context.Background(), system.ReasonRequest{}); !errors.Is(err, system.ErrNoGate) {
		t.Errorf("the halt answered %v, want %v", err, system.ErrNoGate)
	}
	if _, err := lifecycle.Resume(context.Background(), system.ReasonRequest{}); !errors.Is(err, system.ErrNoGate) {
		t.Errorf("the resume answered %v, want %v", err, system.ErrNoGate)
	}
	if _, err := lifecycle.Verify(context.Background(), system.EmptyRequest{}); !errors.Is(err, system.ErrNoGate) {
		t.Errorf("the verify answered %v, want %v", err, system.ErrNoGate)
	}
}

// The gate really reaches the daemon: the halt stops the daemon's own runs,
// and the daemon says so.
func TestTheDaemonOverviewGateReachesTheDaemon(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	gate := system.PortsFromKernel(k).Gate()
	if gate == nil {
		t.Fatal("the gate is missing")
	}
	gate.HaltWith("test")
	if !k.IsHalted() {
		t.Fatal("the gate halted nothing")
	}
	gate.ResumeWith("test")
	if k.IsHalted() {
		t.Fatal("the gate resumed nothing")
	}
	if err := gate.Verify(); err != nil {
		t.Fatalf("the gate's journal check failed: %v", err)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheDaemonOverviewPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(system.Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	if port.NumMethod() == 0 {
		t.Fatal("the daemon overview's port has no methods — the overview cannot answer anything")
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the daemon overview's port %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	// The FIELDS may not carry a kernel verb's name either: a port field
	// that answers to a verb is a verb the adapter may name.
	for i := 0; i < port.NumField(); i++ {
		name := port.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the daemon overview's port field %s is the kernel's own verb — a port field may not carry the name of the verb it stands for", name)
		}
	}
}

// The daemon's own lifecycle service halts the daemon THROUGH the gate.
//
// The gate test above asks the gate; this one asks the SURFACE — the service
// that answers "halt" — and watches the daemon. A service that answered
// "halted" without reaching the daemon would be the worst kind of lie.
func TestTheDaemonOverviewLifecycleHaltsTheDaemon(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	lifecycle := system.NewLifecycle(system.PortsFromKernel(k).Gate(), func() {})
	out, err := lifecycle.Halt(context.Background(), system.ReasonRequest{Reason: json.RawMessage(`"the operator asked"`)})
	if err != nil {
		t.Fatalf("Halt: %v", err)
	}
	if !out.OK || !out.Halted {
		t.Fatalf("the halt reported %+v", out)
	}
	if !k.IsHalted() {
		t.Fatal("the halt halted nothing")
	}

	resumed, err := lifecycle.Resume(context.Background(), system.ReasonRequest{Reason: json.RawMessage(`"the operator changed their mind"`)})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !resumed.OK {
		t.Fatalf("the resume reported %+v", resumed)
	}
	if k.IsHalted() {
		t.Fatal("the resume resumed nothing")
	}
}

// The port answers the questions AT THE MOMENT they are asked: a daemon that
// changes while the overview holds the port is read as it is NOW, not as it
// was when the port was filled.
//
// The test halts the daemon AFTER the port was filled — a port that cached
// its answers would still report a running daemon.
func TestTheDaemonOverviewPortAsksWhenItIsAsked(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := system.PortsFromKernel(k)
	if ports.Stopped() {
		t.Fatal("the daemon is not halted yet")
	}
	k.Halt()
	if !ports.Stopped() {
		t.Fatal("the port answered from the moment it was filled, not from the daemon now")
	}
	if ports.ActiveRunCount() != k.ActiveRuns() {
		t.Fatal("the port's run count does not follow the daemon")
	}
}
