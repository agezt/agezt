// SPDX-License-Identifier: MIT

package datalake

import (
	"testing"
)

func newLake(t *testing.T) *Lake {
	t.Helper()
	var clock int64 = 1000
	l, err := Open(t.TempDir(), func() int64 { clock++; return clock })
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return l
}

func TestCreateInsertGetQuery(t *testing.T) {
	l := newLake(t)
	if _, err := l.CreateCollection(Schema{Name: "expenses", Title: "Expenses", View: "expense"}, "agent-1"); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	// Duplicate name is rejected.
	if _, err := l.CreateCollection(Schema{Name: "expenses"}, "agent-1"); err != ErrExists {
		t.Fatalf("duplicate create err = %v, want ErrExists", err)
	}
	// Invalid name rejected.
	if _, err := l.CreateCollection(Schema{Name: "bad/name"}, "a"); err == nil {
		t.Error("invalid collection name should be rejected")
	}

	a, err := l.Insert("expenses", map[string]any{"item": "coffee", "amount": float64(5)}, "agent-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if a.CreatedBy != "agent-1" || a.ID == "" {
		t.Fatalf("insert provenance/id wrong: %+v", a)
	}
	_, _ = l.Insert("expenses", map[string]any{"item": "lunch sandwich", "amount": float64(12)}, "agent-2")
	_, _ = l.Insert("expenses", map[string]any{"item": "bus ticket", "amount": float64(3)}, "agent-1")

	// Query all.
	all, _ := l.Query("expenses", Query{})
	if len(all) != 3 {
		t.Fatalf("query all = %d, want 3", len(all))
	}
	// Search.
	hit, _ := l.Query("expenses", Query{Search: "SANDWICH"})
	if len(hit) != 1 || hit[0].Fields["item"] != "lunch sandwich" {
		t.Fatalf("search = %+v", hit)
	}
	// Equals.
	eq, _ := l.Query("expenses", Query{Equals: map[string]any{"amount": float64(3)}})
	if len(eq) != 1 || eq[0].Fields["item"] != "bus ticket" {
		t.Fatalf("equals = %+v", eq)
	}
	// Sort by amount ascending.
	asc, _ := l.Query("expenses", Query{SortBy: "amount"})
	if asc[0].Fields["amount"] != float64(3) || asc[2].Fields["amount"] != float64(12) {
		t.Fatalf("sort asc wrong: %v %v", asc[0].Fields["amount"], asc[2].Fields["amount"])
	}
	// Limit.
	lim, _ := l.Query("expenses", Query{Limit: 2})
	if len(lim) != 2 {
		t.Fatalf("limit = %d, want 2", len(lim))
	}

	// Get.
	got, err := l.Get("expenses", a.ID)
	if err != nil || got.Fields["item"] != "coffee" {
		t.Fatalf("Get: %v %+v", err, got)
	}
}

func TestUpdateMergeAndDelete(t *testing.T) {
	l := newLake(t)
	_, _ = l.CreateCollection(Schema{Name: "tasks"}, "a")
	r, _ := l.Insert("tasks", map[string]any{"title": "ship M834", "done": false}, "a")

	// Update merges and can delete a key (nil value).
	up, err := l.Update("tasks", r.ID, map[string]any{"done": true, "title": nil, "note": "merged"}, "agent-x")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if up.Fields["done"] != true || up.Fields["note"] != "merged" {
		t.Fatalf("merge wrong: %+v", up.Fields)
	}
	if _, ok := up.Fields["title"]; ok {
		t.Errorf("nil value should delete the key, still present: %+v", up.Fields)
	}
	if up.UpdatedBy != "agent-x" || up.CreatedBy != "a" {
		t.Errorf("provenance after update: created_by=%s updated_by=%s", up.CreatedBy, up.UpdatedBy)
	}

	if err := l.Delete("tasks", r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := l.Get("tasks", r.ID); err != ErrNotFound {
		t.Errorf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestSystemCollectionProtected(t *testing.T) {
	l := newLake(t)
	_, _ = l.CreateCollection(Schema{Name: "contacts", System: true, Builtin: true}, "daemon")
	if err := l.DropCollection("contacts"); err != ErrSystem {
		t.Fatalf("drop system = %v, want ErrSystem", err)
	}
	_, _ = l.CreateCollection(Schema{Name: "scratch"}, "a")
	if err := l.DropCollection("scratch"); err != nil {
		t.Fatalf("drop normal: %v", err)
	}
}

func TestEnsureIdempotentAndPersistence(t *testing.T) {
	dir := t.TempDir()
	var clock int64 = 1
	now := func() int64 { clock++; return clock }
	l, _ := Open(dir, now)

	sc, created, err := l.EnsureCollection(Schema{Name: "notes", Title: "Notes"}, "daemon")
	if err != nil || !created || sc.Name != "notes" {
		t.Fatalf("ensure first: created=%v err=%v", created, err)
	}
	// Second ensure is a no-op (and must not wipe data).
	_, _ = l.Insert("notes", map[string]any{"text": "remember"}, "a")
	_, created2, _ := l.EnsureCollection(Schema{Name: "notes", Title: "Renamed?"}, "daemon")
	if created2 {
		t.Error("second ensure should not re-create")
	}

	// Reopen from disk: schema + records survive.
	l2, err := Open(dir, now)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	cols := l2.ListCollections()
	if len(cols) != 1 || cols[0].Name != "notes" || cols[0].Count != 1 {
		t.Fatalf("after reopen: %+v", cols)
	}
	recs, _ := l2.Query("notes", Query{})
	if len(recs) != 1 || recs[0].Fields["text"] != "remember" {
		t.Fatalf("records lost on reopen: %+v", recs)
	}
}

func TestSeedBuiltins(t *testing.T) {
	l := newLake(t)
	created, err := l.SeedBuiltins("system")
	if err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}
	want := len(BuiltinSchemas())
	if want == 0 {
		t.Fatal("no built-in schemas defined")
	}
	if len(created) != want {
		t.Fatalf("first seed created %d, want %d", len(created), want)
	}
	// Idempotent: a second seed creates nothing new.
	created2, _ := l.SeedBuiltins("system")
	if len(created2) != 0 {
		t.Errorf("second seed created %v, want none", created2)
	}
	// Built-ins are present, marked, and protected from drop.
	for _, sc := range BuiltinSchemas() {
		got, ok := l.Schema(sc.Name)
		if !ok || !got.Builtin || !got.System || got.View == "" {
			t.Fatalf("built-in %q not seeded correctly: %+v", sc.Name, got)
		}
		if err := l.DropCollection(sc.Name); err != ErrSystem {
			t.Errorf("drop built-in %q = %v, want ErrSystem", sc.Name, err)
		}
	}
	// A user can still add records to a built-in.
	if _, err := l.Insert("contacts", map[string]any{"name": "Ada"}, "agent"); err != nil {
		t.Errorf("insert into built-in: %v", err)
	}
}

func TestQueryNotFound(t *testing.T) {
	l := newLake(t)
	if _, err := l.Query("nope", Query{}); err != ErrNotFound {
		t.Errorf("query missing collection = %v, want ErrNotFound", err)
	}
	if _, err := l.Insert("nope", nil, "a"); err != ErrNotFound {
		t.Errorf("insert missing collection = %v, want ErrNotFound", err)
	}
}
