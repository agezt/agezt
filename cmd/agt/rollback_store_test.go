// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/platform/rollbackstore"
)

// The CLI appends rollback checkpoints to the same catalog file the daemon
// reads and writes. Before this slice the CLI carried its own copy of the
// format AND its own writer — one with a fixed "<path>.tmp" name, the M478
// race two concurrent writers lose. The daemon could read a catalog the CLI
// had written only by the luck of two formats matching.
//
// These tests hold the agreement from both sides.

// TestTheCLIAndTheDaemonShareTheRollbackCatalog: the daemon reads what the
// CLI wrote, and the CLI reads what the daemon wrote.
//
// One file, one format. The CLI's types are aliases of the platform's, so
// "the CLI's catalog" and "the daemon's catalog" are the same value — this
// test pins that the two sides still agree on the FILE, not just on the
// types.
func TestTheCLIAndTheDaemonShareTheRollbackCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback", "checkpoints.json")
	cp := rollbackCheckpoint{
		ID:           "rb-shared",
		Kind:         rollbackCheckpointKindSkill,
		Action:       "apply",
		RunID:        "run-1",
		SubjectID:    "abc123",
		SubjectName:  "triage",
		BeforeStatus: "draft",
		CreatedMS:    99,
	}
	if err := writeRollbackCatalogAt(path, rollbackCatalog{Checkpoints: []rollbackCheckpoint{cp}}); err != nil {
		t.Fatalf("the CLI write failed: %v", err)
	}

	daemonView, err := rollbackstore.LoadAt(path)
	if err != nil {
		t.Fatalf("the daemon cannot read the CLI's catalog: %v", err)
	}
	if len(daemonView.Checkpoints) != 1 {
		t.Fatalf("the daemon sees %d checkpoint(s), want 1", len(daemonView.Checkpoints))
	}
	got := daemonView.Checkpoints[0]
	if got.ID != cp.ID || got.Kind != rollbackstore.KindSkill || got.BeforeStatus != "draft" || got.RunID != "run-1" || got.SubjectName != "triage" {
		t.Fatalf("the daemon sees %+v, want the CLI's checkpoint %+v", got, cp)
	}

	// The other way round: the daemon writes, the CLI reads.
	daemonPath := filepath.Join(t.TempDir(), "rollback", "checkpoints.json")
	if err := rollbackstore.WriteAt(daemonPath, rollbackstore.Catalog{
		Checkpoints: []rollbackstore.Checkpoint{{ID: "from-daemon", Kind: rollbackstore.KindFile, CreatedMS: 5}},
	}); err != nil {
		t.Fatalf("the daemon write failed: %v", err)
	}
	cliView, err := loadRollbackCatalogAt(daemonPath)
	if err != nil {
		t.Fatalf("the CLI cannot read the daemon's catalog: %v", err)
	}
	if len(cliView.Checkpoints) != 1 || cliView.Checkpoints[0].ID != "from-daemon" {
		t.Fatalf("the CLI sees %+v, want the daemon's checkpoint", cliView.Checkpoints)
	}
}

// TestTheCLIRollbackCatalogSurvivesAStaleTempFile: the writer must not use a
// fixed "<path>.tmp" name.
//
// A leftover temp from an earlier crash — or from another writer on the same
// box — must not be able to block or corrupt this write. That is the M478
// race the platform's atomic writer removed: it writes to a UNIQUE temp and
// renames, so no two writers ever share a scratch name.
//
// The old fixed-name writer failed here outright: it tried to overwrite the
// stale temp, and a read-only file cannot be overwritten.
func TestTheCLIRollbackCatalogSurvivesAStaleTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoints.json")

	// A stale scratch name, in the shape that actually blocks a fixed-name
	// writer: a DIRECTORY sitting where the temp file would go. A fixed-name
	// writer cannot write over it (POSIX: EISDIR; Windows: ACCESS_DENIED),
	// so the write fails; the atomic writer uses a UNIQUE temp and never
	// touches it.
	//
	// A read-only FILE would not do: on Windows, CREATE_ALWAYS replaces the
	// file and drops its read-only attribute, so the fixed-name writer would
	// simply succeed.
	stale := path + ".tmp"
	if err := os.Mkdir(stale, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := writeRollbackCatalogAt(path, rollbackCatalog{
		Checkpoints: []rollbackCheckpoint{{ID: "fresh", Kind: rollbackCheckpointKindFile, CreatedMS: 7}},
	}); err != nil {
		t.Fatalf("a stale temp file blocked the write: %v", err)
	}

	cat, err := rollbackstore.LoadAt(path)
	if err != nil {
		t.Fatalf("the daemon cannot read the catalog: %v", err)
	}
	if len(cat.Checkpoints) != 1 || cat.Checkpoints[0].ID != "fresh" {
		t.Fatalf("the catalog holds %+v, want the fresh checkpoint", cat.Checkpoints)
	}
}

// TestTheCLIRollbackCatalogLeavesNoTempFileBehind: the atomic writer's
// promise.
//
// A plain write would leave no temp either, so this does not prove the
// writer on its own — the GATE does that. What this holds is the writer's
// own promise: the catalog directory holds the catalog and nothing else.
func TestTheCLIRollbackCatalogLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoints.json")

	if err := writeRollbackCatalogAt(path, rollbackCatalog{
		Checkpoints: []rollbackCheckpoint{{ID: "x", Kind: rollbackCheckpointKindFile, CreatedMS: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "checkpoints.json" {
		t.Fatalf("the directory holds %v, want exactly [checkpoints.json]", names)
	}
}
