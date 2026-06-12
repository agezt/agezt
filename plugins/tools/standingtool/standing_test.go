// SPDX-License-Identifier: MIT

package standingtool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/standing"
)

// testHost wraps a real standing.Store so the tool's Order construction is
// validated end-to-end against the real standing.Validate.
type testHost struct{ store *standing.Store }

func (h testHost) AddStanding(o standing.Order) (standing.Order, error) { return h.store.Add(o) }
func (h testHost) RemoveStanding(id string) (bool, error)               { return h.store.Remove(id) }
func (h testHost) Standing() *standing.Store                            { return h.store }

func newTool(t *testing.T) (*Tool, *standing.Store) {
	t.Helper()
	st, err := standing.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	tool := New()
	tool.Bind(testHost{st})
	return tool, st
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (map[string]any, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}
	var out map[string]any
	_ = json.Unmarshal([]byte(res.Output), &out)
	return out, res.IsError
}

func TestDefinitionValid(t *testing.T) {
	d := New().Definition()
	if d.Name != "standing" || !json.Valid(d.InputSchema) {
		t.Fatalf("bad definition: %+v", d)
	}
}

func TestCreateEvent_PassesValidationAndPersists(t *testing.T) {
	tool, st := newTool(t)
	out, isErr := invoke(t, tool, map[string]any{
		"op": "create_event", "name": "health-watch", "subject": "observer.delta",
		"plan": "Investigate the health degradation.",
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if st.Count() != 1 {
		t.Fatalf("order not persisted, count=%d", st.Count())
	}
	// Default mode is the conservative "ask".
	if out["mode"] != string(standing.InitiativeAsk) {
		t.Errorf("mode = %v, want ask", out["mode"])
	}
	trigs := out["triggers"].([]any)
	if trigs[0].(map[string]any)["subject"] != "observer.delta" {
		t.Errorf("event subject wrong: %+v", trigs[0])
	}
}

func TestCreateEvent_AssureBudget(t *testing.T) {
	tool, st := newTool(t)
	out, isErr := invoke(t, tool, map[string]any{
		"op": "create_event", "name": "must-fix", "subject": "task.failed",
		"plan": "Diagnose and fix the failure.", "assure": 3,
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if out["assure"].(float64) != 3 {
		t.Errorf("order view assure = %v, want 3", out["assure"])
	}
	// The budget must persist on the stored order so the fire path can read it.
	got := st.List()[0]
	if got.Assure != 3 {
		t.Errorf("stored Assure = %d, want 3", got.Assure)
	}
}

func TestCreate_NegativeAssureClampedToZero(t *testing.T) {
	tool, st := newTool(t)
	invoke(t, tool, map[string]any{
		"op": "create_cron", "name": "n", "schedule": "0 9 * * *", "plan": "p", "assure": -5,
	})
	if got := st.List()[0]; got.Assure != 0 {
		t.Errorf("negative assure should clamp to 0, got %d", got.Assure)
	}
}

func TestCreateCron_PassesValidation(t *testing.T) {
	tool, st := newTool(t)
	out, isErr := invoke(t, tool, map[string]any{
		"op": "create_cron", "name": "nightly", "schedule": "0 9 * * *",
		"plan": "Produce a morning digest.", "mode": "inform_only",
	})
	if isErr {
		t.Fatalf("unexpected error: %v", out)
	}
	if st.Count() != 1 {
		t.Fatalf("not persisted")
	}
	if out["mode"] != "inform_only" {
		t.Errorf("mode override not applied: %v", out["mode"])
	}
}

func TestListAndRemove(t *testing.T) {
	tool, _ := newTool(t)
	create, _ := invoke(t, tool, map[string]any{
		"op": "create_event", "name": "x", "subject": "task.failed", "plan": "look into it",
	})
	id := create["id"].(string)

	list, _ := invoke(t, tool, map[string]any{"op": "list"})
	if list["count"].(float64) != 1 {
		t.Fatalf("list count = %v, want 1", list["count"])
	}

	rm, isErr := invoke(t, tool, map[string]any{"op": "remove", "id": id})
	if isErr || rm["removed"] != id {
		t.Fatalf("remove failed: %+v", rm)
	}
	list2, _ := invoke(t, tool, map[string]any{"op": "list"})
	if list2["count"].(float64) != 0 {
		t.Errorf("order not removed, count = %v", list2["count"])
	}
}

func TestValidationErrors(t *testing.T) {
	tool, _ := newTool(t)
	cases := []map[string]any{
		{"op": "create_event", "subject": "x", "plan": "y"}, // missing name
		{"op": "create_event", "name": "n", "plan": "y"},    // missing subject
		{"op": "create_event", "name": "n", "subject": "x"}, // missing plan
		{"op": "create_cron", "name": "n", "plan": "y"},     // missing schedule
		{"op": "remove"}, // missing id
		{"op": "bogus"},
		{"op": ""},
	}
	for _, c := range cases {
		if _, isErr := invoke(t, tool, c); !isErr {
			t.Errorf("expected error for %v", c)
		}
	}
}

func TestUnboundIsSafe(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("an unbound tool should return an error result")
	}
}
