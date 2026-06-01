// SPDX-License-Identifier: MIT

package governor_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/journal"
)

// ---- helpers ----

func newBus(t *testing.T) (*bus.Bus, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	return b, j
}

type fakeProvider struct {
	name  string
	resp  *agent.CompletionResponse
	err   error
	calls atomic.Int64
}

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.calls.Add(1)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.err != nil {
		return nil, p.err
	}
	return p.resp, nil
}

func okResp(model string, in, out int) *agent.CompletionResponse {
	return &agent.CompletionResponse{
		Message:    agent.Message{Role: agent.RoleAssistant, Content: "ok from " + model},
		StopReason: agent.StopEndTurn,
		Usage:      agent.Usage{InputTokens: in, OutputTokens: out, Model: model},
	}
}

func mustRegister(t *testing.T, r *governor.Registry, infos ...*governor.ProviderInfo) {
	t.Helper()
	for _, info := range infos {
		if err := r.Register(info); err != nil {
			t.Fatalf("Register %s: %v", info.Name, err)
		}
	}
}

// ---- registry ----

func TestRegistry_Register_And_Get(t *testing.T) {
	r := governor.NewRegistry()
	fp := &fakeProvider{name: "a"}
	mustRegister(t, r, &governor.ProviderInfo{Name: "a", Provider: fp, AuthMode: governor.AuthAPIKey})
	got, ok := r.Get("a")
	if !ok || got.Provider != fp {
		t.Errorf("Get returned %+v ok=%v", got, ok)
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("Get should miss")
	}
}

func TestRegistry_RejectsDuplicate(t *testing.T) {
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "a", Provider: &fakeProvider{name: "a"}})
	err := r.Register(&governor.ProviderInfo{Name: "a", Provider: &fakeProvider{name: "a"}})
	if !errors.Is(err, governor.ErrAlreadyRegistered) {
		t.Errorf("got err=%v want ErrAlreadyRegistered", err)
	}
}

func TestRegistry_NameMismatch(t *testing.T) {
	r := governor.NewRegistry()
	err := r.Register(&governor.ProviderInfo{Name: "claimed", Provider: &fakeProvider{name: "actual"}})
	if err == nil {
		t.Fatal("expected name mismatch error")
	}
}

// ---- governor routing ----

func TestComplete_HappyPath_RecordsUsage(t *testing.T) {
	b, j := newBus(t)
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{
		Name: "fake", Provider: &fakeProvider{name: "fake", resp: okResp("claude-sonnet-4-6", 1_000, 500)},
		AuthMode: governor.AuthAPIKey,
	})
	g, err := governor.New(governor.Config{Registry: r, Bus: b})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "claude-sonnet-4-6",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Message.Content != "ok from claude-sonnet-4-6" {
		t.Errorf("content=%q", resp.Message.Content)
	}

	// Spend = 1000 input × 300_000_000 mc/MTok + 500 × 1_500_000_000 mc/MTok
	// = (3e11 + 7.5e11) / 1e6 = 1_050_000 microcents = $0.105
	wantMC := int64(1_050_000)
	if got := g.SpentMicrocents(); got != wantMC {
		t.Errorf("spent=%d want %d", got, wantMC)
	}

	// Journal should contain a routing.decision and a budget.consumed.
	var sawRoute, sawBudget bool
	_ = j.Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindRoutingDecision:
			sawRoute = true
		case event.KindBudgetConsumed:
			sawBudget = true
		}
		return nil
	})
	if !sawRoute {
		t.Error("missing routing.decision event")
	}
	if !sawBudget {
		t.Error("missing budget.consumed event")
	}
}

func TestComplete_BudgetConsumedCarriesCorrelation(t *testing.T) {
	// M47: the request's CorrelationID is stamped onto the budget.consumed
	// event (envelope + payload) so spend can be attributed to its run.
	b, j := newBus(t)
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{
		Name: "fake", Provider: &fakeProvider{name: "fake", resp: okResp("claude-sonnet-4-6", 1_000, 500)},
		AuthMode: governor.AuthAPIKey,
	})
	g, err := governor.New(governor.Config{Registry: r, Bus: b})
	if err != nil {
		t.Fatal(err)
	}

	const corr = "run-SPEND-1"
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model:         "claude-sonnet-4-6",
		CorrelationID: corr,
	}); err != nil {
		t.Fatal(err)
	}

	var found *event.Event
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindBudgetConsumed {
			found = e
		}
		return nil
	})
	if found == nil {
		t.Fatal("no budget.consumed event journaled")
	}
	if found.CorrelationID != corr {
		t.Errorf("budget.consumed envelope correlation = %q, want %q", found.CorrelationID, corr)
	}
	var pl struct {
		Correlation string `json:"correlation_id"`
		Cost        int64  `json:"cost_microcents"`
	}
	if err := json.Unmarshal(found.Payload, &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Correlation != corr {
		t.Errorf("budget.consumed payload correlation_id = %q, want %q", pl.Correlation, corr)
	}
	if pl.Cost <= 0 {
		t.Errorf("budget.consumed cost_microcents = %d, want > 0", pl.Cost)
	}
}

