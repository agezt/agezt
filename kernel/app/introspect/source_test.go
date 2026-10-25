// SPDX-License-Identifier: MIT

package introspect

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/introspectapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/types"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The app-layer implementation must answer the contract port. A compile-time
// assertion: if the implementation's shape drifts from the port, the wiring
// site breaks instead of the tool.
var _ introspectapi.Source = (*kernelSource)(nil)

// TestNewKernelSource: the constructor returns a Source even for a nil kernel
// (the tool stays bound; the gather panics only if it is actually called).
func TestNewKernelSource(t *testing.T) {
	if NewKernelSource(nil) == nil {
		t.Fatal("NewKernelSource should return a Source even with a nil kernel")
	}
}

// TestKernelSourceCarriesTheKernelCeilings: the live kernel's ceilings reach
// the overview as they are — the adapter hands the kernel's own row over, so
// the agent reads the same numbers the status operation publishes.
func TestKernelSourceCarriesTheKernelCeilings(t *testing.T) {
	k, err := kernelruntime.Open(withWorld(t, kernelruntime.Config{
		BaseDir:                    t.TempDir(),
		Provider:                   mock.New(),
		Tools:                      map[string]toolapi.Tool{},
		SubAgentTool:               true,
		SubAgentMaxDepth:           2,
		SubAgentMaxFanout:          3,
		SubAgentMaxSpendMicrocents: 500,
		SubAgentMaxTotal:           7,
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	got := NewKernelSource(k).Overview().Delegation
	want := types.SubAgentLimits{Enabled: true, MaxDepth: 2, MaxFanout: 3, MaxSpendMicrocents: 500, MaxTotal: 7}
	if got != want {
		t.Fatalf("delegation = %+v, want %+v", got, want)
	}
}

// TestThePortReadsTheLiveKernelSchedulesAndStanding: the port's other two
// reads must come from the live kernel too — a schedule the operator added
// and a standing order the operator left switched on have to be listed by
// the tool, or the agent reports a daemon that owns less than it does.
func TestThePortReadsTheLiveKernelSchedulesAndStanding(t *testing.T) {
	// The test is the composition root (W3.49): the schedule library is the
	// daemon's file, so the test opens it and hands the kernel the
	// contract's host port.
	dir := t.TempDir()
	sched, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	// The standing library is the daemon's file too (W3.50): the test opens
	// it and hands the kernel the contract's store port.
	standingStore, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	k, err := kernelruntime.Open(withWorld(t, kernelruntime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		Tools:     map[string]toolapi.Tool{},
		Schedules: sched,
		Standing:  standingStore,
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.Schedules().Add("morning brief", time.Hour, "deepseek-chat", "operator", time.Now()); err != nil {
		t.Fatalf("Add schedule: %v", err)
	}
	if _, err := k.Standing().Add(standingapi.Order{
		Name:     "watch the deploys",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	}); err != nil {
		t.Fatalf("Add standing order: %v", err)
	}

	src := NewKernelSource(k)

	entries := src.Schedules()
	if len(entries) != 1 {
		t.Fatalf("the port listed %d schedules, want 1", len(entries))
	}
	if entries[0].Intent != "morning brief" {
		t.Fatalf("the port listed the wrong schedule: %+v", entries[0])
	}

	orders := src.Standing()
	if len(orders) != 1 {
		t.Fatalf("the port listed %d standing orders, want 1", len(orders))
	}
	if orders[0].Name != "watch the deploys" {
		t.Fatalf("the port listed the wrong standing order: %+v", orders[0])
	}
}

// TestTheReaperProjectionCarriesEveryField: the projection from the
// reaper's own row to the contract's row is a hand-written field-for-field
// copy. A field left out of it would be silent — the agent would read a
// fleet scan that is missing a bucket and never be told. So every field is
// measured here.
func TestTheReaperProjectionCarriesEveryField(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		DeadAgents: []kernelruntime.ReaperAgent{
			{Slug: "idle-one", Name: "Idle One", LastActiveMS: 111},
		},
		DegradedAgents: []kernelruntime.DegradedAgent{
			{
				Slug: "sick-one", Name: "Sick One", Failures: 4, Window: 60, Threshold: 3,
				DoctorAgent: "doctor", SelfRepairEnabled: true, EscalateTo: "human",
				LastFailureMS: 222, LastReason: "boom",
			},
		},
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{
			{
				Slug: "broken-one", Name: "Broken One", Issues: []string{"model: missing"},
				DoctorAgent: "doctor", SelfRepairEnabled: true, EscalateTo: "human",
			},
		},
		StaleArtifacts: 7,
		StaleBytes:     4096,
	}

	out := reaperReport(rep)
	if out.StaleArtifacts != 7 || out.StaleBytes != 4096 {
		t.Fatalf("the stale counters were lost: %+v", out)
	}
	if len(out.DeadAgents) != 1 {
		t.Fatalf("the dead bucket was lost: %+v", out)
	}
	if got := out.DeadAgents[0]; got.Slug != "idle-one" || got.Name != "Idle One" || got.LastActiveMS != 111 {
		t.Fatalf("the dead agent row lost a field: %+v", got)
	}
	if len(out.DegradedAgents) != 1 {
		t.Fatalf("the degraded bucket was lost: %+v", out)
	}
	if got := out.DegradedAgents[0]; got.Slug != "sick-one" || got.Name != "Sick One" ||
		got.Failures != 4 || got.Window != 60 || got.Threshold != 3 ||
		got.DoctorAgent != "doctor" || !got.SelfRepairEnabled || got.EscalateTo != "human" ||
		got.LastFailureMS != 222 || got.LastReason != "boom" {
		t.Fatalf("the degraded agent row lost a field: %+v", got)
	}
	if len(out.MisconfiguredAgents) != 1 {
		t.Fatalf("the misconfigured bucket was lost: %+v", out)
	}
	got := out.MisconfiguredAgents[0]
	if got.Slug != "broken-one" || got.Name != "Broken One" || got.DoctorAgent != "doctor" ||
		!got.SelfRepairEnabled || got.EscalateTo != "human" {
		t.Fatalf("the misconfigured agent row lost a field: %+v", got)
	}
	if len(got.Issues) != 1 || got.Issues[0] != "model: missing" {
		t.Fatalf("the misconfigured agent's issues were lost: %+v", got)
	}
}

// TestTheReaperOverviewProjectionCarriesEveryField: the same projection for
// the overview's fleet snapshot — the counts and the slugs.
func TestTheReaperOverviewProjectionCarriesEveryField(t *testing.T) {
	rep := kernelruntime.ReaperReport{
		DeadAgents:          []kernelruntime.ReaperAgent{{Slug: "a"}, {Slug: "b"}},
		DegradedAgents:      []kernelruntime.DegradedAgent{{Slug: "c"}},
		MisconfiguredAgents: []kernelruntime.MisconfiguredAgent{{Slug: "d"}},
	}
	out := reaperOverview(rep)
	if out.DeadAgents != 2 || out.DegradedAgents != 1 || out.MisconfiguredAgents != 1 {
		t.Fatalf("the fleet counts were lost: %+v", out)
	}
	if len(out.DeadSlugs) != 2 || out.DeadSlugs[0] != "a" || out.DeadSlugs[1] != "b" {
		t.Fatalf("the dead slugs were lost: %+v", out)
	}
	if len(out.DegradedSlugs) != 1 || out.DegradedSlugs[0] != "c" {
		t.Fatalf("the degraded slugs were lost: %+v", out)
	}
	if len(out.MisconfiguredSlugs) != 1 || out.MisconfiguredSlugs[0] != "d" {
		t.Fatalf("the misconfigured slugs were lost: %+v", out)
	}
}

// TestTheOverviewIsAssembledFromTheLiveKernel: the adapter gathers from the
// kernel's own slices — the journal head, the tool names, the schedule counts
// — so the overview is the daemon's state and not a copy of it.
func TestTheOverviewIsAssembledFromTheLiveKernel(t *testing.T) {
	k, err := kernelruntime.Open(withWorld(t, kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(),
		Tools:    map[string]toolapi.Tool{"zebra": nil, "alpha": nil},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	ov := NewKernelSource(k).Overview()
	if ov.Daemon == "" {
		t.Error("the overview must name the daemon")
	}
	head, _ := k.Journal().Head()
	if head < 0 {
		head = 0 // an empty journal has no head; the overview renders 0
	}
	if ov.JournalHead != head {
		t.Errorf("the overview's journal head = %d, want the kernel's %d", ov.JournalHead, head)
	}
	if len(ov.Tools) == 0 {
		t.Error("the overview must list the registered tools")
	}
	for i := 1; i < len(ov.Tools); i++ {
		if ov.Tools[i-1] > ov.Tools[i] {
			t.Fatalf("the tool names must be sorted: %v", ov.Tools)
		}
	}
	// Both registered tools must be there — a list that lost one would still
	// be sorted, so the membership is measured too.
	for _, want := range []string{"alpha", "zebra"} {
		found := false
		for _, got := range ov.Tools {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("the overview lost the tool %q: %v", want, ov.Tools)
		}
	}
}
