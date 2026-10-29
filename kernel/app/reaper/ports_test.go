// SPDX-License-Identifier: MIT

package reaper

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeScanner is a reaper that always answers with the same dead agent, and
// remembers the cutoffs it was asked about.
//
// The reaper is a MODULE the composition root injects, so a kernel can hold
// none — and the fake is what tells the two apart: the kernel that was given
// a scanner answers with the fake's dead agent, and the kernel that was given
// none answers with an empty report.
//
// The cutoffs are remembered because they are TWO different things — the
// agents' idle cutoff and the artifacts' stale cutoff — and a port that
// swapped them would still answer a report.
type fakeScanner struct {
	report      api.ReaperReport
	idleCutoff  int64
	staleCutoff int64
}

func (f *fakeScanner) Scan(idleCutoff, staleCutoff int64) api.ReaperReport {
	f.idleCutoff, f.staleCutoff = idleCutoff, staleCutoff
	return f.report
}

// The reaper family's ports are filled by the APPLICATION from the kernel
// handle — the host never binds them itself.
//
// The test pins the FILLED half: the port hands over the daemon's OWN scan,
// not an empty report.
func TestTheReaperPortsReachTheKernelScan(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	scanner := &fakeScanner{report: api.ReaperReport{DeadAgents: []agentapi.ReaperAgent{{Slug: "idle"}}}}
	k.SetReaperScanner(scanner)

	ports := PortsFromKernel(k)

	report := ports.Scan(11, 22)
	if len(report.DeadAgents) != 1 || report.DeadAgents[0].Slug != "idle" {
		t.Fatalf("the reaper port answered %+v, want the daemon's own scan", report)
	}
	// The two cutoffs are two different things, and they arrive in the order
	// the port says they do.
	if scanner.idleCutoff != 11 || scanner.staleCutoff != 22 {
		t.Fatalf("the scan was asked about %d/%d, want the idle cutoff 11 and the stale cutoff 22", scanner.idleCutoff, scanner.staleCutoff)
	}
}

// And the EMPTY half: a daemon that was given no reaper module holds none, and
// the port answers an EMPTY report — not a report that reads like "nothing is
// wrong" by accident, but the kernel's own answer for an unwired daemon.
func TestTheReaperPortsAnswerEmptyWhenTheDaemonHasNoReaper(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	report := PortsFromKernel(k).Scan(1, 1)
	if len(report.DeadAgents) != 0 {
		t.Fatalf("a daemon with no reaper answers %+v, want an empty report", report)
	}
}

// A host with no kernel holds no reaper.
//
// The port answers for that case with an empty report too: there is no scan to
// ask, and the surfaces that read it show an empty report rather than reading
// from nothing.
func TestTheReaperPortsSurviveAHostWithNoKernel(t *testing.T) {
	report := PortsFromKernel(nil).Scan(1, 1)
	if len(report.DeadAgents) != 0 {
		t.Fatalf("a host with no kernel answers %+v, want an empty report", report)
	}
}

// The port's names may not be the kernel's own: a port that carried the name
// of the verb it stands for would let the adapter name the kernel's verb
// through the port's door.
//
// The rule is checked HERE, by reflection over the port's own method set and
// the kernel's, so it does not depend on what the guard happens to own today.
func TestTheReaperPortNamesAreNotTheKernelVerbs(t *testing.T) {
	port := reflect.TypeOf(Ports{})
	kernel := reflect.TypeOf(&runtime.Kernel{})
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the reaper port's %s is the kernel's own verb — a port may not carry the name of the verb it stands for", name)
		}
	}
	if port.NumMethod() == 0 {
		t.Fatal("the reaper port has no methods — the family cannot answer anything")
	}
}