func TestComplete_FallbackChain(t *testing.T) {
	b, j := newBus(t)
	bad1 := &fakeProvider{name: "bad1", err: errors.New("upstream 503")}
	bad2 := &fakeProvider{name: "bad2", err: errors.New("upstream 429")}
	good := &fakeProvider{name: "local", resp: okResp("llama3.2", 5, 5)}

	r := governor.NewRegistry()
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "bad1", Provider: bad1, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "bad2", Provider: bad2, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "local", Provider: good, AuthMode: governor.AuthLocal, IsFallback: true},
	)
	g, _ := governor.New(governor.Config{Registry: r, Bus: b})

	resp, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "llama3.2"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Message.Content != "ok from llama3.2" {
		t.Errorf("ended up on %q want llama3.2", resp.Message.Content)
	}
	if bad1.calls.Load() != 1 || bad2.calls.Load() != 1 || good.calls.Load() != 1 {
		t.Errorf("call counts: bad1=%d bad2=%d good=%d", bad1.calls.Load(), bad2.calls.Load(), good.calls.Load())
	}

	// Two provider.fallback events should fire (bad1→bad2, bad2→local).
	var fallbacks int
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindProviderFallback {
			fallbacks++
		}
		return nil
	})
	if fallbacks != 2 {
		t.Errorf("provider.fallback events=%d want 2", fallbacks)
	}
}

func TestComplete_FallbackOrder_PrimaryThenFallback(t *testing.T) {
	// IsFallback providers must be tried LAST regardless of registration order.
	b, _ := newBus(t)
	primary := &fakeProvider{name: "primary", err: errors.New("boom")}
	floor := &fakeProvider{name: "floor", resp: okResp("llama3.2", 0, 0)}
	r := governor.NewRegistry()
	// Register floor FIRST to test that IsFallback still defers it.
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "floor", Provider: floor, AuthMode: governor.AuthLocal, IsFallback: true},
		&governor.ProviderInfo{Name: "primary", Provider: primary, AuthMode: governor.AuthAPIKey},
	)
	g, _ := governor.New(governor.Config{Registry: r, Bus: b})

	_, err := g.Complete(context.Background(), agent.CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if primary.calls.Load() != 1 {
		t.Errorf("primary calls=%d want 1", primary.calls.Load())
	}
	if floor.calls.Load() != 1 {
		t.Errorf("floor calls=%d want 1 (called only after primary failed)", floor.calls.Load())
	}
}

func TestComplete_AllFail_ReturnsErrNoProviders(t *testing.T) {
	b, _ := newBus(t)
	p1 := &fakeProvider{name: "p1", err: errors.New("e1")}
	p2 := &fakeProvider{name: "p2", err: errors.New("e2")}
	r := governor.NewRegistry()
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "p1", Provider: p1, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "p2", Provider: p2, AuthMode: governor.AuthAPIKey},
	)
	g, _ := governor.New(governor.Config{Registry: r, Bus: b})

	_, err := g.Complete(context.Background(), agent.CompletionRequest{})
	var e *governor.ErrNoProviders
	if !errors.As(err, &e) {
		t.Fatalf("got %v, want *ErrNoProviders", err)
	}
	if len(e.Tried) != 2 {
		t.Errorf("Tried=%v want 2 entries", e.Tried)
	}
}

func TestComplete_CtxCancel_NoFallback(t *testing.T) {
	// Cancel-class errors must NOT cascade to fallback (user halted).
	b, _ := newBus(t)
	primary := &fakeProvider{name: "primary", err: context.Canceled}
	floor := &fakeProvider{name: "floor", resp: okResp("llama3.2", 0, 0)}
	r := governor.NewRegistry()
	mustRegister(t, r,
		&governor.ProviderInfo{Name: "primary", Provider: primary, AuthMode: governor.AuthAPIKey},
		&governor.ProviderInfo{Name: "floor", Provider: floor, AuthMode: governor.AuthLocal, IsFallback: true},
	)
	g, _ := governor.New(governor.Config{Registry: r, Bus: b})

	_, err := g.Complete(context.Background(), agent.CompletionRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context.Canceled", err)
	}
	if floor.calls.Load() != 0 {
		t.Errorf("floor was tried after cancel; calls=%d", floor.calls.Load())
	}
}

