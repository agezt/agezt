// SPDX-License-Identifier: MIT

package okr

// The library's LIFECYCLE: create, key result, link, unlink, archive.
//
// These tests used to live in the DAEMON, because the daemon used to spell
// the objective lifecycle out itself. The lifecycle is the module's (W3.76r):
// the module keeps the goals, the key results and the rollup rule, and the
// daemon's six forwarders over these very operations are gone.
//
// The coverage moved with the subject.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

// openLibraryService opens a store and builds the library's service over it.
func openLibraryService(t *testing.T) api.ObjectiveService {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "okr"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	return NewService(store, api.ObjectiveDependencies{})
}

// TestTheLibraryDrivesTheWholeObjectiveLifecycle drives the whole
// objective/key-result CRUD surface of the library: create, add a key result,
// link two tasks, roll up, unlink one task, archive.
func TestTheLibraryDrivesTheWholeObjectiveLifecycle(t *testing.T) {
	library := openLibraryService(t)

	// Create an objective.
	obj, err := library.Create("corr-okr", okrapi.CreateSpec{
		Title:       "Ship v1",
		Description: "First public release",
		Owner:       "alice",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if obj.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if obj.Title != "Ship v1" {
		t.Errorf("title = %q, want Ship v1", obj.Title)
	}

	// Add a key result (target 2 tasks).
	obj, err = library.AddKeyResult("corr-okr", obj.ID, "Docs complete", 2)
	if err != nil {
		t.Fatalf("AddKeyResult: %v", err)
	}
	if len(obj.KeyResults) != 1 {
		t.Fatalf("key results = %d, want 1", len(obj.KeyResults))
	}
	krID := obj.KeyResults[0].ID

	// Link two workboard task ids to the KR (they need not exist as done tasks;
	// the rollup counts nothing for an unknown id, so it stays unachieved).
	obj, err = library.LinkTask("corr-okr", obj.ID, krID, "task-1")
	if err != nil {
		t.Fatalf("LinkTask 1: %v", err)
	}
	obj, err = library.LinkTask("corr-okr", obj.ID, krID, "task-2")
	if err != nil {
		t.Fatalf("LinkTask 2: %v", err)
	}
	if got := len(obj.KeyResults[0].TaskIDs); got != 2 {
		t.Errorf("linked tasks = %d, want 2", got)
	}

	// The objective's live progress. The "fetch the objective BY ID and its
	// progress" verb went with W3.76k — it had no caller outside the daemon's
	// own tests, and the OKR family is the work module's — so the rollup is
	// asked of the objective the caller already holds.
	prog := library.Progress(obj)
	if prog.ObjectiveID != obj.ID {
		t.Errorf("rollup objective id = %q, want %q", prog.ObjectiveID, obj.ID)
	}
	if prog.Achieved {
		t.Error("progress should not be achieved with no done tasks")
	}

	// Unlink one task.
	obj, err = library.UnlinkTask("corr-okr", obj.ID, krID, "task-2")
	if err != nil {
		t.Fatalf("UnlinkTask: %v", err)
	}
	if got := len(obj.KeyResults[0].TaskIDs); got != 1 {
		t.Errorf("linked tasks after unlink = %d, want 1", got)
	}

	// Archive the objective.
	obj, err = library.Archive("corr-okr", obj.ID)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if obj.Status != okrapi.StatusArchived {
		t.Errorf("status after archive = %q, want archived", obj.Status)
	}
}

// TestTheLibraryRefusesAnObjectiveWithoutATitle covers the create error path.
func TestTheLibraryRefusesAnObjectiveWithoutATitle(t *testing.T) {
	library := openLibraryService(t)
	if _, err := library.Create("corr", okrapi.CreateSpec{Title: "   "}); err == nil {
		t.Error("Create with a blank title: expected an error")
	}
}
