// SPDX-License-Identifier: MIT

package taste

import (
	"testing"
	"time"
)

func TestCreateListPersistAndDelete(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := st.Create(CreateSpec{Title: "Good PR summary", Body: "One line what, one line why.", Tags: []string{"writing"}}, time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ex.Title != "Good PR summary" || len(ex.Tags) != 1 {
		t.Fatalf("exemplar = %+v", ex)
	}
	if _, err := st.Create(CreateSpec{Title: "no body"}, time.UnixMilli(1100)); err == nil {
		t.Fatal("expected error for missing body")
	}

	// Reopen: persists.
	re, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := re.List(Filter{}); len(got) != 1 || got[0].ID != ex.ID {
		t.Fatalf("reopened list = %+v", got)
	}
	if err := re.Delete(ex.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := re.List(Filter{}); len(got) != 0 {
		t.Fatalf("after delete list = %+v", got)
	}
	if err := re.Delete(ex.ID); err != ErrNotFound {
		t.Fatalf("second delete err = %v, want ErrNotFound", err)
	}
}

func TestForScopeSelectsGlobalPlusMatching(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.Create(CreateSpec{Title: "global", Body: "applies everywhere"}, time.UnixMilli(1000))
	_, _ = st.Create(CreateSpec{Title: "builder-only", Body: "repo style", Scope: "builder"}, time.UnixMilli(2000))
	_, _ = st.Create(CreateSpec{Title: "writer-only", Body: "prose style", Scope: "writer"}, time.UnixMilli(3000))

	// A run scoped to "builder" sees global + builder, not writer.
	got := st.ForScope("builder", 0)
	if len(got) != 2 {
		t.Fatalf("ForScope(builder) = %d exemplars, want 2: %+v", len(got), got)
	}
	// Scoped exemplar sorts first (more specific).
	if got[0].Title != "builder-only" || got[1].Title != "global" {
		t.Fatalf("order = %q, %q", got[0].Title, got[1].Title)
	}

	// An unscoped run sees only global exemplars.
	if g := st.ForScope("", 0); len(g) != 1 || g[0].Title != "global" {
		t.Fatalf("ForScope(\"\") = %+v", g)
	}

	// Limit caps the selection.
	if g := st.ForScope("builder", 1); len(g) != 1 {
		t.Fatalf("ForScope limit=1 returned %d", len(g))
	}
}

func TestListFilterByScopeAndTag(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.Create(CreateSpec{Title: "a", Body: "x", Scope: "builder", Tags: []string{"code"}}, time.UnixMilli(1000))
	_, _ = st.Create(CreateSpec{Title: "b", Body: "y", Scope: "writer", Tags: []string{"prose"}}, time.UnixMilli(2000))

	if got := st.List(Filter{Scope: "builder"}); len(got) != 1 || got[0].Title != "a" {
		t.Fatalf("scope filter = %+v", got)
	}
	if got := st.List(Filter{Tag: "prose"}); len(got) != 1 || got[0].Title != "b" {
		t.Fatalf("tag filter = %+v", got)
	}
}
