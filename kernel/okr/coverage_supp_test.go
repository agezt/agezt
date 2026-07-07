// SPDX-License-Identifier: MIT

package okr

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenStore_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okr.json")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore(empty file): %v", err)
	}
	if s == nil {
		t.Fatal("OpenStore returned nil")
	}
}

func TestOpenStore_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okr.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenStore(dir)
	if err == nil {
		t.Fatal("OpenStore with malformed JSON should error")
	}
}

func TestOpenStore_InvalidStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "okr.json")
	data := `{"version":1,"objectives":[{"id":"o1","title":"test","status":"bogus","created_ms":1000}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenStore(dir)
	if err == nil {
		t.Fatal("OpenStore with invalid status should error")
	}
}

func TestCreate_EmptyTitleSupp(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Create(CreateSpec{Title: "  "}, time.Now())
	if err == nil {
		t.Fatal("Create with empty title should error")
	}
}

func TestMutate_EmptyID(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddKeyResult("", "kr1", 1, time.Now())
	if err == nil {
		t.Fatal("mutate with empty ID should error")
	}
}

func TestObjectivesForTask_EmptyTaskID(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ObjectivesForTask(""); got != nil {
		t.Fatalf("ObjectivesForTask('') = %v, want nil", got)
	}
}
