// SPDX-License-Identifier: MIT

package approval_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

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

func collectKinds(j *journal.Journal) []event.Kind {
	var out []event.Kind
	_ = j.Range(func(e *event.Event) error {
		out = append(out, e.Kind)
		return nil
	})
	return out
}

func TestSubmit_GrantedUnblocksWithDecision(t *testing.T) {
	b, j := newBus(t)
	r := approval.New(approval.Config{Bus: b, Timeout: 2 * time.Second})

	var (
		wg  sync.WaitGroup
		out approval.Outcome
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		out = r.Submit(context.Background(), approval.SubmitSpec{
			Capability: "shell",
			ToolName:   "shell",
			Input:      `{"command":"rm important"}`,
			Reason:     "L1 needs approval",
			Actor:      "agent-run-test",
		})
	}()

	// Wait until the registry actually shows the request.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.PendingCount() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if r.PendingCount() != 1 {
		t.Fatalf("expected 1 pending; got %d", r.PendingCount())
	}

	pending := r.Pending()
	if pending[0].Capability != "shell" {
		t.Errorf("pending capability=%q", pending[0].Capability)
	}

	if err := r.Resolve(pending[0].ID, approval.DecisionGrant, "ok by op", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wg.Wait()

	if out.Decision != approval.DecisionGrant {
		t.Errorf("decision=%q want grant", out.Decision)
	}
	if r.PendingCount() != 0 {
		t.Errorf("expected 0 pending after resolve; got %d", r.PendingCount())
	}

	kinds := collectKinds(j)
	if len(kinds) != 2 || kinds[0] != event.KindApprovalRequested || kinds[1] != event.KindApprovalGranted {
		t.Errorf("kinds=%v want [requested, granted]", kinds)
	}
}

