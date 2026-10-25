// SPDX-License-Identifier: MIT

package guardians

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/guardianapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The adapter answers the contract's port.
var _ guardianapi.Host = (*kernelHost)(nil)

// TestAMissingKernelIsANilPort: a daemon with no kernel hands over a NIL
// port, not a port wrapping nothing.
func TestAMissingKernelIsANilPort(t *testing.T) {
	if got := NewKernelHost(nil); got != nil {
		t.Fatalf("a missing kernel must yield a nil port, got %v", got)
	}
}

// TestTheAdapterAnswersTheLiveRoster: the seeder writes to the LIVE kernel —
// an agent it adds is really in the roster, and the edit it makes is really
// stored.
func TestTheAdapterAnswersTheLiveRoster(t *testing.T) {
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	host := NewKernelHost(k)
	added, err := host.AddAgent(agentapi.Profile{Slug: "guardian-health", System: true})
	if err != nil {
		t.Fatalf("AddAgent: %v", err)
	}
	if added.Slug != "guardian-health" {
		t.Fatalf("the added agent = %+v", added)
	}
	if len(host.Agents()) != 1 {
		t.Fatalf("the live roster holds %d agents, want 1", len(host.Agents()))
	}

	_, found, err := host.UpdateAgent("guardian-health", func(p *agentapi.Profile) {
		// A field the roster does NOT default: the system-guardian defaults
		// would fill a noise policy of their own and mask a dropped edit.
		p.Description = "edited through the port"
	})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if !found {
		t.Fatal("the port must find the agent the roster holds")
	}
	stored, ok := k.Roster().Get("guardian-health")
	if !ok || stored.Description != "edited through the port" {
		t.Fatalf("the edit was not stored in the roster: %+v ok=%v", stored, ok)
	}
}

// TestTheAdapterReportsAnUnknownAgent: an unknown ref is answered as "not
// found" and no error — the standing pattern.
func TestTheAdapterReportsAnUnknownAgent(t *testing.T) {
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	_, found, err := NewKernelHost(k).UpdateAgent("nobody", func(*agentapi.Profile) {})
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
	if found {
		t.Fatal("an unknown agent must not be found")
	}
}

// TestTheAdapterAnswersTheLiveSchedules: the schedule half of the port is
// live too — a schedule the seeder plants is really in the daemon's store.
//
// The test is the composition root (W3.49): the schedule library is the
// daemon's file, so the test opens it and hands the kernel the contract's
// host port.
func TestTheAdapterAnswersTheLiveSchedules(t *testing.T) {
	dir := t.TempDir()
	sched, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	k, err := kernelruntime.Open(withRoster(t, kernelruntime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		Schedules: sched,
	}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	host := NewKernelHost(k)
	entry, err := host.AddInterval("cycle repo-watch", 90*time.Second, "mock-model", "")
	if err != nil {
		t.Fatalf("AddInterval: %v", err)
	}
	if entry.Intent != "cycle repo-watch" {
		t.Fatalf("the schedule = %+v", entry)
	}
	if len(host.Schedules()) != 1 {
		t.Fatalf("the daemon holds %d schedules, want 1", len(host.Schedules()))
	}
}
