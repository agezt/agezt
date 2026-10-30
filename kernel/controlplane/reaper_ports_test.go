// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// fakeReaperScanner is a reaper that always answers with the same dead agent.
//
// The reaper is a MODULE the composition root injects, so the daemon may hold
// none — and the fake is what tells the two apart.
type fakeReaperScanner struct{ report api.ReaperReport }

func (f fakeReaperScanner) Scan(int64, int64) api.ReaperReport { return f.report }

// The host does not bind the reaper family's ports — the APPLICATION fills
// them from the kernel handle.
//
// The test pins that binding end to end: the daemon-level binding hands over
// the **daemon's own** scan, and not an empty report.
func TestTheAdapterBindsTheReaperPortsToTheDaemon(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()
	root.SetReaperScanner(fakeReaperScanner{report: api.ReaperReport{DeadAgents: []agentapi.ReaperAgent{{Slug: "idle"}}}})

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	report := s.reaperPorts().Scan(1, 1)
	if len(report.DeadAgents) != 1 || report.DeadAgents[0].Slug != "idle" {
		t.Fatalf("the daemon's reaper ports answered %+v, want the daemon's own scan", report)
	}
}

// And the EMPTY half: a daemon that was given no reaper module holds none,
// and the binding answers an empty report rather than inventing one.
func TestTheAdapterBindsNoReaperToADaemonWithoutOne(t *testing.T) {
	root, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	s := NewServer(binding.ScopeFromKernel(root, t.TempDir()))

	report := s.reaperPorts().Scan(1, 1)
	if len(report.DeadAgents) != 0 {
		t.Fatalf("a daemon with no reaper answers %+v, want an empty report", report)
	}
}