func TestSubmit_DeniedUnblocksWithDecision(t *testing.T) {
	b, j := newBus(t)
	r := approval.New(approval.Config{Bus: b})

	doneCh := make(chan approval.Outcome, 1)
	go func() {
		doneCh <- r.Submit(context.Background(), approval.SubmitSpec{Capability: "file.delete"})
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.PendingCount() == 1 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	pending := r.Pending()
	if len(pending) != 1 {
		t.Fatalf("pending=%d", len(pending))
	}
	if err := r.Resolve(pending[0].ID, approval.DecisionDeny, "scary", "operator"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	select {
	case out := <-doneCh:
		if out.Decision != approval.DecisionDeny {
			t.Errorf("decision=%q want deny", out.Decision)
		}
	case <-time.After(time.Second):
		t.Fatal("Submit did not unblock")
	}

	kinds := collectKinds(j)
	if len(kinds) != 2 || kinds[1] != event.KindApprovalDenied {
		t.Errorf("kinds=%v want [requested, denied]", kinds)
	}
}

func TestSubmit_TimeoutAutoDenies(t *testing.T) {
	b, j := newBus(t)
	r := approval.New(approval.Config{Bus: b, Timeout: 80 * time.Millisecond})
	start := time.Now()
	out := r.Submit(context.Background(), approval.SubmitSpec{Capability: "shell"})
	elapsed := time.Since(start)

	if out.Decision != approval.DecisionTimeout {
		t.Errorf("decision=%q want timeout", out.Decision)
	}
	if elapsed < 70*time.Millisecond {
		t.Errorf("Submit returned too fast: %s", elapsed)
	}
	if r.PendingCount() != 0 {
		t.Errorf("expected pending=0 after timeout; got %d", r.PendingCount())
	}
	kinds := collectKinds(j)
	if len(kinds) != 2 || kinds[1] != event.KindApprovalTimeout {
		t.Errorf("kinds=%v want [requested, timeout]", kinds)
	}
}

func TestSubmit_CtxCancelExits(t *testing.T) {
	b, _ := newBus(t)
	r := approval.New(approval.Config{Bus: b, Timeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan approval.Outcome, 1)
	go func() { doneCh <- r.Submit(ctx, approval.SubmitSpec{Capability: "shell"}) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case out := <-doneCh:
		if out.Decision != approval.DecisionCancel {
			t.Errorf("decision=%q want cancel", out.Decision)
		}
	case <-time.After(time.Second):
		t.Fatal("Submit did not exit on ctx cancel")
	}
	if r.PendingCount() != 0 {
		t.Errorf("entry not removed after cancel")
	}
}

func TestResolve_UnknownReturnsError(t *testing.T) {
	r := approval.New(approval.Config{})
	err := r.Resolve("appr-bogus", approval.DecisionGrant, "", "")
	if !errors.Is(err, approval.ErrUnknownApproval) {
		t.Errorf("got %v want ErrUnknownApproval", err)
	}
}

func TestResolve_RejectsNonTerminalDecisions(t *testing.T) {
	r := approval.New(approval.Config{})
	for _, d := range []approval.Decision{approval.DecisionTimeout, approval.DecisionCancel, "garbage"} {
		if err := r.Resolve("any", d, "", ""); err == nil {
			t.Errorf("Resolve accepted non-grant/deny decision %q", d)
		}
	}
}

func TestPending_SortedByCreatedAt(t *testing.T) {
	b, _ := newBus(t)
	r := approval.New(approval.Config{Bus: b, Timeout: 5 * time.Second})
	for range 3 {
		go func() { _ = r.Submit(context.Background(), approval.SubmitSpec{Capability: "shell"}) }()
		time.Sleep(3 * time.Millisecond)
	}
	// Wait for all three to register.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.PendingCount() == 3 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	pending := r.Pending()
	if len(pending) != 3 {
		t.Fatalf("pending=%d", len(pending))
	}
	for i := 1; i < len(pending); i++ {
		if pending[i].CreatedAt.Before(pending[i-1].CreatedAt) {
			t.Errorf("pending not sorted at index %d", i)
		}
	}
}

func TestEvent_RequestedPayloadShape(t *testing.T) {
	b, j := newBus(t)
	r := approval.New(approval.Config{Bus: b, Timeout: 30 * time.Millisecond})
	_ = r.Submit(context.Background(), approval.SubmitSpec{
		Capability:            "file.delete",
		ToolName:              "file",
		Input:                 `{"op":"delete","path":"x"}`,
		Reason:                "L1",
		Actor:                 "agent-run-test",
		CorrelationID:         "corr-zzz",
		EffectClass:           "irreversible",
		PredictedEffects:      []string{"delete x"},
		AffectedResources:     []string{"path:x"},
		RollbackNotes:         "restore from backup",
		Confidence:            0.7,
		CanonicalIntent:       "clean files",
		HarmfulInterpretation: "could delete non-cache files",
		AmbiguityScore:        0.85,
		RegretAxes:            map[string]float64{"informational": 0.9},
		ConfirmationPrompt:    "Confirm exact cleanup scope?",
	})
	var requested *event.Event
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindApprovalRequested {
			requested = e
		}
		return nil
	})
	if requested == nil {
		t.Fatal("no approval.requested event")
	}
	if requested.CorrelationID != "corr-zzz" {
		t.Errorf("correlation=%q want corr-zzz", requested.CorrelationID)
	}
	var p struct {
		ApprovalID            string             `json:"approval_id"`
		Capability            string             `json:"capability"`
		ToolName              string             `json:"tool_name"`
		Input                 string             `json:"input"`
		EffectClass           string             `json:"effect_class"`
		PredictedEffects      []string           `json:"predicted_effects"`
		AffectedResources     []string           `json:"affected_resources"`
		RollbackNotes         string             `json:"rollback_notes"`
		Confidence            float64            `json:"confidence"`
		CanonicalIntent       string             `json:"canonical_intent"`
		HarmfulInterpretation string             `json:"harmful_interpretation"`
		AmbiguityScore        float64            `json:"ambiguity_score"`
		RegretAxes            map[string]float64 `json:"regret_axes"`
		ConfirmationPrompt    string             `json:"confirmation_prompt"`
	}
	if err := json.Unmarshal(requested.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Capability != "file.delete" || p.ToolName != "file" {
		t.Errorf("payload: %+v", p)
	}
	if p.ApprovalID == "" {
		t.Error("approval_id empty")
	}
	if p.EffectClass != "irreversible" || p.RollbackNotes != "restore from backup" || p.Confidence != 0.7 {
		t.Errorf("effect bundle scalar fields: %+v", p)
	}
	if len(p.PredictedEffects) != 1 || p.PredictedEffects[0] != "delete x" {
		t.Errorf("predicted_effects=%v", p.PredictedEffects)
	}
	if len(p.AffectedResources) != 1 || p.AffectedResources[0] != "path:x" {
		t.Errorf("affected_resources=%v", p.AffectedResources)
	}
	if p.CanonicalIntent != "clean files" || p.HarmfulInterpretation == "" || p.ConfirmationPrompt == "" {
		t.Errorf("intent metadata missing: %+v", p)
	}
	if p.AmbiguityScore != 0.85 || p.RegretAxes["informational"] != 0.9 {
		t.Errorf("regret metadata missing: %+v", p)
	}
}

// TestDecision_IsTerminal tests that IsTerminal correctly identifies terminal decisions.
func TestDecision_IsTerminal(t *testing.T) {
	tests := []struct {
		decision approval.Decision
		expected bool
		name     string
	}{
		{approval.DecisionGrant, true, "grant_is_terminal"},
		{approval.DecisionDeny, true, "deny_is_terminal"},
		{approval.DecisionTimeout, true, "timeout_is_terminal"},
		{approval.DecisionCancel, true, "cancel_is_terminal"},
		{approval.Decision(""), false, "empty_is_not_terminal"},
		{approval.Decision("grantx"), false, "typo_is_not_terminal"},
		{approval.Decision("GRANT"), false, "case_sensitive"},
		{approval.Decision("grant "), false, "trailing_space_not_terminal"},
		{approval.Decision(" grant"), false, "leading_space_not_terminal"},
		{approval.Decision("unknown"), false, "unknown_not_terminal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.decision.IsTerminal()
			if got != tt.expected {
				t.Errorf("Decision(%q).IsTerminal() = %v, want %v", tt.decision, got, tt.expected)
			}
		})
	}
}

// TestDecision_IsTerminal_AllFourTerminalStates verifies all four defined terminal states are recognized.
func TestDecision_IsTerminal_AllFourTerminalStates(t *testing.T) {
	if !approval.DecisionGrant.IsTerminal() {
		t.Error("DecisionGrant should be terminal")
	}
	if !approval.DecisionDeny.IsTerminal() {
		t.Error("DecisionDeny should be terminal")
	}
	if !approval.DecisionTimeout.IsTerminal() {
		t.Error("DecisionTimeout should be terminal")
	}
	if !approval.DecisionCancel.IsTerminal() {
		t.Error("DecisionCancel should be terminal")
	}
}

// TestDecision_IsTerminal_DirectConstant tests that IsTerminal works on Decision constants directly.
func TestDecision_IsTerminal_DirectConstant(t *testing.T) {
	if !approval.Decision("grant").IsTerminal() {
		t.Error("Decision(grant).IsTerminal() should be true")
	}
	if approval.Decision("pending").IsTerminal() {
		t.Error("Decision(pending).IsTerminal() should be false")
	}
}