// ---- budget ----

func TestBudgetCeiling_RefusesNewCalls(t *testing.T) {
	b, j := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1_000_000, 0)} // costs ~$0.30 per call
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{
		Registry:               r,
		Bus:                    b,
		DailyCeilingMicrocents: 200_000, // $0.020 — first call blows past it
	})

	// First call succeeds (spend happens after the call).
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// Second call is blocked at the pre-check.
	_, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"})
	if !errors.Is(err, governor.ErrBudgetExceeded) {
		t.Errorf("second call: got %v, want ErrBudgetExceeded", err)
	}
	if prov.calls.Load() != 1 {
		t.Errorf("provider was called %d times; should be 1 (2nd blocked pre-call)", prov.calls.Load())
	}

	// budget.exceeded should be in the journal.
	var exceeded bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindBudgetExceeded {
			exceeded = true
		}
		return nil
	})
	if !exceeded {
		t.Error("missing budget.exceeded event")
	}
}

// A sibling governor from WithDailyCeiling shares the provider pool but keeps
// its OWN spend ledger and ceiling: exhausting the sibling's budget must not
// touch the parent's headroom, and vice versa (the M14 per-tenant quota seam).
func TestWithDailyCeiling_IndependentLedgers(t *testing.T) {
	b, _ := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1_000_000, 0)} // ~$0.30/call
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})

	parent, err := governor.New(governor.Config{
		Registry: r, Bus: b,
		DailyCeilingMicrocents: 100_000_000, // generous; parent never blocks here
	})
	if err != nil {
		t.Fatal(err)
	}

	// Sibling with a tiny ceiling: $0.020 — its second call must be blocked.
	tenant, err := parent.WithDailyCeiling(200_000)
	if err != nil {
		t.Fatal(err)
	}

	// Sibling: first call ok, second blocked at the pre-check.
	if _, err := tenant.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Fatalf("tenant first call: %v", err)
	}
	if _, err := tenant.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); !errors.Is(err, governor.ErrBudgetExceeded) {
		t.Errorf("tenant second call: got %v, want ErrBudgetExceeded", err)
	}

	// Parent's ledger is untouched by the sibling's spend — it can still run.
	if parent.SpentMicrocents() != 0 {
		t.Errorf("parent ledger = %d, want 0 (sibling spend must not bleed in)", parent.SpentMicrocents())
	}
	if _, err := parent.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Errorf("parent call after sibling exhausted: %v (parent must keep its headroom)", err)
	}
	// And the sibling's own ceiling is what we set, not the parent's.
	if got := tenant.DailyCeilingMicrocents(); got != 200_000 {
		t.Errorf("sibling ceiling = %d, want 200000", got)
	}
}

func TestRateLimit_PerMinuteWindow(t *testing.T) {
	b, j := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1, 1)}
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthLocal})

	clock := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	g, _ := governor.New(governor.Config{
		Registry: r, Bus: b,
		RateLimitPerMin: 2,
		Now:             func() time.Time { return clock },
	})

	// First two calls in the 10:30 window are admitted.
	for i := 0; i < 2; i++ {
		if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
			t.Fatalf("call %d should be admitted: %v", i+1, err)
		}
	}
	// Third in the same minute is rejected (provider not called a 3rd time).
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); !errors.Is(err, governor.ErrRateLimited) {
		t.Errorf("3rd call: got %v, want ErrRateLimited", err)
	}
	if prov.calls.Load() != 2 {
		t.Errorf("provider called %d times; want 2 (3rd blocked pre-call)", prov.calls.Load())
	}
	// rate.limited event is journaled.
	var limited bool
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindRateLimited {
			limited = true
		}
		return nil
	})
	if !limited {
		t.Error("missing rate.limited event")
	}

	// Advance into the next clock-minute: the window resets, calls admitted again.
	clock = clock.Add(time.Minute)
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Errorf("call after window rollover should be admitted: %v", err)
	}
	if prov.calls.Load() != 3 {
		t.Errorf("provider calls = %d, want 3 after rollover", prov.calls.Load())
	}
}

