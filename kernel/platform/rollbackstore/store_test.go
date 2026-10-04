// SPDX-License-Identifier: MIT

package rollbackstore_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/platform/rollbackstore"
)

func TestRestoreFileRetainsContentAndAbsence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "notes.txt")
	data := []byte("previous\x00bytes\n")
	cp := rollbackstore.Checkpoint{ID: "restore", Kind: rollbackstore.KindFile, Before: map[string]any{
		"abs_path": path, "exists": true, "content_b64": base64.StdEncoding.EncodeToString(data), "mode_perm": json.Number("384"),
	}}
	result, err := rollbackstore.RestoreFile(cp)
	if err != nil || !reflect.DeepEqual(result, map[string]any{"path": path, "restored": "content", "bytes": len(data)}) {
		t.Fatalf("restore result=%v error=%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || !reflect.DeepEqual(got, data) {
		t.Fatalf("restored bytes=%q error=%v", got, err)
	}
	cp.Before["exists"] = false
	for attempt := 0; attempt < 2; attempt++ {
		result, err = rollbackstore.RestoreFile(cp)
		if err != nil || !reflect.DeepEqual(result, map[string]any{"path": path, "restored": "absent"}) {
			t.Fatalf("absent result=%v error=%v", result, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("absent restore retained target: %v", err)
		}
	}
}

func TestRestoreFileValidationPrecedesWrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		before  map[string]any
		message string
	}{
		{nil, "checkpoint proof is missing file snapshot data"},
		{map[string]any{"exists": true}, "checkpoint proof is missing abs_path"},
		{map[string]any{"abs_path": root, "exists": true}, "refusing to restore over directory at " + root},
		{map[string]any{"abs_path": path, "exists": true, "content_b64": "!invalid!"}, "decode content_b64:"},
	} {
		if _, err := rollbackstore.RestoreFile(rollbackstore.Checkpoint{ID: "proof", Before: tc.before}); err == nil || !strings.HasPrefix(err.Error(), tc.message) {
			t.Errorf("validation error=%v want=%q", err, tc.message)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "keep" {
			t.Fatalf("invalid restore changed file: %q error=%v", got, err)
		}
	}
}

func TestCatalogRetainsWireDataVersionAndFind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGEZT_HOME", home)
	path, err := rollbackstore.DefaultPath()
	if err != nil || path != filepath.Join(home, filepath.FromSlash(rollbackstore.RelativePath)) {
		t.Fatalf("catalog path=%q error=%v", path, err)
	}
	if missing, err := rollbackstore.Load(); err != nil || missing.Version != 1 || len(missing.Checkpoints) != 0 {
		t.Fatalf("missing catalog=%v error=%v", missing, err)
	}
	checkpoint := rollbackstore.Checkpoint{ID: "entry", Kind: rollbackstore.KindFile, RunID: "run", SubjectID: "path", CreatedMS: 123, Before: map[string]any{"exists": false}}
	if err := rollbackstore.WriteAt(path, rollbackstore.Catalog{Checkpoints: []rollbackstore.Checkpoint{checkpoint}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.HasSuffix(string(raw), "\n") || !strings.Contains(string(raw), `"version": 1`) {
		t.Fatalf("catalog encoding=%s error=%v", raw, err)
	}
	cat, err := rollbackstore.LoadAt(path)
	if err != nil || cat.Version != 1 || !reflect.DeepEqual(cat.Checkpoints, []rollbackstore.Checkpoint{checkpoint}) {
		t.Fatalf("catalog roundtrip=%+v error=%v", cat, err)
	}
	index, found := rollbackstore.Find(cat, "entry")
	if index != 0 || found == nil || found.ID != "entry" {
		t.Fatalf("find index=%d checkpoint=%v", index, found)
	}
	if index, found := rollbackstore.Find(cat, "missing"); index != -1 || found != nil {
		t.Fatalf("missing find=%d/%v", index, found)
	}
	if err := os.WriteFile(path, []byte(`{"version":0,"checkpoints":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if cat, err := rollbackstore.LoadAt(path); err != nil || cat.Version != 1 {
		t.Fatalf("zero version=%v error=%v", cat, err)
	}
	if err := os.WriteFile(path, []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := rollbackstore.LoadAt(path); err == nil {
		t.Fatal("corrupt catalog silently accepted")
	}
}
