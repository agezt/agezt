// SPDX-License-Identifier: MIT

package okr

// The library's SERVICE: the objective verbs the daemon used to spell out
// itself.
//
// These tests pin the three things the move changed and the one it did not:
// the library REFUSES without a store (the contract's row), the library
// PUBLISHES what happened to a goal (through the host's port, in the
// library's own shape), the library ROLLS UP from the live status of the
// linked tasks (the host's board, asked through a port), and the library
// RECOMPUTES the durable "achieved" signal when a task crosses the target.
// The refusals and the answers did not move; only the place that knows them
// did.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

func openLibrary(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "okr"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	return store
}

// The library refuses every verb without a store.
//
// The refusal is the CONTRACT's row: the host may not name the module, and the
// module may not name the host, so the answer they share has to live in the
// contract. A daemon with no library answers no library.
func TestTheLibraryRefusesWithoutAStore(t *testing.T) {
	svc := NewService(nil, api.ObjectiveDependencies{})
	if _, err := svc.Create("corr", okrapi.CreateSpec{Title: "none"}); err != okrapi.ErrNoLibrary {
		t.Errorf("Create without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.AddKeyResult("corr", "o-1", "kr", 1); err != okrapi.ErrNoLibrary {
		t.Errorf("AddKeyResult without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.LinkTask("corr", "o-1", "kr-1", "t-1"); err != okrapi.ErrNoLibrary {
		t.Errorf("LinkTask without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.UnlinkTask("corr", "o-1", "kr-1", "t-1"); err != okrapi.ErrNoLibrary {
		t.Errorf("UnlinkTask without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Archive("corr", "o-1"); err != okrapi.ErrNoLibrary {
		t.Errorf("Archive without a store: err = %v, want the contract's refusal", err)
	}
	if _, _, ok := svc.ProgressOf("o-1"); ok {
		t.Error("ProgressOf without a store must answer no objective")
	}
}

// The library says what happened to a goal, on the host's bus.
//
// The subject, the kind and the payload are the LIBRARY's own: the host's port
// carries the event, the library decides what it says.
func TestTheLibraryPublishesWhatHappenedToAGoal(t *testing.T) {
	var published []event.Spec
	svc := NewService(openLibrary(t), api.ObjectiveDependencies{
		Publish: func(spec event.Spec) { published = append(published, spec) },
	})
	goal, err := svc.Create("corr-1", okrapi.CreateSpec{Title: "owned"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("published %d events, want 1", len(published))
	}
	got := published[0]
	if got.Kind != event.KindOKRObjectiveCreated {
		t.Errorf("kind = %q, want %q", got.Kind, event.KindOKRObjectiveCreated)
	}
	if got.Subject != "okr."+goal.ID {
		t.Errorf("subject = %q, want %q", got.Subject, "okr."+goal.ID)
	}
	if got.CorrelationID != "corr-1" {
		t.Errorf("correlation = %q, want %q", got.CorrelationID, "corr-1")
	}
	payload, ok := got.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload is %T, want a map", got.Payload)
	}
	if payload["action"] != "created" {
		t.Errorf("action = %v, want %q", payload["action"], "created")
	}
	if payload["id"] != goal.ID {
		t.Errorf("payload id = %v, want %q", payload["id"], goal.ID)
	}
	if payload["title"] != "owned" {
		t.Errorf("payload title = %v, want %q", payload["title"], "owned")
	}

	// A key result says its own thing: the library publishes EVERY move, and a
	// mutation that stayed silent would leave the fleet's goals unwatched.
	updated, err := svc.AddKeyResult("corr-1", goal.ID, "criterion", 2)
	if err != nil {
		t.Fatalf("AddKeyResult: %v", err)
	}
	if len(published) != 2 {
		t.Fatalf("published %d events, want 2", len(published))
	}
	if published[1].Kind != event.KindOKRObjectiveUpdated {
		t.Errorf("key-result kind = %q, want %q", published[1].Kind, event.KindOKRObjectiveUpdated)
	}
	if published[1].Payload.(map[string]any)["title"] != "criterion" {
		t.Errorf("key-result payload = %v, want the criterion that was added", published[1].Payload)
	}

	// An archive says it too — a retired goal that stayed silent would read as
	// a goal still in flight.
	if _, err := svc.Archive("corr-1", updated.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if len(published) != 3 {
		t.Fatalf("published %d events, want 3", len(published))
	}
	if published[2].Payload.(map[string]any)["action"] != "archived" {
		t.Errorf("archive payload = %v, want the archived action", published[2].Payload)
	}
}

// The library asks the host for a task's live status, and counts what is DONE.
//
// The rule — what "done" means — is the library's, but the STATUS is the
// host's: the board is the host's, and the library only asks. A library that
// reached into the board itself would hold the host's machinery.
func TestTheLibraryRollsUpFromTheLiveStatusOfTheLinkedTasks(t *testing.T) {
	statuses := map[string]workboardapi.Status{
		"task-1": workboardapi.StatusDone,
		"task-2": workboardapi.StatusReady,
	}
	var asked []string
	svc := NewService(openLibrary(t), api.ObjectiveDependencies{
		TaskStatus: func(taskID string) (workboardapi.Status, bool) {
			asked = append(asked, taskID)
			status, ok := statuses[taskID]
			return status, ok
		},
	})
	goal, err := svc.Create("corr-2", okrapi.CreateSpec{Title: "rolled"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	goal, err = svc.AddKeyResult("corr-2", goal.ID, "criterion", 2)
	if err != nil {
		t.Fatalf("AddKeyResult: %v", err)
	}
	krID := goal.KeyResults[0].ID
	for _, taskID := range []string{"task-1", "task-2"} {
		goal, err = svc.LinkTask("corr-2", goal.ID, krID, taskID)
		if err != nil {
			t.Fatalf("LinkTask: %v", err)
		}
	}

	progress := svc.Progress(goal)
	if len(asked) == 0 {
		t.Fatal("the library must ask the host for the tasks' status")
	}
	if progress.KeyResults[0].Done != 1 {
		t.Errorf("done = %d, want 1 — only the done task counts", progress.KeyResults[0].Done)
	}
	if progress.KeyResults[0].Total != 2 {
		t.Errorf("total = %d, want 2", progress.KeyResults[0].Total)
	}
	if progress.Achieved {
		t.Error("a goal with one open task must not be achieved")
	}
	if progress.Percent != 50 {
		t.Errorf("percent = %d, want 50", progress.Percent)
	}

	// The goal and its rollup come back together.
	got, pr, ok := svc.ProgressOf(goal.ID)
	if !ok || got.ID != goal.ID {
		t.Fatalf("ProgressOf = %+v %v, want the goal", got, ok)
	}
	if pr.Percent != 50 {
		t.Errorf("ProgressOf percent = %d, want 50", pr.Percent)
	}

	// An unknown goal is answered "not there" — not a zeroed goal that reads
	// like a real one.
	if _, _, ok := svc.ProgressOf("no-such-goal"); ok {
		t.Error("ProgressOf must answer no objective for an unknown id")
	}
}

// A host with no board counts nothing.
//
// The library stays quiet rather than broken: the goal still moves, and the
// caller still holds the answer.
func TestTheLibraryCountsNothingWithoutABoard(t *testing.T) {
	svc := NewService(openLibrary(t), api.ObjectiveDependencies{})
	goal, err := svc.Create("corr-3", okrapi.CreateSpec{Title: "noboard"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	goal, err = svc.AddKeyResult("corr-3", goal.ID, "criterion", 1)
	if err != nil {
		t.Fatalf("AddKeyResult: %v", err)
	}
	if _, err := svc.LinkTask("corr-3", goal.ID, goal.KeyResults[0].ID, "task-1"); err != nil {
		t.Fatalf("LinkTask: %v", err)
	}
	progress := svc.Progress(goal)
	if progress.KeyResults[0].Done != 0 {
		t.Errorf("done = %d, want 0 — without a board nothing counts", progress.KeyResults[0].Done)
	}
	if progress.Achieved {
		t.Error("without a board nothing is achieved")
	}
}

// The library maintains the durable "achieved" signal when a task crosses the
// target.
//
// The rollup itself is always computed live on read; this only keeps the
// stored status and the achieved event in step with it. A library that stopped
// recomputing would leave a goal marked "active" after its tasks were done.
func TestTheLibraryRecomputesTheAchievedSignalForATask(t *testing.T) {
	var published []event.Spec
	done := map[string]bool{}
	svc := NewService(openLibrary(t), api.ObjectiveDependencies{
		Publish: func(spec event.Spec) { published = append(published, spec) },
		TaskStatus: func(taskID string) (workboardapi.Status, bool) {
			if done[taskID] {
				return workboardapi.StatusDone, true
			}
			return workboardapi.StatusReady, true
		},
	})
	goal, err := svc.Create("corr-4", okrapi.CreateSpec{Title: "crossing"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	goal, err = svc.AddKeyResult("corr-4", goal.ID, "criterion", 1)
	if err != nil {
		t.Fatalf("AddKeyResult: %v", err)
	}
	krID := goal.KeyResults[0].ID
	if _, err := svc.LinkTask("corr-4", goal.ID, krID, "task-1"); err != nil {
		t.Fatalf("LinkTask: %v", err)
	}

	// The task is not done yet: the goal stays active.
	svc.RecomputeForTask("corr-4", "task-1")
	stored, _, ok := svc.ProgressOf(goal.ID)
	if !ok {
		t.Fatal("ProgressOf must find the goal")
	}
	if stored.Status == okrapi.StatusAchieved {
		t.Fatal("the goal must not be achieved while its task is open")
	}

	// The task completes: the goal crosses the target and says so.
	done["task-1"] = true
	svc.RecomputeForTask("corr-4", "task-1")
	achieved := false
	for _, spec := range published {
		if spec.Kind == event.KindOKRObjectiveAchieved {
			achieved = true
			if spec.Subject != "okr."+goal.ID {
				t.Errorf("the achieved event names %q, want %q", spec.Subject, "okr."+goal.ID)
			}
		}
	}
	if !achieved {
		t.Fatal("the library must announce the goal it achieved")
	}
	final, _, ok := svc.ProgressOf(goal.ID)
	if !ok {
		t.Fatal("ProgressOf must find the goal")
	}
	if final.Status != okrapi.StatusAchieved {
		t.Errorf("status = %q, want %q", final.Status, okrapi.StatusAchieved)
	}

	// And back: a reopened task reopens the goal.
	done["task-1"] = false
	svc.RecomputeForTask("corr-4", "task-1")
	reopened, _, ok := svc.ProgressOf(goal.ID)
	if !ok {
		t.Fatal("ProgressOf must find the goal")
	}
	if reopened.Status != okrapi.StatusActive {
		t.Errorf("status = %q, want %q — a reopened task reopens the goal", reopened.Status, okrapi.StatusActive)
	}
}

// A host with no bus is quiet rather than broken.
//
// The goal still moved and the caller still holds the answer: the library
// publishes through a port, and a daemon with no bus simply has no port.
func TestTheLibraryIsQuietWithoutABus(t *testing.T) {
	svc := NewService(openLibrary(t), api.ObjectiveDependencies{})
	goal, err := svc.Create("corr-5", okrapi.CreateSpec{Title: "quiet"})
	if err != nil {
		t.Fatalf("Create without a bus: %v", err)
	}
	if goal.Title != "quiet" {
		t.Errorf("title = %q, want %q", goal.Title, "quiet")
	}
}
