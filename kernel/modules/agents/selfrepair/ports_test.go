// SPDX-License-Identifier: MIT

package selfrepair

// The self-repair module's ports, and the ratchet that keeps them the only
// way in.
//
// Until W5.4 the module held the whole kernel handle and asked it for eleven
// verbs. That was the LAST cross-module edge in the tree: the agents module
// reached into the runs module (the daemon's own package) and the archcheck
// allowlist carried the line to excuse it.
//
// The module now names CAPABILITIES and the composition root fills them
// (cmd/agezt/selfrepair_port.go). These tests are the ratchet: the module
// must not grow the handle back, and a host that supplies nothing must be
// answered for, not called into.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// autoRepairPorts fills the module's ports from a kernel handle.
//
// The TEST HOST is the only place in this package that may hold a handle: the
// module itself may not (archcheck's principle P5), and until W5.4 it did —
// which is why this helper exists now and the production code has none.
//
// The wiring is the composition root's (cmd/agezt/selfrepair_port.go): the
// daemon hands the module the same eleven capabilities there.
func autoRepairPorts(k *kernelruntime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{
		Bus:          k.Bus(),
		Journal:      autoRepairJournalRange(k),
		LiveProvider: k.Provider,
		Scan:         k.ReaperScan,
		Profile:      k.RosterGet,
		Profiles:     k.RosterList,
		Roster:       k.RosterService(),
		Runner:       k.RunnerService(),
		Correlation:  k.NewCorrelation,
		BindProfile:  k.WithAgentProfile,
		CapCost:      k.WithMaxCost,
	}
}

// autoRepairJournalRange walks the daemon's durable journal once.
func autoRepairJournalRange(k *kernelruntime.Kernel) func(func(*event.Event) error) error {
	return func(fn func(*event.Event) error) error {
		j := k.Journal()
		if j == nil {
			return nil
		}
		return j.Range(fn)
	}
}

// TestTheSelfRepairModuleHoldsNoKernelHandle is the ratchet on the EDGE.
//
// The module may not name the daemon's package: the daemon is the runs
// module's own body, and a module may reach another module only through its
// api package. The run context is the same story one level down — the module
// asks for the binding through a PORT (the daemon's door), it does not import
// the run context's vocabulary.
//
// The check is over the package's PRODUCTION files: the test files may hold a
// handle (the test host does), and a file-scoped check would watch one file
// while the edge could hide in another.
func TestTheSelfRepairModuleHoldsNoKernelHandle(t *testing.T) {
	forbidden := []string{
		"github.com/agezt/agezt/kernel/runtime",
		"github.com/agezt/agezt/kernel/modules/runs/runctx",
	}
	for _, path := range forbidden {
		if productionPackageContains(t, path) {
			t.Fatalf("the self-repair module imports %q; the host's capabilities arrive through the ports", path)
		}
	}
	// The name itself must be gone: a stale alias would import the package
	// without spelling the path in the file the check reads.
	if productionPackageContains(t, "kernelruntime.") {
		t.Fatal("the self-repair module still names the daemon's package")
	}
}

// TestTheModulePortsAnswerForAHostThatSuppliedNothing pins the other half:
// the ports are OPTIONAL, and a host that hands the module nothing is
// answered for — no panic, no nil dereference, no invented fact.
func TestTheModulePortsAnswerForAHostThatSuppliedNothing(t *testing.T) {
	ports := Ports{}

	if _, ok := ports.profile("builder"); ok {
		t.Fatal("a host with no roster must not answer with an agent")
	}
	if got := ports.profiles(); got != nil {
		t.Fatalf("profiles = %+v, want none for a host with no roster", got)
	}
	if got := ports.scan(0, 0); len(got.MisconfiguredAgents) != 0 || len(got.DegradedAgents) != 0 {
		t.Fatalf("scan = %+v, want an empty report for a host with no scanner", got)
	}
	if err := ports.journalRange(func(*event.Event) error { return nil }); err != nil {
		t.Fatalf("journal walk = %v, want no history for a host with no journal", err)
	}
	if ports.liveProvider() != nil {
		t.Fatal("a host with no provider must answer with none")
	}
	if got := ports.correlation(); got != "" {
		t.Fatalf("correlation = %q, want none for a host that mints none", got)
	}
	ctx := context.Background()
	if ports.bindProfile(ctx, roster.Profile{Slug: "builder"}) != ctx {
		t.Fatal("a host with no profile door must leave the context alone")
	}
	if ports.capCost(ctx, 100) != ctx {
		t.Fatal("a host with no cost door must leave the context alone")
	}
}

// TestTheWakeRefusesWithoutItsDoors pins the one place where a missing port
// is NOT answered for: a wake runs a GOVERNED run for a named agent, and a
// wake without the run engine, the correlation or the profile binding would
// run somebody else. It refuses instead.
func TestTheWakeRefusesWithoutItsDoors(t *testing.T) {
	if _, err := autoRepairWakeAgent(context.Background(), Ports{}, autoRepairCandidate{
		Slug:       "builder",
		EscalateTo: "lead",
	}, nil); err == nil {
		t.Fatal("a wake without the run engine, the correlation and the profile binding must refuse")
	}
}

// TestTheScanPortIsTheModulesOwnReaper pins that the fleet scan arrives
// through the module's own reaper vocabulary (the api package's row), not
// through the daemon's names.
func TestTheScanPortIsTheModulesOwnReaper(t *testing.T) {
	var seen []int64
	ports := Ports{Scan: func(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
		seen = append(seen, agentIdleCutoffMs, artifactStaleCutoffMs)
		return api.ReaperReport{}
	}}
	ports.scan(11, 22)
	if len(seen) != 2 || seen[0] != 11 || seen[1] != 22 {
		t.Fatalf("scan cutoffs = %+v, want the caller's two cutoffs", seen)
	}
}

// TestTheProfileBindingIsTheHostsDoor pins that the module ASKS the host to
// bind the profile: the port is a question, not a private binding.
//
// The daemon's half is pinned on the other side of the boundary
// (cmd/agezt/selfrepair_port_test.go: the daemon's door really carries the
// profile into the run context). This half pins that the module really asks —
// a module that bound the profile itself would be a second spelling of the
// run context's rule, and the two would drift.
func TestTheProfileBindingIsTheHostsDoor(t *testing.T) {
	var (
		boundProfile roster.Profile
		cappedAt     int64
	)
	ports := Ports{
		BindProfile: func(ctx context.Context, p roster.Profile) context.Context {
			boundProfile = p
			return ctx
		},
		CapCost: func(ctx context.Context, mc int64) context.Context {
			cappedAt = mc
			return ctx
		},
	}

	ports.bindProfile(context.Background(), roster.Profile{Slug: "lead"})
	if boundProfile.Slug != "lead" {
		t.Fatalf("bound profile = %+v, want the profile the module asked for", boundProfile)
	}

	ports.capCost(context.Background(), 321)
	if cappedAt != 321 {
		t.Fatalf("cost ceiling = %d, want the ceiling the module asked for", cappedAt)
	}
}

// productionPackageContains reports whether the package's own PRODUCTION
// sources hold the given text. The test files may legitimately mention a
// name — the test host holds a kernel handle on purpose.
func productionPackageContains(t *testing.T, needle string) bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the self-repair package: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), needle) {
			return true
		}
	}
	return false
}
