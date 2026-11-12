// SPDX-License-Identifier: MIT

package resume

// The ticket's LIFECYCLE lives in this package since W3.76ah. The daemon used
// to answer for it through three verbs (ClaimResumeTicket,
// FinalizeResumeTicket, ResumeCheckpointFn) and the run engine — which lives
// INSIDE this module — asked the host for them: a round trip inside one
// module. These tests pin the module's half of that move: the lifecycle is
// here, and the host's own duties (the boot-time resumer and the shutdown
// protocol) stay there.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// testService builds a lifecycle over a fresh store.
func testService(t *testing.T, enabled bool) (*Service, *Store) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "resume"), 0)
	if err != nil {
		t.Fatalf("resume.Open: %v", err)
	}
	return New(Dependencies{Store: store, Enabled: func() bool { return enabled }}), store
}

// TestTheScopeRuleIsTheModulesRule pins the scope rule: a ROOT run carries a
// ticket, a delegated run does not.
func TestTheScopeRuleIsTheModulesRule(t *testing.T) {
	svc, _ := testService(t, true)

	root := context.Background()
	if !svc.active(root) {
		t.Fatal("a root run must carry a ticket")
	}
	delegated := runctx.WithWakeContext(root, runctx.WakeContext{ParentCorrelation: "corr-parent"})
	if svc.active(delegated) {
		t.Fatal("a delegated run must not carry its own ticket")
	}
}

// TestTheScopeRuleNeedsTheFeatureAndTheStore pins the other two doors: resume
// off, or no store, means no ticket.
func TestTheScopeRuleNeedsTheFeatureAndTheStore(t *testing.T) {
	off, _ := testService(t, false)
	if off.active(context.Background()) {
		t.Fatal("resume disabled must mean no ticket")
	}
	nostore := New(Dependencies{Enabled: func() bool { return true }})
	if nostore.active(context.Background()) {
		t.Fatal("no store must mean no ticket")
	}
}

// TestTheClaimWritesTheTicketAndOwnsIt pins the claim: a fresh ticket on disk
// and an owned context, so the caller knows it must finalize.
func TestTheClaimWritesTheTicketAndOwnsIt(t *testing.T) {
	svc, store := testService(t, true)

	ctx, owns := svc.Claim(context.Background(), "corr-1", "do the thing", KindRun, 0)
	if !owns {
		t.Fatal("a root run must own its ticket")
	}
	if _, ok := OwnedKindFromCtx(ctx); !ok {
		t.Fatal("the claimed context must carry the owned marker")
	}
	ticket, ok, err := store.Get("corr-1")
	if err != nil || !ok {
		t.Fatalf("the ticket must be on disk: %v", err)
	}
	if ticket.Kind != KindRun || ticket.Intent != "do the thing" {
		t.Fatalf("the ticket lost its facts: %+v", ticket)
	}
}

// TestTheClaimStaysQuietWhenAnOuterFrameOwnsTheTicket pins the ownership
// dance: a governed wrapper (RunAssured/RunWithRetry) or the resumer already
// owns the ticket, so the inner run neither recreates it (which would reset
// the crash-loop attempt counter) nor claims it again.
func TestTheClaimStaysQuietWhenAnOuterFrameOwnsTheTicket(t *testing.T) {
	svc, store := testService(t, true)

	owned := WithOwned(context.Background(), KindAssured)
	ctx, owns := svc.Claim(owned, "corr-1", "task", KindRun, 0)
	if owns {
		t.Fatal("an outer frame's ticket must not be claimed again")
	}
	if _, ok, _ := store.Get("corr-1"); ok {
		t.Fatal("a second claim must not write a second ticket")
	}
	if _, ok := OwnedKindFromCtx(ctx); !ok {
		t.Fatal("the owned marker must survive the claim")
	}
}