// WithLimits gives a sibling its own rate window as well as its own ledger:
// throttling the sibling must not throttle the parent.
func TestWithLimits_IndependentRateWindows(t *testing.T) {
	b, _ := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1, 1)}
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthLocal})

	clock := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	parent, _ := governor.New(governor.Config{
		Registry: r, Bus: b,
		Now: func() time.Time { return clock }, // no rate cap on the parent
	})
	tenant, _ := parent.WithLimits(0, 1) // 1 call/min for the tenant
	// Tenant's single allowance is consumed, second is throttled.
	if _, err := tenant.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Fatalf("tenant first call: %v", err)
	}
	if _, err := tenant.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); !errors.Is(err, governor.ErrRateLimited) {
		t.Errorf("tenant second call: got %v, want ErrRateLimited", err)
	}
	// Parent (no cap) is unaffected and keeps running.
	for i := 0; i < 5; i++ {
		if _, err := parent.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
			t.Errorf("parent call %d should not be throttled by tenant: %v", i+1, err)
		}
	}
}

func TestBudgetRollover_NewUTCDay(t *testing.T) {
	b, _ := newBus(t)
	prov := &fakeProvider{name: "p", resp: okResp("claude-sonnet-4-6", 1_000_000, 0)}
	r := governor.NewRegistry()
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})

	clock := time.Date(2026, 1, 1, 23, 59, 0, 0, time.UTC)
	g, _ := governor.New(governor.Config{
		Registry: r, Bus: b,
		DailyCeilingMicrocents: 1_000_000, // small but >= one call's cost
		Now:                    func() time.Time { return clock },
	})

	// First call on Jan 1.
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "claude-sonnet-4-6"}); err != nil {
		t.Fatal(err)
	}
	jan1Spent := g.SpentMicrocents()
	if jan1Spent <= 0 {
		t.Fatalf("expected nonzero spend, got %d", jan1Spent)
	}

	// Roll the clock to Jan 2 and confirm the counter resets.
	clock = time.Date(2026, 1, 2, 0, 5, 0, 0, time.UTC)
	if got := g.SpentMicrocents(); got != 0 {
		t.Errorf("after rollover spend=%d want 0", got)
	}
}

// ---- pricing ----

func TestPricing_KnownModelHasCost(t *testing.T) {
	c := mustParseInt(t, fmt.Sprintf("%d", costMicrocentsForTest("claude-sonnet-4-6", 1_000_000, 1_000_000)))
	// 1 MTok input + 1 MTok output:
	// (1_000_000 * 300_000_000 + 1_000_000 * 1_500_000_000) / 1_000_000 = 1_800_000_000 microcents = $18
	want := int64(1_800_000_000)
	if c != want {
		t.Errorf("cost=%d want %d", c, want)
	}
}

func TestPricing_UnknownModelIsFree(t *testing.T) {
	c := costMicrocentsForTest("some-unknown-model-9000", 999_999, 999_999)
	if c != 0 {
		t.Errorf("unknown model cost=%d want 0", c)
	}
}

