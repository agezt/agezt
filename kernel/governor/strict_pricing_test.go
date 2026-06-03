// SPDX-License-Identifier: MIT

package governor_test

// M193: optional strict-pricing gate. An unpriced model is charged $0 and
// bypasses the budget (fail-open) by default; with StrictPricing on, such a
// request is refused before any provider call, while known-free and priced
// models still pass.

import (
	"context"
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
)

func newStrictGov(t *testing.T, strict bool) (*governor.Governor, *fakeProvider, *journal.Journal) {
	t.Helper()
	b, j := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1, 1)}
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, err := governor.New(governor.Config{Registry: r, Bus: b, StrictPricing: strict})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return g, prov, j
}

func TestStrictPricing_RefusesUnknownModel(t *testing.T) {
	g, prov, j := newStrictGov(t, true)

	_, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "totally-unknown-model-xyz"})
	if !errors.Is(err, governor.ErrUnpricedModel) {
		t.Fatalf("err = %v, want ErrUnpricedModel", err)
	}
	if prov.calls.Load() != 0 {
		t.Errorf("provider called %d times; an unpriced model must be refused pre-call", prov.calls.Load())
	}
	// The refusal is auditable.
	var seen bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindBudgetUnpriced {
			seen = true
		}
		return nil
	})
	if !seen {
		t.Error("missing budget.unpriced event")
	}
}

func TestStrictPricing_AllowsKnownFreeAndPriced(t *testing.T) {
	for _, model := range []string{"claude-sonnet-4-6", "llama3.2"} {
		g, prov, _ := newStrictGov(t, true)
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: model}); err != nil {
			t.Errorf("model %q refused under strict pricing: %v", model, err)
		}
		if prov.calls.Load() != 1 {
			t.Errorf("model %q: provider called %d times, want 1", model, prov.calls.Load())
		}
	}
}

func TestStrictPricing_EmptyModelNotGated(t *testing.T) {
	g, prov, _ := newStrictGov(t, true)
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: ""}); err != nil {
		t.Errorf("empty model gated under strict pricing: %v", err)
	}
	if prov.calls.Load() != 1 {
		t.Errorf("empty model: provider called %d times, want 1", prov.calls.Load())
	}
}

func TestStrictPricing_OffByDefaultAllowsUnknown(t *testing.T) {
	g, prov, _ := newStrictGov(t, false)
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "totally-unknown-model-xyz"}); err != nil {
		t.Errorf("unknown model rejected with strict pricing OFF (regression): %v", err)
	}
	if prov.calls.Load() != 1 {
		t.Errorf("provider called %d times, want 1 (unknown model is charged $0 when not strict)", prov.calls.Load())
	}
}
