// SPDX-License-Identifier: MIT

package runtime

// The host's face of the agents module's reaper. The module is injected —
// the runtime may not import a module's implementation (archcheck's principle
// P5) — so the kernel's scan IS the module's scan, and a kernel whose
// composition root forgot to wire it has no scanner to ask.

import (
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
)

// fakeReaper stands in for the agents module's reaper.
type fakeReaper struct {
	report     api.ReaperReport
	calls      int
	idleCutoff int64
}

func (f *fakeReaper) Scan(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
	f.calls++
	f.idleCutoff = agentIdleCutoffMs
	return f.report
}

// TestReaperScan_DelegatesToTheInjectedModule: the kernel's scan is the
// module's scan — the cutoffs travel with it and its report comes back.
func TestReaperScan_DelegatesToTheInjectedModule(t *testing.T) {
	k := openCausesKernel(t)
	fake := &fakeReaper{report: api.ReaperReport{DeadAgents: []api.ReaperAgent{{Slug: "idle"}}}}
	k.SetReaperScanner(fake)

	rep := k.ReaperScan(42, 43)
	if fake.calls != 1 || fake.idleCutoff != 42 {
		t.Fatalf("the module was not asked as the scan demands: calls=%d idle cutoff=%d", fake.calls, fake.idleCutoff)
	}
	if len(rep.DeadAgents) != 1 || rep.DeadAgents[0].Slug != "idle" {
		t.Fatalf("the module's report did not reach the caller: %+v", rep)
	}
}

// TestReaperScan_WithoutTheModuleAnswersEmpty: an unwired kernel has no
// scanner to ask, so it answers with an empty report rather than a panic.
func TestReaperScan_WithoutTheModuleAnswersEmpty(t *testing.T) {
	k := openCausesKernel(t)
	rep := k.ReaperScan(time.Now().UnixMilli(), time.Now().UnixMilli())
	if !rep.Empty() {
		t.Fatalf("an unwired kernel must answer with an empty report: %+v", rep)
	}
}

// TestReaperPorts_HandTheHostItsOwnFacts: the ports are the host's own
// sources — its journal, its fleet — not the module's.
func TestReaperPorts_HandTheHostItsOwnFacts(t *testing.T) {
	k := openCausesKernel(t)
	ports := k.ReaperPorts()
	if ports.Journal == nil || ports.Profiles == nil {
		t.Fatal("the host's ports must carry the journal and the fleet")
	}
	if _, err := k.Bus().Publish(event.Spec{Subject: "test.reaper", Kind: event.KindInfo, Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	seen := 0
	_ = ports.Journal(func(e *event.Event) error {
		seen++
		return nil
	})
	if seen == 0 {
		t.Fatal("the journal port walked no events")
	}
	if profiles := ports.Profiles(); profiles == nil {
		t.Fatal("the fleet port listed no profiles")
	}
}