// costMicrocentsForTest invokes the package's internal pricing path via a
// recorded run since the symbol is package-private.
func costMicrocentsForTest(model string, in, out int) int64 {
	// We exercise pricing through a real Complete by reading the
	// budget.consumed event payload — keeps the test honest about the
	// public observable behaviour.
	b, j := setupBusForCostExtraction()
	prov := &fakeProvider{name: "p", resp: okResp(model, in, out)}
	r := governor.NewRegistry()
	_ = r.Register(&governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{Registry: r, Bus: b})
	_, _ = g.Complete(context.Background(), agent.CompletionRequest{Model: model})
	var cost int64
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindBudgetConsumed {
			var p struct {
				CostMicrocents int64 `json:"cost_microcents"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			cost = p.CostMicrocents
		}
		return nil
	})
	return cost
}

// setupBusForCostExtraction is a lighter helper than newBus that we can use
// in non-testing.T contexts (the pricing helpers above).
func setupBusForCostExtraction() (*bus.Bus, *journal.Journal) {
	dir, err := tempDir()
	if err != nil {
		panic(err)
	}
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		panic(err)
	}
	return bus.New(j), j
}

// tempDir creates a unique temp dir without needing a testing.T.
func tempDir() (string, error) {
	d, err := osMkdirTemp("", "gov-test-*")
	return d, err
}

// Indirection so this test file compiles without importing os twice.
var osMkdirTemp = func(dir, pattern string) (string, error) { return osMkdirTempFn(dir, pattern) }

// ---- misc ----

func mustParseInt(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	_, err := fmt.Sscan(s, &n)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return n
}

// ---- catalog-driven pricing (M1.f) ----

func TestPricing_CatalogOverridesFallbackTable(t *testing.T) {
	// Install a tiny catalog with a brand-new model not in the fallback
	// table, plus an override of a model that IS in the fallback table.
	cat, err := catalog.ParseAPIFile([]byte(`{
      "anthropic": {
        "id": "anthropic", "npm": "@ai-sdk/anthropic",
        "models": {
          "claude-opus-4-7": {"id":"claude-opus-4-7","cost":{"input":1,"output":2}},
          "brand-new-model": {"id":"brand-new-model","cost":{"input":42,"output":84}}
        }
      }
    }`))
	if err != nil {
		t.Fatalf("ParseAPIFile: %v", err)
	}
	governor.SetCatalog(cat)
	t.Cleanup(func() { governor.SetCatalog(nil) })

	// Existing model: catalog wins over fallback table.
	if got := costMicrocentsForTest("claude-opus-4-7", 1_000_000, 1_000_000); got != 3_000_000_000 {
		// 1*1e9 + 2*1e9 = 3e9 microcents at 1 MTok each.
		t.Errorf("claude-opus-4-7 catalog override: got %d want 3_000_000_000", got)
	}
	// New model: only the catalog knows about it.
	if got := costMicrocentsForTest("brand-new-model", 1_000_000, 1_000_000); got != 126_000_000_000 {
		// 42*1e9 + 84*1e9 = 126e9
		t.Errorf("brand-new-model: got %d want 126_000_000_000", got)
	}
}

func TestPricing_CatalogMissingFallsBackToTable(t *testing.T) {
	// Empty catalog → must still find fallback-table prices.
	governor.SetCatalog(catalog.NewEmpty())
	t.Cleanup(func() { governor.SetCatalog(nil) })

	if got := costMicrocentsForTest("claude-sonnet-4-6", 1_000_000, 1_000_000); got != 1_800_000_000 {
		// Fallback table: 300_000_000 + 1_500_000_000 = 1.8e9 at 1 MTok each.
		t.Errorf("fallback miss for known model: got %d", got)
	}
}

// ---- Registry.Replace / Remove / Governor.Replace ----

func TestRegistry_ReplaceSwapsEntry(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "p1", Provider: &fakeProvider{name: "p1"}, AuthMode: governor.AuthAPIKey,
	}); err != nil {
		t.Fatal(err)
	}
	newProv := &fakeProvider{name: "p1", resp: okResp("p1", 1, 1)}
	if err := reg.Replace(&governor.ProviderInfo{
		Name: "p1", Provider: newProv, AuthMode: governor.AuthSubscription,
	}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, ok := reg.Get("p1")
	if !ok {
		t.Fatal("entry gone after Replace")
	}
	if got.AuthMode != governor.AuthSubscription {
		t.Errorf("AuthMode = %q, want subscription", got.AuthMode)
	}
	// Order preserved when replacing in place.
	names := reg.Names()
	if len(names) != 1 || names[0] != "p1" {
		t.Errorf("order disturbed: %v", names)
	}
}

func TestRegistry_ReplaceNonExistentAppends(t *testing.T) {
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name: "p1", Provider: &fakeProvider{name: "p1"},
	}); err != nil {
		t.Fatal(err)
	}
	// Replace on a brand-new name acts like Register.
	if err := reg.Replace(&governor.ProviderInfo{
		Name: "p2", Provider: &fakeProvider{name: "p2"},
	}); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	all := reg.All()
	if len(all) != 2 {
		t.Fatalf("want 2 entries, got %d", len(all))
	}
	if all[0].Name != "p1" || all[1].Name != "p2" {
		t.Errorf("order wrong: %v / %v", all[0].Name, all[1].Name)
	}
}

func TestRegistry_RemoveDropsAndPreservesOrder(t *testing.T) {
	reg := governor.NewRegistry()
	for _, n := range []string{"a", "b", "c"} {
		_ = reg.Register(&governor.ProviderInfo{Name: n, Provider: &fakeProvider{name: n}})
	}
	if !reg.Remove("b") {
		t.Error("Remove returned false for existing entry")
	}
	if reg.Remove("b") {
		t.Error("Remove returned true for already-removed entry")
	}
	all := reg.All()
	if len(all) != 2 || all[0].Name != "a" || all[1].Name != "c" {
		t.Errorf("order broken after Remove: %v", []string{all[0].Name, all[1].Name})
	}
}

func TestGovernor_ReplaceRoutesToNewProvider(t *testing.T) {
	// Locks in M1.r's invariant: Governor.Replace MUST swap the cached
	// routing chain — direct Registry.Replace alone would leak the old
	// provider into Complete until daemon restart.
	reg := governor.NewRegistry()
	old := &fakeProvider{name: "primary", resp: okResp("primary-old", 1, 1)}
	if err := reg.Register(&governor.ProviderInfo{
		Name: "primary", Provider: old, AuthMode: governor.AuthAPIKey,
	}); err != nil {
		t.Fatal(err)
	}
	gov, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, _ := newBus(t)
	gov.SetBus(b)

	// First call hits the old provider.
	_, _ = gov.Complete(context.Background(), agent.CompletionRequest{Model: "x"})
	if old.calls.Load() != 1 {
		t.Errorf("old.calls = %d, want 1", old.calls.Load())
	}

	// Swap in a new provider under the same name. Old provider should
	// stop receiving calls; new one starts receiving them.
	fresh := &fakeProvider{name: "primary", resp: okResp("primary-fresh", 1, 1)}
	if err := gov.Replace(&governor.ProviderInfo{
		Name: "primary", Provider: fresh, AuthMode: governor.AuthSubscription,
	}); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	_, _ = gov.Complete(context.Background(), agent.CompletionRequest{Model: "x"})
	if old.calls.Load() != 1 {
		t.Errorf("old.calls = %d, want 1 (Governor leaked old provider after Replace)", old.calls.Load())
	}
	if fresh.calls.Load() != 1 {
		t.Errorf("fresh.calls = %d, want 1 (Governor did not adopt new provider)", fresh.calls.Load())
	}
}

// ---- Subscription-first routing (DECISIONS C2) ----

func TestGovernor_RoutesSubscriptionBeforeAPIKey(t *testing.T) {
	// Register an API-key provider first, then a subscription provider.
	// Insertion order would put the API-key first; subscription-first
	// routing must override that.
	reg := governor.NewRegistry()
	apiProv := &fakeProvider{name: "paid", resp: okResp("paid", 1, 1)}
	subProv := &fakeProvider{name: "sub", resp: okResp("sub", 1, 1)}
	_ = reg.Register(&governor.ProviderInfo{Name: "paid", Provider: apiProv, AuthMode: governor.AuthAPIKey})
	_ = reg.Register(&governor.ProviderInfo{Name: "sub", Provider: subProv, AuthMode: governor.AuthSubscription})
	gov, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, _ := newBus(t)
	gov.SetBus(b)

	_, _ = gov.Complete(context.Background(), agent.CompletionRequest{Model: "x"})
	if subProv.calls.Load() != 1 {
		t.Errorf("subscription provider should be tried first (calls=%d)", subProv.calls.Load())
	}
	if apiProv.calls.Load() != 0 {
		t.Errorf("api-key provider should not have been called (calls=%d) — subscription succeeded", apiProv.calls.Load())
	}
}

func TestGovernor_RoutesLocalAheadOfAPIKeyButBehindSubscription(t *testing.T) {
	// Three providers, registered in worst-first order, each set to
	// fail so Complete walks the entire chain — and the order of
	// failures reveals the actual chain order.
	reg := governor.NewRegistry()
	api := &fakeProvider{name: "api", err: errors.New("api boom")}
	loc := &fakeProvider{name: "loc", err: errors.New("loc boom")}
	sub := &fakeProvider{name: "sub", err: errors.New("sub boom")}
	_ = reg.Register(&governor.ProviderInfo{Name: "api", Provider: api, AuthMode: governor.AuthAPIKey})
	_ = reg.Register(&governor.ProviderInfo{Name: "loc", Provider: loc, AuthMode: governor.AuthLocal})
	_ = reg.Register(&governor.ProviderInfo{Name: "sub", Provider: sub, AuthMode: governor.AuthSubscription})

	gov, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, _ := newBus(t)
	gov.SetBus(b)

	_, err = gov.Complete(context.Background(), agent.CompletionRequest{Model: "x"})
	if err == nil {
		t.Fatal("expected ErrNoProviders when all fail")
	}
	// Tried in subscription → local → api order.
	if sub.calls.Load() != 1 || loc.calls.Load() != 1 || api.calls.Load() != 1 {
		t.Errorf("each provider should be tried once: sub=%d loc=%d api=%d",
			sub.calls.Load(), loc.calls.Load(), api.calls.Load())
	}
	// The error must mention them in the chosen chain order so
	// operators can read the failure log and trust the routing.
	var npe *governor.ErrNoProviders
	if !errors.As(err, &npe) {
		t.Fatalf("expected *ErrNoProviders, got %T: %v", err, err)
	}
	if len(npe.Tried) != 3 {
		t.Fatalf("Tried list size=%d want 3", len(npe.Tried))
	}
	if npe.Tried[0] != "sub" || npe.Tried[1] != "loc" || npe.Tried[2] != "api" {
		t.Errorf("Tried order=%v, want [sub loc api]", npe.Tried)
	}
}

func TestGovernor_StableSortWithinSameTier(t *testing.T) {
	// Two api-key providers registered in order a, b. They must be
	// tried in registration order — subscription-first only reorders
	// across tiers, not within them.
	reg := governor.NewRegistry()
	a := &fakeProvider{name: "a", err: errors.New("a boom")}
	b := &fakeProvider{name: "b", resp: okResp("b", 1, 1)}
	_ = reg.Register(&governor.ProviderInfo{Name: "a", Provider: a, AuthMode: governor.AuthAPIKey})
	_ = reg.Register(&governor.ProviderInfo{Name: "b", Provider: b, AuthMode: governor.AuthAPIKey})
	gov, err := governor.New(governor.Config{Registry: reg})
	if err != nil {
		t.Fatal(err)
	}
	bus2, _ := newBus(t)
	gov.SetBus(bus2)
	_, err = gov.Complete(context.Background(), agent.CompletionRequest{Model: "x"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if a.calls.Load() != 1 {
		t.Errorf("a should be tried first (calls=%d)", a.calls.Load())
	}
	if b.calls.Load() != 1 {
		t.Errorf("b should be tried after a fails (calls=%d)", b.calls.Load())
	}
}

// capLookup builds a ModelToolCapable func from a fixed map of model→toolcap;
// any model absent from the map reports known=false.
func capLookup(m map[string]bool) func(string) (bool, bool) {
	return func(model string) (bool, bool) {
		c, ok := m[model]
		return c, ok
	}
}

func TestStrictCapabilities_RejectsToolsOnNonToolModel(t *testing.T) {
	b, j := newBus(t)
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("mini", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, err := governor.New(governor.Config{
		Registry:                r,
		Bus:                     b,
		StrictModelCapabilities: true,
		ModelToolCapable:        capLookup(map[string]bool{"mini": false}),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Complete(context.Background(), agent.CompletionRequest{
		Model: "mini",
		Tools: []agent.ToolDef{{Name: "shell"}},
	})
	if !errors.Is(err, governor.ErrModelLacksToolUse) {
		t.Fatalf("err = %v, want ErrModelLacksToolUse", err)
	}
	// The provider must NOT have been called (pre-flight reject).
	if prov.calls.Load() != 0 {
		t.Errorf("provider was called %d times; expected 0 (pre-flight reject)", prov.calls.Load())
	}
	// A capability.rejected event must be journaled.
	found := false
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindCapabilityRejected {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("expected a capability.rejected event in the journal")
	}
}

// recordingProvider captures the req.Model it was called with, so a test
// can assert capability down-routing remapped the model (M37).
type recordingProvider struct {
	name     string
	resp     *agent.CompletionResponse
	gotModel string
	calls    atomic.Int64
}

func (p *recordingProvider) Name() string { return p.name }
func (p *recordingProvider) Complete(_ context.Context, req agent.CompletionRequest) (*agent.CompletionResponse, error) {
	p.gotModel = req.Model
	p.calls.Add(1)
	return p.resp, nil
}

func altLookup(m map[string]string) func(string) (string, bool) {
	return func(model string) (string, bool) {
		a, ok := m[model]
		return a, ok
	}
}

// TestDownRoute_RemapsToolIncapableModel — with down-routing on, a tools
// request to a tool-incapable model is remapped to a capable alternative,
// the provider sees the new model, and a capability.rerouted event is
// journaled (M37).
func TestDownRoute_RemapsToolIncapableModel(t *testing.T) {
	b, j := newBus(t)
	r := governor.NewRegistry()
	prov := &recordingProvider{name: "p", resp: okResp("big", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, err := governor.New(governor.Config{
		Registry:               r,
		Bus:                    b,
		DownRouteToolModels:    true,
		ModelToolCapable:       capLookup(map[string]bool{"mini": false, "big": true}),
		ToolCapableAlternative: altLookup(map[string]string{"mini": "big"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "mini", Tools: []agent.ToolDef{{Name: "shell"}},
	}); err != nil {
		t.Fatalf("down-route should let the request proceed; got %v", err)
	}
	if prov.gotModel != "big" {
		t.Errorf("provider saw model %q, want remapped %q", prov.gotModel, "big")
	}
	var from, to string
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindCapabilityRerouted {
			var p struct {
				From string `json:"from_model"`
				To   string `json:"to_model"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			from, to = p.From, p.To
		}
		return nil
	})
	if from != "mini" || to != "big" {
		t.Errorf("capability.rerouted = %q→%q, want mini→big", from, to)
	}
}