// TestTheTicketCapturesTheGovernanceCeilings pins the capture: the lifecycle
// READS the run's context and hands the facts to the module's builder. A
// tightened trust ceiling, a cost cap and a per-run timeout must all survive
// into the ticket — resume must never silently regain authority or guess a
// cap.
func TestTheTicketCapturesTheGovernanceCeilings(t *testing.T) {
	profile := agentapi.Profile{Slug: "ops", Soul: "ops soul", Model: "ops-model", TrustCeiling: "L2"}
	ctx := runctx.WithAgentProfile(context.Background(), profile)
	ctx = runctx.WithMaxCost(ctx, 321)
	ctx = runctx.WithRunTimeout(ctx, 2*time.Second)

	svc, _ := testService(t, true)
	ticket := svc.buildTicket(ctx, "corr-1", "task", KindRun, 0)
	if ticket.TrustCeiling == nil || *ticket.TrustCeiling != int(edict.LevelAskFirst) {
		t.Fatalf("trust ceiling lost: %+v", ticket.TrustCeiling)
	}
	if ticket.MaxCostMc != 321 {
		t.Fatalf("cost cap lost: %d", ticket.MaxCostMc)
	}
	if ticket.RunTimeoutMs != 2000 {
		t.Fatalf("run timeout lost: %d", ticket.RunTimeoutMs)
	}
	if ticket.AgentSlug != "ops" {
		t.Fatalf("agent slug lost: %q", ticket.AgentSlug)
	}
	// The run's resolved settings travel with the ticket AND keep their SOURCE:
	// a profile-sourced system prompt is reconstructible on the next boot, an
	// explicit per-run override is not. A ticket that lost the source would
	// resume a run under different constraints — or refuse to resume one that
	// it could.
	if !ticket.Resumable {
		t.Fatalf("a profile-sourced run must stay resumable: %+v", ticket)
	}
	overridden := runctx.WithSystem(runctx.WithAgentProfile(context.Background(), profile), "explicit system")
	overrideTicket := svc.buildTicket(overridden, "corr-2", "task", KindRun, 0)
	if overrideTicket.Resumable {
		t.Fatalf("a per-run override must not be resumable: %+v", overrideTicket)
	}
}

