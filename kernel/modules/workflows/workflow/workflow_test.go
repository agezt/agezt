// SPDX-License-Identifier: MIT

package workflow

import (
	"encoding/json"
	"testing"
)

func graph(nodes []Node, edges []Edge) Workflow {
	return Workflow{Name: "test-flow", Nodes: nodes, Edges: edges}
}

func trigger() Node { return Node{ID: "start", Type: NodeTrigger} }

func toolNode(id string) Node {
	return Node{ID: id, Type: NodeTool, Config: json.RawMessage(`{"tool":"shell","args":{"command":"ls"}}`)}
}

// The rule's own tests moved WITH the rule to the contract layer
// (kernel/contract/workflowapi/workflow_test.go, W3.29): the validators and
// the rows they judge are one thing now.

func TestInterpolate(t *testing.T) {
	data := map[string]any{
		"trigger": map[string]any{"payload": map[string]any{"city": "izmir", "n": float64(3)}},
		"fetch":   map[string]any{"output": map[string]any{"items": []any{map[string]any{"title": "first"}, "second"}}},
		"plain":   map[string]any{"output": "hello"},
	}
	cases := []struct{ in, want string }{
		{"city={{trigger.payload.city}}", "city=izmir"},
		{"n={{trigger.payload.n}}", "n=3"},
		{"t={{fetch.output.items.0.title}}", "t=first"},
		{"s={{ fetch.output.items.1 }}", "s=second"},
		{"whole={{plain.output}}", "whole=hello"},
		{"obj={{fetch.output}}", `obj={"items":[{"title":"first"},"second"]}`},
		{"miss=[{{ghost.output}}]", "miss=[]"}, // unknown path → empty, run survives
		{"no templates here", "no templates here"},
		{"dangling {{open", "dangling {{open"},
	}
	for _, tc := range cases {
		if got := Interpolate(tc.in, data); got != tc.want {
			t.Errorf("Interpolate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStore_UpsertByName(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	w := graph([]Node{trigger(), toolNode("a")}, []Edge{{From: "start", To: "a"}})
	saved, created, err := s.Save(w)
	if err != nil || !created || saved.ID == "" || !saved.Enabled {
		t.Fatalf("first save = %+v/%v/%v", saved, created, err)
	}

	// Disable, then upsert a new graph under the same name: identity +
	// enabled survive, the graph is replaced wholesale.
	if _, err := s.SetEnabled(saved.Name, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	w2 := w
	w2.Nodes = []Node{trigger(), toolNode("a"), toolNode("b")}
	w2.Edges = []Edge{{From: "start", To: "a"}, {From: "a", To: "b"}}
	updated, created, err := s.Save(w2)
	if err != nil || created {
		t.Fatalf("upsert = %v/%v", created, err)
	}
	if updated.ID != saved.ID || updated.Enabled || len(updated.Nodes) != 3 {
		t.Fatalf("upsert lost identity/state: %+v", updated)
	}

	// Invalid graphs never reach disk.
	bad := w
	bad.Nodes = nil
	if _, _, err := s.Save(bad); err == nil {
		t.Fatal("invalid save accepted")
	}
	if got, _ := s.Get(saved.ID); len(got.Nodes) != 3 {
		t.Fatalf("failed save mutated the store: %+v", got)
	}
}

// TestTriggerSpec_NormalizesDailyAt proves the normalization reaches the
// consumers through the single parse chokepoint, for both spellings and for a
// workflow loaded back off disk (stored configs predate the fix).
func TestTriggerSpec_NormalizesDailyAt(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	for _, tc := range []struct{ name, stored, want string }{
		{"unpadded", "9:05", "09:05"},
		{"padded", "19:30", "19:30"},
	} {
		w := Workflow{
			Name:  tc.name,
			Nodes: []Node{{ID: "start", Type: NodeTrigger, Config: json.RawMessage(`{"kind":"cron","daily_at":"` + tc.stored + `"}`)}},
		}
		// The public contract is preserved: Validate keeps accepting the
		// one-digit form it has always accepted.
		if err := Validate(w); err != nil {
			t.Fatalf("Validate(%q) rejected: %v", tc.stored, err)
		}
		if _, _, err := s.Save(w); err != nil {
			t.Fatalf("Save(%q): %v", tc.name, err)
		}
		if got := w.TriggerSpec().DailyAt; got != tc.want {
			t.Errorf("TriggerSpec().DailyAt for %q = %q, want %q", tc.stored, got, tc.want)
		}
	}
	// Reopen from disk: pre-existing rows are repaired on read, no migration.
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, ok := re.Get("unpadded")
	if !ok {
		t.Fatal("workflow lost across reopen")
	}
	if spec := got.TriggerSpec(); spec.DailyAt != "09:05" {
		t.Errorf("stored daily_at normalized to %q on reload, want %q", spec.DailyAt, "09:05")
	}
}

func TestStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	s, _ := OpenStore(dir)
	if _, _, err := s.Save(graph([]Node{trigger()}, nil)); err != nil {
		t.Fatalf("Save: %v", err)
	}
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, found := re.Get("test-flow"); !found || got.TriggerNode() == nil {
		t.Fatalf("reload = %+v/%v", got, found)
	}
	gone, ok, err := re.Remove("test-flow")
	if err != nil || !ok || gone.Name != "test-flow" || re.Count() != 0 {
		t.Fatalf("Remove = %+v/%v/%v count=%d", gone, ok, err, re.Count())
	}
}
