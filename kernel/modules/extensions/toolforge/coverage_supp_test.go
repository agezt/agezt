// SPDX-License-Identifier: MIT

package toolforge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scripttools.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(dir)
	if err == nil {
		t.Fatal("Open with malformed JSON should error")
	}
}

func TestList_Empty(t *testing.T) {
	s := openStore(t)
	out := s.List()
	if len(out) != 0 {
		t.Fatalf("List on empty store = %d, want 0", len(out))
	}
}

func TestList_WithItems(t *testing.T) {
	s := openStore(t)
	draft(t, s, "atool")
	draft(t, s, "btool")
	out := s.List()
	if len(out) != 2 {
		t.Fatalf("List = %d, want 2", len(out))
	}
	// Sorted by creation time: "atool" was added first.
	if out[0].Name != "atool" || out[1].Name != "btool" {
		t.Errorf("List order: got %q, want [atool btool]", []string{out[0].Name, out[1].Name})
	}
}

func TestGet_NotFound(t *testing.T) {
	s := openStore(t)
	if _, found := s.Get("nonexistent"); found {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestRecordTest_NotFound(t *testing.T) {
	s := openStore(t)
	if _, err := s.RecordTest("nonexistent", true); err == nil {
		t.Fatal("RecordTest(nonexistent) should error")
	}
}

func TestPromote_NotFound(t *testing.T) {
	s := openStore(t)
	if _, err := s.Promote("nonexistent"); err == nil {
		t.Fatal("Promote(nonexistent) should error")
	}
}

func TestQuarantine_NotFound(t *testing.T) {
	s := openStore(t)
	if _, err := s.Quarantine("nonexistent"); err == nil {
		t.Fatal("Quarantine(nonexistent) should error")
	}
}

func TestUpdate_NotFound(t *testing.T) {
	s := openStore(t)
	if _, err := s.Update("nonexistent", func(dst *ScriptTool) {}); err == nil {
		t.Fatal("Update(nonexistent) should error")
	}
}
