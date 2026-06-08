// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// makeGovernor builds a Governor wrapped around the mock provider
// with the given per-task caps. Pure test helper — mirrors what
// cmd/agezt's buildGovernor does at daemon start, minus the
// fallback registry and audit wiring (neither matters for the
// budget snapshot path).
func makeGovernor(t *testing.T, taskBudgets map[string]int64) *governor.Governor {
	t.Helper()
	reg := governor.NewRegistry()
	if err := reg.Register(&governor.ProviderInfo{
		Name:     "mock",
		Provider: mock.New(mock.FinalText("ok")),
		AuthMode: governor.AuthLocal,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	g, err := governor.New(governor.Config{
		Registry:               reg,
		DailyCeilingMicrocents: 1_000_000_000, // $1.00 — just a number
		TaskBudgets:            taskBudgets,
		Now:                    func() time.Time { return time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("governor.New: %v", err)
	}
	return g
}

// TestBudget_ReturnsSnapshotShape verifies the wire shape: utc_date,
// global counters, and a sorted per_task array with the expected
// fields. We don't spend any budget here — pure shape test (the
// per-task accounting logic itself is covered by
// kernel/governor/task_budget_test.go).
func TestBudget_ReturnsSnapshotShape(t *testing.T) {
	gov := makeGovernor(t, map[string]int64{
		"plan": 100_000_000, // $0.10
		"code": 500_000_000, // $0.50
	})
	_, _, c, _ := startPair(t, gov)

	res, err := c.Call(context.Background(), controlplane.CmdBudget, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got, _ := res["utc_date"].(string); got != "2026-05-29" {
		t.Errorf("utc_date = %q want 2026-05-29", got)
	}
	if got := mcOf(res["ceiling_mc"]); got != 1_000_000_000 {
		t.Errorf("ceiling_mc = %d want 1_000_000_000", got)
	}
	if got := mcOf(res["spent_mc"]); got != 0 {
		t.Errorf("spent_mc = %d want 0", got)
	}
	rows, ok := res["per_task"].([]any)
	if !ok {
		t.Fatalf("per_task wrong type: %T", res["per_task"])
	}
	if len(rows) != 2 {
		t.Fatalf("per_task len = %d want 2", len(rows))
	}
	// Sorted by task_type alphabetically: code, plan.
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	if first["task_type"] != "code" || second["task_type"] != "plan" {
		t.Errorf("per_task not sorted: %v / %v", first["task_type"], second["task_type"])
	}
	if mcOf(first["ceiling_mc"]) != 500_000_000 {
		t.Errorf("code cap = %d want 500_000_000", mcOf(first["ceiling_mc"]))
	}
	if mcOf(second["ceiling_mc"]) != 100_000_000 {
		t.Errorf("plan cap = %d want 100_000_000", mcOf(second["ceiling_mc"]))
	}
}

// TestBudget_NoCapsReturnsEmptyPerTask verifies that when no
// TaskBudgets are configured, per_task comes back as an empty
// array (not null) so the JSON shape stays stable for downstream
// jq pipelines.
func TestBudget_NoCapsReturnsEmptyPerTask(t *testing.T) {
	gov := makeGovernor(t, nil)
	_, _, c, _ := startPair(t, gov)

	res, err := c.Call(context.Background(), controlplane.CmdBudget, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rows, ok := res["per_task"].([]any)
	if !ok {
		t.Fatalf("per_task wrong type: %T (want []any even when empty)", res["per_task"])
	}
	if len(rows) != 0 {
		t.Errorf("per_task should be empty, got %d rows", len(rows))
	}
}

// TestBudget_NonGovernorProviderErrorsCleanly: when the daemon's
// provider isn't a *governor.Governor (test rigs using a bare
// mock), the handler returns a clear error rather than crashing.
// Defensive — production daemons always wrap their provider in
// a governor, but a test bypassing that should still get a
// useful response.
func TestBudget_NonGovernorProviderErrorsCleanly(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdBudget, nil)
	if err == nil {
		t.Fatal("expected error when provider isn't a governor")
	}
}

// TestBudgetSet_AdjustsCeilingAndReturnsSnapshot exercises the runtime
// "ayarla" knob (M607): CmdBudgetSet changes the global ceiling and returns the
// post-set snapshot, and a follow-up CmdBudget reads the new value back — proof
// the override is live for both enforcement and reporting.
func TestBudgetSet_AdjustsCeilingAndReturnsSnapshot(t *testing.T) {
	gov := makeGovernor(t, nil)
	_, _, c, _ := startPair(t, gov)

	// Raise to $2.50.
	res, err := c.Call(context.Background(), controlplane.CmdBudgetSet, map[string]any{"ceiling_mc": 2_500_000_000})
	if err != nil {
		t.Fatalf("budget_set: %v", err)
	}
	if got := mcOf(res["ceiling_mc"]); got != 2_500_000_000 {
		t.Errorf("post-set ceiling_mc = %d want 2_500_000_000", got)
	}
	// Read back via the independent read path.
	res2, err := c.Call(context.Background(), controlplane.CmdBudget, nil)
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if got := mcOf(res2["ceiling_mc"]); got != 2_500_000_000 {
		t.Errorf("read-back ceiling_mc = %d want 2_500_000_000", got)
	}
	if gov.DailyCeilingMicrocents() != 2_500_000_000 {
		t.Errorf("governor effective ceiling = %d want 2_500_000_000", gov.DailyCeilingMicrocents())
	}

	// Set to 0 (unlimited).
	if _, err := c.Call(context.Background(), controlplane.CmdBudgetSet, map[string]any{"ceiling_mc": 0}); err != nil {
		t.Fatalf("budget_set 0: %v", err)
	}
	if gov.DailyCeilingMicrocents() != 0 {
		t.Errorf("effective ceiling after set-0 = %d want 0 (unlimited)", gov.DailyCeilingMicrocents())
	}
}

// TestBudgetSet_AcceptsStringArg covers the Web UI path: the query-string write
// proxy forwards ceiling_mc as a STRING, so the handler must coerce it.
func TestBudgetSet_AcceptsStringArg(t *testing.T) {
	gov := makeGovernor(t, nil)
	_, _, c, _ := startPair(t, gov)
	res, err := c.Call(context.Background(), controlplane.CmdBudgetSet, map[string]any{"ceiling_mc": "750000000"})
	if err != nil {
		t.Fatalf("budget_set string arg: %v", err)
	}
	if got := mcOf(res["ceiling_mc"]); got != 750_000_000 {
		t.Errorf("ceiling_mc from string = %d want 750_000_000", got)
	}
}

// TestBudgetSet_RejectsMissingAndBadArgs: a missing or non-numeric ceiling_mc
// is a clear error, not a silent no-op or a panic.
func TestBudgetSet_RejectsMissingAndBadArgs(t *testing.T) {
	gov := makeGovernor(t, nil)
	_, _, c, _ := startPair(t, gov)
	if _, err := c.Call(context.Background(), controlplane.CmdBudgetSet, nil); err == nil {
		t.Error("expected error when ceiling_mc is absent")
	}
	if _, err := c.Call(context.Background(), controlplane.CmdBudgetSet, map[string]any{"ceiling_mc": "abc"}); err == nil {
		t.Error("expected error when ceiling_mc is non-numeric")
	}
}

// mcOf is a helper that accepts the float64 / int64 ambiguity of
// JSON-decoded numbers. Mirrors the cmd/agt/budget.go helper.
func mcOf(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

// Compile-time pin: agent.Provider is the surface startPair needs;
// governor.Governor satisfies it. If that breaks, this test stops
// compiling — a useful canary if the Governor interface drifts.
var _ agent.Provider = (*governor.Governor)(nil)
