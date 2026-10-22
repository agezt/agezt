// SPDX-License-Identifier: MIT

package datalake

import (
	"testing"
)

// TestCountAndListCollections covers Count (present + missing), and
// ListCollections/titleOf across multiple schemas so the sort comparator runs.
func TestCountAndListCollections(t *testing.T) {
	l := newLake(t)

	// Two collections with titles that force titleOf ordering, plus one
	// title-less collection to exercise the Name fallback in titleOf.
	if _, err := l.CreateCollection(Schema{Name: "beta", Title: "Zebra"}, "a"); err != nil {
		t.Fatalf("CreateCollection beta: %v", err)
	}
	if _, err := l.CreateCollection(Schema{Name: "alpha", Title: "Apple"}, "a"); err != nil {
		t.Fatalf("CreateCollection alpha: %v", err)
	}
	if _, err := l.CreateCollection(Schema{Name: "gamma"}, "a"); err != nil {
		t.Fatalf("CreateCollection gamma: %v", err)
	}

	if _, err := l.Insert("alpha", map[string]any{"x": 1}, "a"); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := l.Insert("alpha", map[string]any{"x": 2}, "a"); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Count present collection.
	n, err := l.Count("alpha")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("Count(alpha) = %d, want 2", n)
	}
	// Count missing collection -> ErrNotFound.
	if _, err := l.Count("nope"); err != ErrNotFound {
		t.Fatalf("Count(missing) err = %v, want ErrNotFound", err)
	}

	// ListCollections sorts by lowercase title, then name.
	list := l.ListCollections()
	if len(list) != 3 {
		t.Fatalf("ListCollections len = %d, want 3", len(list))
	}
	// "Apple" < "Zebra" < "gamma" (title-less falls back to name "gamma").
	if list[0].Name != "alpha" {
		t.Fatalf("ListCollections[0] = %q, want alpha", list[0].Name)
	}
}

// TestQuerySortNumericStringAndDefault exercises Query sorting: numeric field
// (toFloat float64/int paths), string field, default created-time sort, plus
// Desc, Limit, and Offset paging.
func TestQuerySortNumericStringAndDefault(t *testing.T) {
	l := newLake(t)
	if _, err := l.CreateCollection(Schema{Name: "items", Title: "Items"}, "a"); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// Mixed numeric types to exercise toFloat's float64/int branches.
	rows := []map[string]any{
		{"n": float64(3), "name": "cherry"},
		{"n": 1, "name": "apple"},
		{"n": int64(2), "name": "banana"},
	}
	for _, r := range rows {
		if _, err := l.Insert("items", r, "a"); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	// Numeric ascending sort.
	asc, err := l.Query("items", Query{SortBy: "n"})
	if err != nil {
		t.Fatalf("Query numeric: %v", err)
	}
	if len(asc) != 3 || asc[0].Fields["name"] != "apple" || asc[2].Fields["name"] != "cherry" {
		t.Fatalf("numeric asc order wrong: %+v", asc)
	}

	// Numeric descending sort.
	desc, err := l.Query("items", Query{SortBy: "n", Desc: true})
	if err != nil {
		t.Fatalf("Query numeric desc: %v", err)
	}
	if desc[0].Fields["name"] != "cherry" {
		t.Fatalf("numeric desc[0] = %v, want cherry", desc[0].Fields["name"])
	}

	// String sort (name).
	byName, err := l.Query("items", Query{SortBy: "name"})
	if err != nil {
		t.Fatalf("Query string: %v", err)
	}
	if byName[0].Fields["name"] != "apple" {
		t.Fatalf("string sort[0] = %v, want apple", byName[0].Fields["name"])
	}

	// Default (created-time) sort + Limit + Offset paging.
	page, err := l.Query("items", Query{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("Query default paged: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("paged len = %d, want 1", len(page))
	}

	// Offset beyond the end returns an empty page.
	empty, err := l.Query("items", Query{Offset: 100})
	if err != nil {
		t.Fatalf("Query offset overflow: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("overflow page len = %d, want 0", len(empty))
	}
}

// TestErrorPathsMissingCollection covers Get/Update/Delete/Query/Schema on a
// non-existent collection (all should surface ErrNotFound / not-ok).
func TestErrorPathsMissingCollection(t *testing.T) {
	l := newLake(t)

	if _, err := l.Get("ghost", "id"); err != ErrNotFound {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", err)
	}
	if _, err := l.Update("ghost", "id", map[string]any{"a": 1}, "x"); err != ErrNotFound {
		t.Fatalf("Update(missing) err = %v, want ErrNotFound", err)
	}
	if err := l.Delete("ghost", "id"); err != ErrNotFound {
		t.Fatalf("Delete(missing) err = %v, want ErrNotFound", err)
	}
	if _, err := l.Query("ghost", Query{}); err != ErrNotFound {
		t.Fatalf("Query(missing) err = %v, want ErrNotFound", err)
	}
	if _, ok := l.Schema("ghost"); ok {
		t.Fatalf("Schema(missing) ok = true, want false")
	}
}
