// SPDX-License-Identifier: MIT

package runtime_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openOKRKernel opens a fresh kernel for OKR tests.
func openOKRKernel(t *testing.T) *runtime.Kernel {
	t.Helper()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	return k
}

// TestOKR_FullLifecycle drives the whole objective/key-result CRUD surface on
// the kernel: OKR store access, CreateObjective, AddObjectiveKeyResult,
// LinkObjectiveTask, UnlinkObjectiveTask, ObjectiveRollup, Rollup, and
// ArchiveObjective. This covers the previously-untested okr.go handlers.
func TestOKR_FullLifecycle(t *testing.T) {
	k := openOKRKernel(t)

	// OKR() must return a live store.
	if k.OKR() == nil {
		t.Fatal("OKR() returned nil store")
	}

	// Create an objective.
	obj, err := k.CreateObjective("corr-okr", okr.CreateSpec{
		Title:       "Ship v1",
		Description: "First public release",
		Owner:       "alice",
	})
	if err != nil {
		t.Fatalf("CreateObjective: %v", err)
	}
	if obj.ID == "" {
		t.Fatal("CreateObjective returned empty ID")
	}
	if obj.Title != "Ship v1" {
		t.Errorf("title = %q, want Ship v1", obj.Title)
	}

	// Add a key result (target 2 tasks).
	obj, err = k.AddObjectiveKeyResult("corr-okr", obj.ID, "Docs complete", 2)
	if err != nil {
		t.Fatalf("AddObjectiveKeyResult: %v", err)
	}
	if len(obj.KeyResults) != 1 {
		t.Fatalf("key results = %d, want 1", len(obj.KeyResults))
	}
	krID := obj.KeyResults[0].ID

	// Link two workboard task ids to the KR (they need not exist as done tasks;
	// taskDone returns false for unknown ids, so the rollup stays unachieved).
	obj, err = k.LinkObjectiveTask("corr-okr", obj.ID, krID, "task-1")
	if err != nil {
		t.Fatalf("LinkObjectiveTask 1: %v", err)
	}
	obj, err = k.LinkObjectiveTask("corr-okr", obj.ID, krID, "task-2")
	if err != nil {
		t.Fatalf("LinkObjectiveTask 2: %v", err)
	}
	if got := len(obj.KeyResults[0].TaskIDs); got != 2 {
		t.Errorf("linked tasks = %d, want 2", got)
	}

	// ObjectiveRollup returns the objective + live progress.
	rObj, prog, ok := k.ObjectiveRollup(obj.ID)
	if !ok {
		t.Fatal("ObjectiveRollup: objective not found")
	}
	if rObj.ID != obj.ID {
		t.Errorf("rollup objective id = %q, want %q", rObj.ID, obj.ID)
	}
	if prog.Achieved {
		t.Error("progress should not be achieved with no done tasks")
	}

	// Rollup on the objective value directly.
	prog2 := k.Rollup(obj)
	if prog2.ObjectiveID != obj.ID {
		t.Errorf("Rollup objective id = %q, want %q", prog2.ObjectiveID, obj.ID)
	}

	// Unlink one task.
	obj, err = k.UnlinkObjectiveTask("corr-okr", obj.ID, krID, "task-2")
	if err != nil {
		t.Fatalf("UnlinkObjectiveTask: %v", err)
	}
	if got := len(obj.KeyResults[0].TaskIDs); got != 1 {
		t.Errorf("linked tasks after unlink = %d, want 1", got)
	}

	// Archive the objective.
	obj, err = k.ArchiveObjective("corr-okr", obj.ID)
	if err != nil {
		t.Fatalf("ArchiveObjective: %v", err)
	}
	if obj.Status != okr.StatusArchived {
		t.Errorf("status after archive = %q, want archived", obj.Status)
	}
}

// TestOKR_RollupMissing covers ObjectiveRollup's not-found branch.
func TestOKR_RollupMissing(t *testing.T) {
	k := openOKRKernel(t)
	if _, _, ok := k.ObjectiveRollup("no-such-id"); ok {
		t.Error("ObjectiveRollup on unknown id returned ok=true, want false")
	}
}

// TestOKR_CreateInvalid covers CreateObjective's error path (empty title).
func TestOKR_CreateInvalid(t *testing.T) {
	k := openOKRKernel(t)
	if _, err := k.CreateObjective("corr", okr.CreateSpec{Title: "   "}); err == nil {
		t.Error("CreateObjective with blank title: expected error")
	}
}
