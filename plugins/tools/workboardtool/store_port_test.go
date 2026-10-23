// SPDX-License-Identifier: MIT

package workboardtool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
)

// capturingStore records the filter the tool asked for and answers with a
// fixed set of tasks — it is how the tool's own view of the board is measured
// without a disk.
type capturingStore struct {
	got     workboardapi.Filter
	tasks   []workboardapi.Task
	getSeen string
}

func (s *capturingStore) Get(id string) (workboardapi.Task, bool) {
	s.getSeen = id
	for _, t := range s.tasks {
		if t.ID == id {
			return t, true
		}
	}
	return workboardapi.Task{}, false
}

func (s *capturingStore) List(f workboardapi.Filter) []workboardapi.Task {
	s.got = f
	return s.tasks
}

// TestTheListAsksThePortForAFilteredBoard: the tool reads the board through
// the port, and the filter it asks for is the operator's filter — the status,
// the tenant, the assignee, the archived switch and the limit.
func TestTheListAsksThePortForAFilteredBoard(t *testing.T) {
	store := &capturingStore{tasks: []workboardapi.Task{
		{ID: "t-1", Title: "one", Status: workboardapi.StatusTodo},
	}}
	tool := New()
	tool.Bind(&fakeKernel{}, store)

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list","status":"todo","tenant":"team-a","assignee":"builder","include_archived":true,"limit":7}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("list errored: %s", res.Output)
	}
	if store.got.Status != workboardapi.StatusTodo {
		t.Errorf("the filter status is %q, want %q", store.got.Status, workboardapi.StatusTodo)
	}
	if store.got.Tenant != "team-a" {
		t.Errorf("the filter tenant is %q", store.got.Tenant)
	}
	if store.got.Assignee != "builder" {
		t.Errorf("the filter assignee is %q", store.got.Assignee)
	}
	if !store.got.IncludeArchived {
		t.Error("the filter must carry the archived switch")
	}
	if store.got.Limit != 7 {
		t.Errorf("the filter limit is %d, want 7", store.got.Limit)
	}
}

// TestTheListBoundsTheLimit: the limit the operator asks for is bounded — a
// missing limit means the default, and no limit may exceed the ceiling.
func TestTheListBoundsTheLimit(t *testing.T) {
	for _, c := range []struct {
		name string
		raw  string
		want int
	}{
		{"the default", `{"op":"list"}`, 20},
		{"an explicit limit", `{"op":"list","limit":5}`, 5},
		{"over the ceiling", `{"op":"list","limit":101}`, 100},
		{"at the ceiling", `{"op":"list","limit":100}`, 100},
		{"a negative limit", `{"op":"list","limit":-3}`, 20},
	} {
		store := &capturingStore{}
		tool := New()
		tool.Bind(&fakeKernel{}, store)
		if _, err := tool.Invoke(context.Background(), json.RawMessage(c.raw)); err != nil {
			t.Fatalf("%s: Invoke: %v", c.name, err)
		}
		if store.got.Limit != c.want {
			t.Errorf("%s: the filter limit is %d, want %d", c.name, store.got.Limit, c.want)
		}
	}
}

// TestTheShowAsksThePortForTheTask: op=show reads the task through the port
// — by id, trimmed.
func TestTheShowAsksThePortForTheTask(t *testing.T) {
	store := &capturingStore{tasks: []workboardapi.Task{
		{ID: "t-1", Title: "one", Status: workboardapi.StatusTodo},
	}}
	tool := New()
	tool.Bind(&fakeKernel{}, store)

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"show","id":"  t-1  "}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("show errored: %s", res.Output)
	}
	if store.getSeen != "t-1" {
		t.Errorf("the port was asked for %q, want %q", store.getSeen, "t-1")
	}
}

// TestTheWireCarriesTheFailedAttemptCount: the retry rule's input is on the
// wire — the agent sees how many attempts have already failed.
func TestTheWireCarriesTheFailedAttemptCount(t *testing.T) {
	v := taskView(workboardapi.Task{
		ID:     "t-1",
		Title:  "one",
		Status: workboardapi.StatusReview,
		Attempts: []workboardapi.Attempt{
			{ID: "a1", Status: "failed"},
			{ID: "a2", Status: "done"},
		},
	})
	if got, ok := v["failed_attempt_count"]; !ok {
		t.Fatal("the wire must carry the failed attempt count")
	} else if got.(int) != 1 {
		t.Errorf("the failed attempt count is %v, want 1", got)
	}

	// A task without failures carries no count at all — the key is not a
	// zero.
	v = taskView(workboardapi.Task{ID: "t-2", Title: "two", Status: workboardapi.StatusTodo})
	if _, ok := v["failed_attempt_count"]; ok {
		t.Error("the wire must drop the failed attempt count when there is none")
	}
}

// TestTheUnavailableBoardIsReported: an unwired board is reported, not
// called into — the tool answers "not available" instead of crashing.
func TestTheUnavailableBoardIsReported(t *testing.T) {
	tool := New()
	tool.Bind(&fakeKernel{}, nil)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("an unwired board must be reported, got %+v", res)
	}
}
