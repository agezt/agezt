// SPDX-License-Identifier: MIT

package workboard

// The board's SERVICE: the verbs the daemon used to spell out itself.
//
// These tests pin the three things the move changed and the one it did not:
// the board REFUSES without a store (the contract's row), the board PUBLISHES
// what happened to a task (through the host's port, in the contract's shape),
// and the board TELLS the host that a task reached done (the objective
// rollup is the host's business). The refusals and the answers did not move;
// only the place that knows them did.

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/work/api"
)

func openBoard(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "workboard"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	return store
}

// The board refuses every verb without a store.
//
// The refusal is the CONTRACT's row: the host may not name the module, and the
// module may not name the host, so the answer they share has to live in the
// contract. A daemon with no board answers no board.
func TestTheBoardRefusesWithoutAStore(t *testing.T) {
	svc := NewService(nil, api.BoardDependencies{})
	if _, _, err := svc.Create("corr", workboardapi.CreateSpec{Title: "none"}); err != workboardapi.ErrNoStore {
		t.Errorf("Create without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Claim("corr", "id", "agent", "run"); err != workboardapi.ErrNoStore {
		t.Errorf("Claim without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Heartbeat("corr", "id", "agent", "run"); err != workboardapi.ErrNoStore {
		t.Errorf("Heartbeat without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Comment("corr", "id", "author", "body"); err != workboardapi.ErrNoStore {
		t.Errorf("Comment without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Block("corr", "id", "actor", "reason"); err != workboardapi.ErrNoStore {
		t.Errorf("Block without a store: err = %v, want the contract's refusal", err)
	}
	if _, _, err := svc.Fail("corr", "id", "actor", "reason"); err != workboardapi.ErrNoStore {
		t.Errorf("Fail without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Unblock("corr", "id", "actor"); err != workboardapi.ErrNoStore {
		t.Errorf("Unblock without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.SetRetryPolicy("corr", "id", "actor", nil); err != workboardapi.ErrNoStore {
		t.Errorf("SetRetryPolicy without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Complete("corr", "id", "actor"); err != workboardapi.ErrNoStore {
		t.Errorf("Complete without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Review("corr", "id", "actor", "summary"); err != workboardapi.ErrNoStore {
		t.Errorf("Review without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Archive("corr", "id", "actor"); err != workboardapi.ErrNoStore {
		t.Errorf("Archive without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.Link("corr", "id", "type", "target"); err != workboardapi.ErrNoStore {
		t.Errorf("Link without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.AddDependency("corr", "id", "other"); err != workboardapi.ErrNoStore {
		t.Errorf("AddDependency without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.ReclaimStale("corr", "id", "actor", time.Minute); err != workboardapi.ErrNoStore {
		t.Errorf("ReclaimStale without a store: err = %v, want the contract's refusal", err)
	}
	if _, err := svc.SweepStaleClaims("corr", "actor", time.Minute, 10); err != workboardapi.ErrNoStore {
		t.Errorf("SweepStaleClaims without a store: err = %v, want the contract's refusal", err)
	}
}

// The board says what happened to a task, on the host's bus.
//
// The subject, the kind and the payload are the BOARD's own: the host's port
// carries the event, the board decides what it says. The payload's shape is
// the contract's row, because the tools read one shape.
func TestTheBoardPublishesWhatHappenedToATask(t *testing.T) {
	var published []event.Spec
	svc := NewService(openBoard(t), api.BoardDependencies{
		Publish: func(spec event.Spec) { published = append(published, spec) },
	})
	task, created, err := svc.Create("corr-1", workboardapi.CreateSpec{Title: "owned"})
	if err != nil || !created {
		t.Fatalf("Create: created = %v, err = %v", created, err)
	}
	if len(published) != 1 {
		t.Fatalf("published %d events, want 1", len(published))
	}
	got := published[0]
	if got.Kind != event.KindWorkboardTaskCreated {
		t.Errorf("kind = %q, want %q", got.Kind, event.KindWorkboardTaskCreated)
	}
	if got.Subject != "workboard."+task.ID {
		t.Errorf("subject = %q, want %q", got.Subject, "workboard."+task.ID)
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
	if payload["id"] != task.ID {
		t.Errorf("payload id = %v, want %q", payload["id"], task.ID)
	}
	if payload["title"] != "owned" {
		t.Errorf("payload title = %v, want %q", payload["title"], "owned")
	}

	// A claim says its own thing: the board publishes EVERY move, and a claim
	// that stayed silent would leave the fleet's board unwatched.
	if _, err := svc.Claim("corr-1", task.ID, "agent-1", "run-1"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(published) != 2 {
		t.Fatalf("published %d events, want 2", len(published))
	}
	if published[1].Kind != event.KindWorkboardTaskClaimed {
		t.Errorf("claim kind = %q, want %q", published[1].Kind, event.KindWorkboardTaskClaimed)
	}
	if published[1].Payload.(map[string]any)["agent"] != "agent-1" {
		t.Errorf("claim payload = %v, want the agent that claimed it", published[1].Payload)
	}
}

// The board tells the host that a task reached done.
//
// The objective rollup is the HOST's business today: a done task counts
// toward a Key Result and the host keeps the library. The board calls the
// hook and holds no knowledge of objectives.
func TestTheBoardTellsTheHostThatATaskReachedDone(t *testing.T) {
	var completed []string
	svc := NewService(openBoard(t), api.BoardDependencies{
		TaskCompleted: func(corr, taskID string) { completed = append(completed, corr+"/"+taskID) },
	})
	task, _, err := svc.Create("corr-2", workboardapi.CreateSpec{Title: "owned"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Complete("corr-2", task.ID, "actor"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(completed) != 1 || completed[0] != "corr-2/"+task.ID {
		t.Fatalf("the host was told %v, want one rollup for %s", completed, task.ID)
	}
}

// A host with no bus is quiet rather than broken.
//
// The task still moved and the caller still holds the answer: the board
// publishes through a port, and a daemon with no bus simply has no port.
func TestTheBoardIsQuietWithoutABus(t *testing.T) {
	svc := NewService(openBoard(t), api.BoardDependencies{})
	task, created, err := svc.Create("corr-3", workboardapi.CreateSpec{Title: "quiet"})
	if err != nil || !created {
		t.Fatalf("Create without a bus: created = %v, err = %v", created, err)
	}
	if task.Title != "quiet" {
		t.Errorf("title = %q, want %q", task.Title, "quiet")
	}
}
