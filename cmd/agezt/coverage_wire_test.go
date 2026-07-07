// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/board"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestWireAutoRepair_Disabled covers the two "disabled" early returns: a nil
// kernel and an explicit AGEZT_AUTO_REPAIR=off opt-out.
func TestWireAutoRepair_Disabled(t *testing.T) {
	noNotify := func(board.Message, string) {}

	// Nil kernel → "disabled" (guards the k == nil branch).
	if got := wireAutoRepair(context.Background(), nil, t.TempDir(), &fakeAutoRepairMailbox{}, noNotify); got != "disabled" {
		t.Errorf("nil kernel = %q, want disabled", got)
	}

	// A real kernel but AGEZT_AUTO_REPAIR=off → disabled via opt-out.
	t.Setenv("AGEZT_AUTO_REPAIR", "off")
	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	got := wireAutoRepair(context.Background(), k, t.TempDir(), &fakeAutoRepairMailbox{}, noNotify)
	if !strings.HasPrefix(got, "disabled") {
		t.Errorf("AUTO_REPAIR=off = %q, want a disabled string", got)
	}
}

// TestWireAutoRepair_Armed covers the happy path: with a live kernel and no
// opt-out, wiring subscribes to the pulse subject and launches the coordinator.
func TestWireAutoRepair_Armed(t *testing.T) {
	t.Setenv("AGEZT_AUTO_REPAIR", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops the spawned coordinator goroutine promptly.

	k, err := kernelruntime.Open(kernelruntime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("unused")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	got := wireAutoRepair(ctx, k, t.TempDir(), &fakeAutoRepairMailbox{}, func(board.Message, string) {})
	if !strings.HasPrefix(got, "armed") {
		t.Errorf("wireAutoRepair = %q, want an 'armed' string", got)
	}
}
