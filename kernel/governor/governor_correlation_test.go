// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
)

// firstOfKind returns the first journaled event of the given kind, or nil.
func firstOfKind(j *journal.Journal, kind event.Kind) *event.Event {
	var found *event.Event
	_ = j.Range(func(e *event.Event) error {
		if found == nil && e.Kind == kind {
			ev := *e
			found = &ev
		}
		return nil
	})
	return found
}

// TestGovernorEvents_CarryRunCorrelation: every Governor decision/pre-flight
// event must carry the request's correlation id, so it lands in the run timeline
// and `agt why <id>` reaches it — rather than being orphaned (the M379-class bug,
// generalised across the Governor). Each subtest drives one event kind with a
// known correlation and asserts the journaled event carries it.
func TestGovernorEvents_CarryRunCorrelation(t *testing.T) {
	const corr = "run-GOVCORR-1"

	t.Run("routing.decision", func(t *testing.T) {
		b, j := newBus(t)
		r := governor.NewRegistry()
		prov := &fakeProvider{name: "p", resp: okResp("m", 1, 1)}
		mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
		g, _ := governor.New(governor.Config{Registry: r, Bus: b})
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "m", CorrelationID: corr}); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		assertCorr(t, j, event.KindRoutingDecision, corr)
	})

	t.Run("provider.fallback", func(t *testing.T) {
		b, j := newBus(t)
		bad := &fakeProvider{name: "bad", err: errors.New("upstream 503")}
		good := &fakeProvider{name: "local", resp: okResp("m", 1, 1)}
		r := governor.NewRegistry()
		mustRegister(t, r,
			&governor.ProviderInfo{Name: "bad", Provider: bad, AuthMode: governor.AuthAPIKey},
			&governor.ProviderInfo{Name: "local", Provider: good, AuthMode: governor.AuthLocal, IsFallback: true},
		)
		g, _ := governor.New(governor.Config{Registry: r, Bus: b})
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "m", CorrelationID: corr}); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		assertCorr(t, j, event.KindProviderFallback, corr)
	})

	t.Run("rate.limited", func(t *testing.T) {
		b, j := newBus(t)
		prov := &fakeProvider{name: "p", resp: okResp("m", 1, 1)}
		r := governor.NewRegistry()
		mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
		g, _ := governor.New(governor.Config{Registry: r, Bus: b, RateLimitPerMin: 1})
		// First call admitted; second exceeds the 1/min gate.
		_, _ = g.Complete(context.Background(), agent.CompletionRequest{Model: "m", CorrelationID: "run-first"})
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "m", CorrelationID: corr}); !errors.Is(err, governor.ErrRateLimited) {
			t.Fatalf("second call err = %v, want ErrRateLimited", err)
		}
		assertCorr(t, j, event.KindRateLimited, corr)
	})

	t.Run("budget.exceeded", func(t *testing.T) {
		b, j := newBus(t)
		prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1_000_000, 0)}
		r := governor.NewRegistry()
		mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
		g, _ := governor.New(governor.Config{Registry: r, Bus: b, DailyCeilingMicrocents: 200_000})
		// First call spends past the ceiling; second is blocked pre-flight.
		_, _ = g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6", CorrelationID: "run-first"})
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6", CorrelationID: corr}); !errors.Is(err, governor.ErrBudgetExceeded) {
			t.Fatalf("second call err = %v, want ErrBudgetExceeded", err)
		}
		// The blocked (second) call's event is the one carrying corr; it is the
		// most recent budget.exceeded, so scan for an event with corr specifically.
		assertCorrPresent(t, j, event.KindBudgetExceeded, corr)
	})

	t.Run("capability.rerouted", func(t *testing.T) {
		b, j := newBus(t)
		r := governor.NewRegistry()
		prov := &recordingProvider{name: "p", resp: okResp("big", 1, 1)}
		mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
		g, _ := governor.New(governor.Config{
			Registry: r, Bus: b, DownRouteToolModels: true,
			ModelToolCapable:       capLookup(map[string]bool{"mini": false, "big": true}),
			ToolCapableAlternative: altLookup(map[string]string{"mini": "big"}),
		})
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{
			Model: "mini", Tools: []agent.ToolDef{{Name: "shell"}}, CorrelationID: corr,
		}); err != nil {
			t.Fatalf("Complete: %v", err)
		}
		assertCorr(t, j, event.KindCapabilityRerouted, corr)
	})

	t.Run("capability.rejected", func(t *testing.T) {
		b, j := newBus(t)
		r := governor.NewRegistry()
		prov := &fakeProvider{name: "p", resp: okResp("mini", 1, 1)}
		mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
		g, _ := governor.New(governor.Config{
			Registry: r, Bus: b, StrictModelCapabilities: true,
			ModelToolCapable: capLookup(map[string]bool{"mini": false}),
		})
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{
			Model: "mini", Tools: []agent.ToolDef{{Name: "shell"}}, CorrelationID: corr,
		}); !errors.Is(err, governor.ErrModelLacksToolUse) {
			t.Fatalf("err = %v, want ErrModelLacksToolUse", err)
		}
		assertCorr(t, j, event.KindCapabilityRejected, corr)
	})
}

// assertCorr asserts the FIRST event of kind carries exactly corr.
func assertCorr(t *testing.T, j *journal.Journal, kind event.Kind, corr string) {
	t.Helper()
	ev := firstOfKind(j, kind)
	if ev == nil {
		t.Fatalf("no %s event journaled", kind)
	}
	if ev.CorrelationID != corr {
		t.Errorf("%s CorrelationID = %q, want %q (event orphaned from its run)", kind, ev.CorrelationID, corr)
	}
}

// assertCorrPresent asserts SOME event of kind carries corr (when earlier calls
// emit the same kind under a different correlation).
func assertCorrPresent(t *testing.T, j *journal.Journal, kind event.Kind, corr string) {
	t.Helper()
	found := false
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == kind && e.CorrelationID == corr {
			found = true
		}
		return nil
	})
	if !found {
		t.Errorf("no %s event carried correlation %q (event orphaned from its run)", kind, corr)
	}
}
