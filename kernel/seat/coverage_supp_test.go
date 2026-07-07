// SPDX-License-Identifier: MIT

package seat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenStore_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seats.json")
	if err := os.WriteFile(path, []byte(`not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenStore(dir)
	if err == nil {
		t.Fatal("OpenStore with malformed JSON should error")
	}
}

func TestStore_GetSeatNotFound(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestStore_DeleteNotFound(t *testing.T) {
	s, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("nonexistent"); err == nil {
		t.Fatal("Delete(nonexistent) should error")
	}
}
