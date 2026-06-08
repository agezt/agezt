// SPDX-License-Identifier: MIT

package board

import "testing"

func TestPostReadTopics(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	st.Post("a", "researcher", "first", 100)
	st.Post("b", "", "other", 200)
	st.Post("a", "", "second", 300)

	// Topic filter, newest first.
	got := st.Read("A", 10) // case-insensitive topic match
	if len(got) != 2 || got[0].Text != "second" {
		t.Fatalf("Read(a) = %+v, want [second, first]", got)
	}
	// Unfiltered.
	if len(st.Read("", 10)) != 3 {
		t.Errorf("unfiltered Read = %d, want 3", len(st.Read("", 10)))
	}
	// Limit.
	if len(st.Read("", 1)) != 1 {
		t.Errorf("limited Read should honor limit")
	}
	tp := st.Topics()
	if tp["a"] != 2 || tp["b"] != 1 {
		t.Errorf("Topics = %+v", tp)
	}
}

func TestPersistenceAndFreshOpenSeesWrites(t *testing.T) {
	dir := t.TempDir()
	w, _ := Open(dir)
	if _, err := w.Post("t", "", "durable", 100); err != nil {
		t.Fatal(err)
	}
	// A SECOND, independent Open (mirrors the control plane's read path) sees the
	// committed write — the property the Web UI board view relies on.
	r, _ := Open(dir)
	if got := r.Read("t", 10); len(got) != 1 || got[0].Text != "durable" {
		t.Fatalf("fresh Open did not see committed write: %+v", got)
	}
}

func TestCapAtMaxMessages(t *testing.T) {
	st, _ := Open(t.TempDir())
	for i := 0; i < MaxMessages+50; i++ {
		st.Post("t", "", "m", int64(i))
	}
	if n := len(st.Read("", MaxMessages+100)); n != MaxMessages {
		t.Errorf("board grew to %d, want cap %d", n, MaxMessages)
	}
}