// TestDownRoute_NoAlternativeFallsThroughToStrict — down-routing + strict
// together: when no alternative exists, the request falls through to the
// strict gate and is rejected (M37 composes with M25).
func TestDownRoute_NoAlternativeFallsThroughToStrict(t *testing.T) {
	r := governor.NewRegistry()
	prov := &recordingProvider{name: "p", resp: okResp("mini", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{
		Registry:                r,
		StrictModelCapabilities: true,
		DownRouteToolModels:     true,
		ModelToolCapable:        capLookup(map[string]bool{"mini": false}),
		ToolCapableAlternative:  altLookup(map[string]string{}), // no alternative
	})
	_, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "mini", Tools: []agent.ToolDef{{Name: "shell"}},
	})
	if !errors.Is(err, governor.ErrModelLacksToolUse) {
		t.Fatalf("err = %v, want ErrModelLacksToolUse (no alt → strict reject)", err)
	}
	if prov.calls.Load() != 0 {
		t.Errorf("provider called %d times; want 0 (rejected pre-flight)", prov.calls.Load())
	}
}

// TestDownRoute_OffLeavesModelUnchanged — with down-routing off, the model
// is not remapped (the provider sees the original, incapable model).
func TestDownRoute_OffLeavesModelUnchanged(t *testing.T) {
	r := governor.NewRegistry()
	prov := &recordingProvider{name: "p", resp: okResp("mini", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{
		Registry:               r,
		ModelToolCapable:       capLookup(map[string]bool{"mini": false, "big": true}),
		ToolCapableAlternative: altLookup(map[string]string{"mini": "big"}),
		// DownRouteToolModels not set → off.
	})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "mini", Tools: []agent.ToolDef{{Name: "shell"}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if prov.gotModel != "mini" {
		t.Errorf("provider saw model %q, want unchanged %q (down-route off)", prov.gotModel, "mini")
	}
}

func TestStrictCapabilities_AllowsWhenNoTools(t *testing.T) {
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("mini", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{
		Registry:                r,
		StrictModelCapabilities: true,
		ModelToolCapable:        capLookup(map[string]bool{"mini": false}),
	})
	// No tools in the request → the gate doesn't apply.
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{Model: "mini"}); err != nil {
		t.Errorf("non-tool request should pass on a non-tool model; got %v", err)
	}
}

func TestStrictCapabilities_UnknownModelNotBlocked(t *testing.T) {
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("who", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{
		Registry:                r,
		StrictModelCapabilities: true,
		ModelToolCapable:        capLookup(map[string]bool{"mini": false}), // "who" absent → unknown
	})
	// Unknown model must not be blocked even with tools (catalog-gap safety).
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "who", Tools: []agent.ToolDef{{Name: "shell"}},
	}); err != nil {
		t.Errorf("unknown model must not be blocked; got %v", err)
	}
}

func TestStrictCapabilities_OffByDefault(t *testing.T) {
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("mini", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	// Strict OFF: even a tools request to a non-tool model passes (advisory-only).
	g, _ := governor.New(governor.Config{
		Registry:         r,
		ModelToolCapable: capLookup(map[string]bool{"mini": false}),
	})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "mini", Tools: []agent.ToolDef{{Name: "shell"}},
	}); err != nil {
		t.Errorf("strict off: request should pass; got %v", err)
	}
}

func TestStrictCapabilities_AllowsToolCapableModel(t *testing.T) {
	r := governor.NewRegistry()
	prov := &fakeProvider{name: "p", resp: okResp("big", 1, 1)}
	mustRegister(t, r, &governor.ProviderInfo{Name: "p", Provider: prov, AuthMode: governor.AuthAPIKey})
	g, _ := governor.New(governor.Config{
		Registry:                r,
		StrictModelCapabilities: true,
		ModelToolCapable:        capLookup(map[string]bool{"big": true}),
	})
	if _, err := g.Complete(context.Background(), agent.CompletionRequest{
		Model: "big", Tools: []agent.ToolDef{{Name: "shell"}},
	}); err != nil {
		t.Errorf("tool-capable model should pass; got %v", err)
	}
}
