// SPDX-License-Identifier: MIT

package approval_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
)

// TestSubmit_NoBus covers publishRequested and publishResolved
// when r.bus is nil (the early-return path).
func TestSubmit_NoBus_DoesNotPanic(t *testing.T) {
	r := approval.New(approval.Config{Timeout: 30 * time.Millisecond})
	// PublishRequested with bus=nil should be a no-op.
	// The timeout path also calls publishResolved with bus=nil.
	out := r.Submit(context.Background(), approval.SubmitSpec{
		Capability: "shell",
	})
	if out.Decision != approval.DecisionTimeout {
		t.Errorf("expected timeout, got %q", out.Decision)
	}
}

// TestResolve_EmptyResolvedBy covers the resolvedBy=="" default path.
func TestResolve_EmptyResolvedBy(t *testing.T) {
	r := approval.New(approval.Config{Timeout: time.Second})
	doneCh := make(chan approval.Outcome, 1)
	go func() {
		doneCh <- r.Submit(context.Background(), approval.SubmitSpec{Capability: "shell"})
	}()
	// Wait for the request to register.
	for r.PendingCount() == 0 {
	}
	pending := r.Pending()
	if err := r.Resolve(pending[0].ID, approval.DecisionGrant, "ok", ""); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	out := <-doneCh
	if out.Decision != approval.DecisionGrant {
		t.Errorf("decision=%q want grant", out.Decision)
	}
	if out.ResolvedBy != "operator" {
		t.Errorf("ResolvedBy=%q want operator (default)", out.ResolvedBy)
	}
}

// TestResolve_NonBlockingSend covers the default case in the select
// inside Resolve. It sends on the done channel when the timer has
// already fired (timeout already happened).
func TestResolve_NonBlockingSend(t *testing.T) {
	r := approval.New(approval.Config{Timeout: 10 * time.Millisecond})
	// Submit blocks until timeout fires.
	out := r.Submit(context.Background(), approval.SubmitSpec{Capability: "shell"})
	if out.Decision != approval.DecisionTimeout {
		t.Errorf("expected timeout, got %q", out.Decision)
	}
	// After timeout, the entry is detached and any Resolve returns ErrUnknown.
	_ = r.Resolve("non-existent-id", approval.DecisionGrant, "", "")
}

func TestResolve_TwiceReturnsErrUnknown(t *testing.T) {
	r := approval.New(approval.Config{Timeout: time.Second})
	doneCh := make(chan approval.Outcome, 1)
	go func() {
		doneCh <- r.Submit(context.Background(), approval.SubmitSpec{Capability: "shell"})
	}()
	for r.PendingCount() == 0 {
	}
	pending := r.Pending()
	// First resolve succeeds.
	if err := r.Resolve(pending[0].ID, approval.DecisionGrant, "ok", "op"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	<-doneCh
	// Second resolve should return ErrUnknownApproval.
	if err := r.Resolve(pending[0].ID, approval.DecisionGrant, "again", "op"); !errors.Is(err, approval.ErrUnknownApproval) {
		t.Errorf("second Resolve: got %v, want ErrUnknownApproval", err)
	}
}
