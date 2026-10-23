// SPDX-License-Identifier: MIT

package workboardapi

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestTheTaskWireShape: the task row is a CONTRACT — it crosses the wire
// between the daemon, the CLI, the web UI and the agent-facing tool. The
// always-present keys and the dropped-when-empty keys are therefore part of
// the promise, not an accident of the struct tags.
//
// The key set is measured EXACTLY: a renamed or added key is a wire break.
func TestTheTaskWireShape(t *testing.T) {
	raw, err := json.Marshal(Task{ID: "t-1", Title: "Ship", Status: StatusTodo})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"id", "title", "status", "created_ms", "updated_ms"}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("the task wire must always carry %q, got %v", k, keys(got))
		}
	}
	optional := []string{
		"description", "priority", "tenant", "assignee", "owner", "idempotency_key",
		"tags", "artifacts", "criteria", "proof", "seat", "retry_policy", "claim",
		"dependencies", "attempts", "comments", "links", "block_reason",
		"completed_ms", "archived_ms",
	}
	for _, k := range optional {
		if _, ok := got[k]; ok {
			t.Errorf("the task wire must drop %q when it is empty, got %v", k, keys(got))
		}
	}
	if len(got) != len(want) {
		t.Errorf("the task wire has %d keys, want %d: %v", len(got), len(want), keys(got))
	}
}