// TestTheKeepDeleteRuleIsTheModulesRule pins the termination rule: a run
// interrupted by a shutdown keeps its ticket, every other terminal outcome
// deletes it.
func TestTheKeepDeleteRuleIsTheModulesRule(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "resume"), 0)
	if err != nil {
		t.Fatalf("resume.Open: %v", err)
	}
	suspending := false
	svc := New(Dependencies{
		Store:      store,
		Enabled:    func() bool { return true },
		Suspending: func() bool { return suspending },
	})

	if err := store.Put(&Ticket{Corr: "corr-1", Intent: "task", Kind: KindRun}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	svc.Finalize("corr-1", context.Canceled)
	if _, ok, _ := store.Get("corr-1"); ok {
		t.Fatal("an ordinary cancellation deletes the ticket")
	}

	if err := store.Put(&Ticket{Corr: "corr-2", Intent: "task", Kind: KindRun}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	suspending = true
	svc.Finalize("corr-2", context.Canceled)
	if _, ok, _ := store.Get("corr-2"); !ok {
		t.Fatal("a run interrupted by a shutdown KEEPS its ticket")
	}
}

// TestTheCheckpointRefreshesTheSnapshot pins the per-iteration snapshot: the
// closure copies the messages (the loop keeps appending after it returns) and
// stores them with the iteration.
func TestTheCheckpointRefreshesTheSnapshot(t *testing.T) {
	svc, store := testService(t, true)

	if err := store.Put(&Ticket{Corr: "corr-1", Intent: "task", Kind: KindRun}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	checkpoint := svc.CheckpointFn("corr-1")
	if checkpoint == nil {
		t.Fatal("a stored ticket must have a checkpoint")
	}
	msgs := []llm.Message{{Role: "user", Content: "hello"}}
	checkpoint(3, msgs)
	msgs[0].Content = "mutated after the snapshot"

	ticket, ok, err := store.Get("corr-1")
	if err != nil || !ok {
		t.Fatalf("the ticket must survive: %v", err)
	}
	if ticket.Iter != 3 {
		t.Fatalf("the iteration lost: %d", ticket.Iter)
	}
	if len(ticket.Messages) != 1 || ticket.Messages[0].Content != "hello" {
		t.Fatalf("the snapshot must be a copy: %+v", ticket.Messages)
	}
}

// TestTheCheckpointIsFreeWhenResumeIsOff pins the zero-overhead promise: a
// service without a store hands the loop no closure at all.
func TestTheCheckpointIsFreeWhenResumeIsOff(t *testing.T) {
	svc := New(Dependencies{})
	if svc.CheckpointFn("corr-1") != nil {
		t.Fatal("no store must mean no checkpoint")
	}
}

// TestTheAnomalyTravelsOnTheBus pins the lifecycle's one host-facing port: the
// anomalies are published on the host's bus.
func TestTheAnomalyTravelsOnTheBus(t *testing.T) {
	var seen []event.Spec
	svc := New(Dependencies{
		Store:   &Store{},
		Enabled: func() bool { return true },
		Publish: func(spec event.Spec) (*event.Event, error) {
			seen = append(seen, spec)
			return nil, nil
		},
	})
	svc.publishAnomaly("snapshot_failed", "corr-1", context.Canceled)
	if len(seen) != 1 {
		t.Fatalf("the anomaly must be published once: %+v", seen)
	}
	if seen[0].Subject != "run.resume.anomaly" {
		t.Fatalf("the anomaly subject changed: %q", seen[0].Subject)
	}
	if seen[0].Kind != event.KindAnomalyDetected {
		t.Fatalf("the anomaly kind changed: %q", seen[0].Kind)
	}
}

// TestTheSuspendTransitionIsTheModulesOwn pins the suspend transition: the
// module's lifecycle flips the tickets, and a failure is published as an
// anomaly rather than swallowed.
func TestTheSuspendTransitionIsTheModulesOwn(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "resume"), 0)
	if err != nil {
		t.Fatalf("resume.Open: %v", err)
	}
	var published int
	svc := New(Dependencies{
		Store:   store,
		Enabled: func() bool { return true },
		Publish: func(spec event.Spec) (*event.Event, error) {
			published++
			return nil, nil
		},
	})
	if err := store.Put(&Ticket{Corr: "corr-1", Intent: "task", Kind: KindRun, Status: StatusActive}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	n, err := svc.SuspendAll()
	if err != nil {
		t.Fatalf("SuspendAll: %v", err)
	}
	if n != 1 {
		t.Fatalf("one ticket must be suspended: %d", n)
	}
	ticket, ok, _ := store.Get("corr-1")
	if !ok || ticket.Status != StatusSuspended {
		t.Fatalf("the ticket must be suspended: %+v", ticket)
	}
	if published != 0 {
		t.Fatalf("a clean suspend publishes no anomaly: %d", published)
	}
}

// TestTheTicketKeepsTheProfileDefaultsAndTheExplicitOverrides pins the
// capture's last rule: a run that used the PROFILE's defaults is
// reconstructible on the next boot, and a run that overrode them is not —
// resume under the wrong constraints is worse than no resume at all.
func TestTheTicketKeepsTheProfileDefaultsAndTheExplicitOverrides(t *testing.T) {
	profile := agentapi.Profile{Slug: "ops", Soul: "ops soul", Model: "ops-model", TrustCeiling: "L2"}
	for _, tc := range []struct {
		name      string
		decorate  func(context.Context) context.Context
		resumable bool
	}{
		{"profile defaults", func(ctx context.Context) context.Context { return ctx }, true},
		{"explicit system", func(ctx context.Context) context.Context { return runctx.WithSystem(ctx, "custom") }, false},
		{"explicit model", func(ctx context.Context) context.Context { return runctx.WithModel(ctx, "custom") }, false},
		{"same system explicitly", func(ctx context.Context) context.Context {
			return runctx.WithSystem(ctx, runctx.AgentProfileSystem(profile))
		}, false},
		{"same model explicitly", func(ctx context.Context) context.Context { return runctx.WithModel(ctx, profile.Model) }, false},
		{"explicit tools", func(ctx context.Context) context.Context { return runctx.WithTools(ctx, []string{"echo"}) }, false},
		{"explicit empty tools", func(ctx context.Context) context.Context { return runctx.WithTools(ctx, []string{}) }, false},
		{"retargeted identity", func(ctx context.Context) context.Context { return runctx.WithAgentIdent(ctx, "other", 0) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := runctx.WithAgentProfile(context.Background(), profile)
			ctx = runctx.WithMaxCost(ctx, 321)
			ctx = tc.decorate(ctx)
			svc, _ := testService(t, true)
			ticket := svc.buildTicket(ctx, "corr", "task", KindRun, 0)
			if ticket.Resumable != tc.resumable {
				t.Fatalf("Resumable=%v want %v", ticket.Resumable, tc.resumable)
			}
			if ticket.TrustCeiling == nil || *ticket.TrustCeiling != int(edict.LevelAskFirst) || ticket.MaxCostMc != 321 {
				t.Fatalf("governance lost: %+v", ticket)
			}
		})
	}
	ctx := runctx.WithAgentProfile(context.Background(), agentapi.Profile{Soul: "unnamed soul", Model: "unnamed-model"})
	svc, _ := testService(t, true)
	if svc.buildTicket(ctx, "unnamed", "task", KindRun, 0).Resumable {
		t.Fatal("unnamed profile cannot be reconstructed")
	}
}

// TestTheLifecycleIsQuietWithoutItsPorts pins the nil-port discipline: a
// service without a bus stays quiet, and one without a store writes nothing.
func TestTheLifecycleIsQuietWithoutItsPorts(t *testing.T) {
	svc := New(Dependencies{Enabled: func() bool { return true }})
	ctx, owns := svc.Claim(context.Background(), "corr-1", "task", KindRun, 0)
	if owns {
		t.Fatal("no store must mean no ticket")
	}
	if _, ok := OwnedKindFromCtx(ctx); ok {
		t.Fatal("no store must mean no owned marker")
	}
	// A finalize without a store is a no-op, and it must not panic.
	svc.Finalize("corr-1", context.Canceled)
}
