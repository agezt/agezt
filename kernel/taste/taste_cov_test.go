// SPDX-License-Identifier: MIT

package taste

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ex, err := st.Create(CreateSpec{Title: "t", Body: "b"}, time.UnixMilli(1000))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Found.
	got, ok := st.Get(ex.ID)
	if !ok || got.ID != ex.ID {
		t.Fatalf("Get(%q) = %+v, %v; want the exemplar", ex.ID, got, ok)
	}
	// Not found.
	if _, ok := st.Get("does-not-exist"); ok {
		t.Error("Get(missing) should report not found")
	}
}

func TestList_TagFilterAndLimit(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.Create(CreateSpec{Title: "a", Body: "b", Tags: []string{"x"}}, time.UnixMilli(1000))
	_, _ = st.Create(CreateSpec{Title: "b", Body: "b", Tags: []string{"y"}}, time.UnixMilli(2000))
	_, _ = st.Create(CreateSpec{Title: "c", Body: "b", Tags: []string{"x"}}, time.UnixMilli(3000))

	// Tag filter narrows to the two "x"-tagged exemplars.
	if got := st.List(Filter{Tag: "x"}); len(got) != 2 {
		t.Fatalf("List(Tag=x) = %d, want 2", len(got))
	}
	// Limit caps the result set (newest first).
	got := st.List(Filter{Limit: 1})
	if len(got) != 1 {
		t.Fatalf("List(Limit=1) = %d, want 1", len(got))
	}
	if got[0].Title != "c" {
		t.Errorf("List(Limit=1)[0].Title = %q, want newest 'c'", got[0].Title)
	}
	// Scope filter narrows correctly (no scoped exemplars → empty).
	if got := st.List(Filter{Scope: "nope"}); len(got) != 0 {
		t.Errorf("List(Scope=nope) = %d, want 0", len(got))
	}
}

func TestForScope_Limit(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.Create(CreateSpec{Title: "g1", Body: "b"}, time.UnixMilli(1000))
	_, _ = st.Create(CreateSpec{Title: "g2", Body: "b"}, time.UnixMilli(2000))
	_, _ = st.Create(CreateSpec{Title: "s1", Body: "b", Scope: "builder"}, time.UnixMilli(3000))

	// Limit caps ForScope results; scoped exemplar sorts first.
	got := st.ForScope("builder", 2)
	if len(got) != 2 {
		t.Fatalf("ForScope(builder, 2) = %d, want 2", len(got))
	}
	if got[0].Scope != "builder" {
		t.Errorf("ForScope first result = %q, want scoped 'builder' first", got[0].Scope)
	}
}

func TestCreate_BodyTooLarge(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", maxBodyBytes+1)
	if _, err := st.Create(CreateSpec{Title: "t", Body: big}, time.UnixMilli(1000)); err == nil {
		t.Fatal("Create with an over-long body should error")
	}
}

func TestCreate_TitleOnlyWhitespace(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Whitespace-only title trims to empty → validation error.
	if _, err := st.Create(CreateSpec{Title: "   ", Body: "b"}, time.UnixMilli(1000)); err == nil {
		t.Fatal("Create with a whitespace-only title should error")
	}
}

func TestOpenStore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	// A zero-byte taste.json must be treated as an empty store, not a parse error.
	if err := os.WriteFile(filepath.Join(dir, "taste.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore(empty file) error = %v", err)
	}
	if got := st.List(Filter{}); len(got) != 0 {
		t.Errorf("empty-file store should list nothing, got %d", len(got))
	}
}

func TestOpenStore_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "taste.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(dir); err == nil {
		t.Fatal("OpenStore with corrupt JSON should return an error")
	}
}

func TestCleanStrings_DedupAndTrim(t *testing.T) {
	got := cleanStrings([]string{" a ", "b", " a ", "", "c"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("cleanStrings = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cleanStrings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSaveLocked_WriteError forces saveLocked's temp-write error branch by
// making the ".tmp" sibling an existing directory, so os.WriteFile fails.
// Because the test is in-package, it can build a Store with a crafted path.
func TestSaveLocked_WriteError(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "taste.json")
	// Occupy the temp path with a directory: WriteFile(storePath+".tmp") must fail.
	if err := os.Mkdir(storePath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	st := &Store{path: storePath}
	// Create calls saveLocked; the write failure must propagate and roll back the
	// in-memory append.
	if _, err := st.Create(CreateSpec{Title: "t", Body: "b"}, time.UnixMilli(1000)); err == nil {
		t.Fatal("Create should fail when the temp file cannot be written")
	}
	if got := st.List(Filter{}); len(got) != 0 {
		t.Errorf("failed Create should roll back; list = %d, want 0", len(got))
	}
}