// TestTheTaskWireCarriesTheOptionalsWhenSet: the optional keys are dropped
// when empty and present when set — both halves are the promise.
func TestTheTaskWireCarriesTheOptionalsWhenSet(t *testing.T) {
	raw, err := json.Marshal(Task{
		ID:             "t-1",
		Title:          "Ship",
		Status:         StatusRunning,
		Description:    "details",
		Priority:       3,
		Tenant:         "team-a",
		Assignee:       "builder",
		Owner:          "lead",
		IdempotencyKey: "wb-1",
		Tags:           []string{"urgent"},
		Artifacts:      []string{"log.txt"},
		BlockReason:    "waiting",
		CompletedMS:    100,
		ArchivedMS:     200,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{
		"description", "priority", "tenant", "assignee", "owner", "idempotency_key",
		"tags", "artifacts", "block_reason", "completed_ms", "archived_ms",
	} {
		if _, ok := got[k]; !ok {
			t.Errorf("the task wire must carry %q when it is set, got %v", k, keys(got))
		}
	}
}

// TestTheStatusValuesAreTheContract: the lifecycle states are WIRE — they are
// stored in files, printed by the CLI and typed by the agent-facing tool. A
// renamed state would silently orphan every stored task.
func TestTheStatusValuesAreTheContract(t *testing.T) {
	for _, c := range []struct {
		got  Status
		want string
	}{
		{StatusTriage, "triage"},
		{StatusTodo, "todo"},
		{StatusReady, "ready"},
		{StatusRunning, "running"},
		{StatusBlocked, "blocked"},
		{StatusReview, "review"},
		{StatusDone, "done"},
		{StatusArchived, "archived"},
	} {
		if string(c.got) != c.want {
			t.Errorf("the status is %q, want %q", string(c.got), c.want)
		}
	}
}

// TestTheKnownStatusesAreTheEight: the board has exactly eight states. A new
// one has to be added on purpose, with its place in the lifecycle.
func TestTheKnownStatusesAreTheEight(t *testing.T) {
	known := []Status{
		StatusTriage, StatusTodo, StatusReady, StatusRunning,
		StatusBlocked, StatusReview, StatusDone, StatusArchived,
	}
	for _, s := range known {
		if !KnownStatus(s) {
			t.Errorf("%q must be a known status", s)
		}
	}
	if KnownStatus("nope") {
		t.Error("an unknown status must not be known")
	}
	if KnownStatus(Status("")) {
		t.Error("the empty status must not be known")
	}
}

// TestTheStatusOrderIsTheLifecycle: the order drives the board's columns and
// the sort. It must be strictly increasing along the lifecycle and unknown
// statuses must have no position.
func TestTheStatusOrderIsTheLifecycle(t *testing.T) {
	lifecycle := []Status{
		StatusTriage, StatusTodo, StatusReady, StatusRunning,
		StatusBlocked, StatusReview, StatusDone, StatusArchived,
	}
	prev := StatusOrder(lifecycle[0])
	if prev != 0 {
		t.Fatalf("the first status sits at %d, want 0", prev)
	}
	for _, s := range lifecycle[1:] {
		got := StatusOrder(s)
		if got != prev+1 {
			t.Errorf("%q sits at %d, want %d", s, got, prev+1)
		}
		prev = got
	}
	if got := StatusOrder("nope"); got != -1 {
		t.Errorf("an unknown status sits at %d, want -1", got)
	}
}

// TestParseStatusNormalizesAndValidates: the rule the whole daemon enforces —
// the status is trimmed, lowercased and then checked against the lifecycle.
func TestParseStatusNormalizesAndValidates(t *testing.T) {
	for _, c := range []struct {
		in   string
		want Status
	}{
		{"todo", StatusTodo},
		{"  TODO  ", StatusTodo},
		{"Ready", StatusReady},
		{"archived", StatusArchived},
	} {
		got, err := ParseStatus(c.in)
		if err != nil {
			t.Errorf("ParseStatus(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseStatus(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParseStatusRefusesTheUnknown: an unknown status is refused, and the
// refusal is the CONTRACT's error — the tool maps it to one answer.
func TestParseStatusRefusesTheUnknown(t *testing.T) {
	for _, in := range []string{"", "   ", "bogus", "donee"} {
		got, err := ParseStatus(in)
		if err == nil {
			t.Errorf("ParseStatus(%q) must fail, got %q", in, got)
			continue
		}
		if !errors.Is(err, ErrInvalidStatus) {
			t.Errorf("ParseStatus(%q) = %v, want the contract's error", in, err)
		}
		if got != "" {
			t.Errorf("ParseStatus(%q) = %q, want the empty status", in, got)
		}
	}
}

// TestParseStatusRefusesTheEmptyStatusWithoutASuffix: the empty status is
// refused in its own right — it is not "an unknown status called ". That is
// the difference between a refusal and a normalization, and the two are
// answered differently downstream.
func TestParseStatusRefusesTheEmptyStatusWithoutASuffix(t *testing.T) {
	_, err := ParseStatus("   ")
	if err == nil {
		t.Fatal("the empty status must be refused")
	}
	if err.Error() != ErrInvalidStatus.Error() {
		t.Errorf("the empty status is refused as %q, want %q", err.Error(), ErrInvalidStatus.Error())
	}
	// An UNKNOWN status, by contrast, names itself.
	_, err = ParseStatus("bogus")
	if err == nil {
		t.Fatal("an unknown status must be refused")
	}
	if err.Error() == ErrInvalidStatus.Error() {
		t.Error("an unknown status must name itself")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("the refusal must name the status, got %q", err.Error())
	}
}

// TestFailedAttemptCountCountsTheFailures: the retry rule's input and the
// number the board shows. Only a failed or stale attempt counts.
func TestFailedAttemptCountCountsTheFailures(t *testing.T) {
	task := Task{Attempts: []Attempt{
		{ID: "a1", Status: "failed"},
		{ID: "a2", Status: "done"},
		{ID: "a3", Status: "stale"},
		{ID: "a4", Status: "running"},
		{ID: "a5", Status: "FAILED"},
		{ID: "a6", Status: " stale "},
	}}
	if got := FailedAttemptCount(task); got != 4 {
		t.Errorf("FailedAttemptCount = %d, want 4", got)
	}
	if got := FailedAttemptCount(Task{}); got != 0 {
		t.Errorf("FailedAttemptCount of an empty task = %d, want 0", got)
	}
}

// TestTheStorePortIsTheReadSide: the port through which the stored tasks are
// read. Its shape is the contract — the implementation satisfies it, the
// agent-facing tool speaks it.
func TestTheStorePortIsTheReadSide(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 2 {
		t.Fatalf("Store has %d methods, want 2 (Get + List)", typ.NumMethod())
	}
	get, ok := typ.MethodByName("Get")
	if !ok {
		t.Fatal("Store must have a Get method")
	}
	if get.Type.NumIn() != 1 || get.Type.NumOut() != 2 {
		t.Fatalf("Get must take an id and answer (Task, bool), got %v", get.Type)
	}
	if get.Type.Out(0) != reflect.TypeOf(Task{}) || get.Type.Out(1).Kind() != reflect.Bool {
		t.Fatalf("Get must answer (Task, bool), got %v", get.Type)
	}
	list, ok := typ.MethodByName("List")
	if !ok {
		t.Fatal("Store must have a List method")
	}
	if list.Type.NumIn() != 1 || list.Type.In(0) != reflect.TypeOf(Filter{}) {
		t.Fatalf("List must take a Filter, got %v", list.Type)
	}
	if list.Type.NumOut() != 1 || list.Type.Out(0) != reflect.TypeOf([]Task{}) {
		t.Fatalf("List must answer []Task, got %v", list.Type)
	}
}

// keys returns the sorted key names of a decoded JSON object — for the
// failure messages.
func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
