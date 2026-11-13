// SPDX-License-Identifier: MIT

package loopcfg

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/modules/runs/agent"
)

// The loop configuration's ENTRY belongs to the module (W3.77).
//
// The daemon held a door over the rule (BuildLoopConfig): it projected its
// effective configuration, wired its live collaborators and called the
// module's service. The run engine — which lives INSIDE the module that owns
// the rule — asked the host for it through its port, and the delegation engine
// — which lives next door in the same module — asked the host for it too: two
// round trips inside one module.
//
// The knobs are the HOST's own state (the effective configuration is per-run:
// the operator's live edits plus the agent's own overrides), so the rule asks
// for them through the dependency the host wired — per run, never a snapshot.

// TestBuild_AsksTheHostForThisRunsKnobs pins the crossing: the rule asks the
// host for the knobs of THIS run, and the answer lands on the loop.
func TestBuild_AsksTheHostForThisRunsKnobs(t *testing.T) {
	asked := 0
	var askedCtx context.Context
	deps := baseDeps(toolSet("read"))
	deps.Config = func(ctx context.Context) Config {
		asked++
		askedCtx = ctx
		return Config{MaxIter: 42, ToolTimeout: 7}
	}
	svc := New(deps)

	ctx := context.WithValue(context.Background(), struct{ run string }{"corr"}, "corr")
	lc := svc.Build(ctx, "corr-1", "fake-model")

	if asked != 1 {
		t.Fatalf("the host was asked %d times, want once per build", asked)
	}
	if askedCtx != ctx {
		t.Fatal("the host was asked with another context — the knobs are per run")
	}
	if lc.MaxIter != 42 {
		t.Fatalf("MaxIter = %d, want 42 (the host's knobs)", lc.MaxIter)
	}
	if lc.ToolTimeout != 7 {
		t.Fatalf("ToolTimeout = %s, want 7s", lc.ToolTimeout)
	}
}

// TestBuild_AsksTheHostForEveryRun pins the crossing as PER RUN: a service
// built once still reads the knobs of the run it is building.
func TestBuild_AsksTheHostForEveryRun(t *testing.T) {
	iter := 1
	deps := baseDeps(toolSet("read"))
	deps.Config = func(context.Context) Config {
		iter++
		return Config{MaxIter: iter}
	}
	svc := New(deps)

	if lc := svc.Build(context.Background(), "corr-1", "fake-model"); lc.MaxIter != 2 {
		t.Fatalf("MaxIter = %d, want 2 (the first run's knobs)", lc.MaxIter)
	}
	if lc := svc.Build(context.Background(), "corr-2", "fake-model"); lc.MaxIter != 3 {
		t.Fatalf("MaxIter = %d, want 3 (the second run's knobs)", lc.MaxIter)
	}
}

// TestBuild_WithoutAHostKeepsTheModulesOwnCeiling pins the fallback: a
// service wired without the host's knobs keeps the MODULE's default ceiling.
// A zero ceiling would end every run at its first turn, and a missing host is
// a wiring mistake — not a reason to run with no limits at all.
func TestBuild_WithoutAHostKeepsTheModulesOwnCeiling(t *testing.T) {
	svc := New(baseDeps(toolSet("read")))
	lc := svc.Build(context.Background(), "corr-1", "fake-model")
	if lc.MaxIter != agent.DefaultMaxIter {
		t.Fatalf("MaxIter = %d, want the module's own default (%d)", lc.MaxIter, agent.DefaultMaxIter)
	}
}
