// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
)

// `agt doctor --repair` writes the daemon's memory store. The daemon reads
// that exact file back on every boot, so the CLI is not "writing an output
// file" — it is writing a store the daemon owns. Two writers for one file
// is the bug this slice removes: the CLI used to hand-roll the format, the
// mode and the (non-atomic) write, while the daemon used the store
// framework.
//
// These tests hold the agreement between the two sides. The GATE holds the
// writer: the site is a raw-write violation if it ever goes back to
// os.WriteFile.

// TestTheDoctorRepairWritesTheStoreTheDaemonOpens: after a repair, the
// daemon's own store opens the file the CLI wrote.
//
// This is the property that matters: the CLI's repair is not "some JSON the
// CLI likes", it is a store the daemon can take over.
func TestTheDoctorRepairWritesTheStoreTheDaemonOpens(t *testing.T) {
	base := t.TempDir()

	if c := checkMemoryStoreFile(base, true); c.Status != statusOK {
		t.Fatalf("the repair should report OK, got %+v", c)
	}

	store, err := memory.Open(filepath.Join(base, "memory"))
	if err != nil {
		t.Fatalf("the daemon cannot open the store the CLI wrote: %v", err)
	}
	defer store.Close()

	all, err := store.All()
	if err != nil {
		t.Fatalf("the daemon cannot read the repaired store: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("the repaired store holds %d record(s), want none", len(all))
	}
}

// TestTheDoctorRepairWritesTheBytesTheDaemonWrites: the CLI's repaired file
// is byte-identical to the one the daemon's own store writer produces.
//
// If the two ever diverge, the daemon rewrites the file on its next
// mutation and the CLI's repair turns into a no-op — or a diff the operator
// cannot explain. One file, one format, one writer.
func TestTheDoctorRepairWritesTheBytesTheDaemonWrites(t *testing.T) {
	base := t.TempDir()
	if c := checkMemoryStoreFile(base, true); c.Status != statusOK {
		t.Fatalf("the repair should report OK, got %+v", c)
	}
	cliBytes, err := os.ReadFile(filepath.Join(base, "memory", "memory.json"))
	if err != nil {
		t.Fatalf("the CLI wrote no store file: %v", err)
	}

	// The daemon's own writer: open a store, mutate it, and let the store
	// snapshot itself — the path every memory write takes.
	daemonDir := t.TempDir()
	store, err := memory.Open(daemonDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Put(memory.Record{ID: "r1", Content: "c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Delete("r1"); err != nil {
		t.Fatal(err)
	}
	daemonBytes, err := os.ReadFile(filepath.Join(daemonDir, "memory.json"))
	if err != nil {
		t.Fatalf("the daemon wrote no store file: %v", err)
	}

	if !bytes.Equal(cliBytes, daemonBytes) {
		t.Fatalf("the CLI wrote %q, the daemon writes %q — one file, two formats", cliBytes, daemonBytes)
	}
}

// TestTheDoctorBOMRepairKeepsTheRecords: normalizing the file must not lose
// what was in it.
//
// The repair re-serializes the store through the store framework, so the
// records have to survive the round trip. A BOM fix that emptied the store
// would be a data loss wearing a repair's clothes.
func TestTheDoctorBOMRepairKeepsTheRecords(t *testing.T) {
	base := t.TempDir()
	memDir := filepath.Join(base, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	record := `{"r1":{"id":"r1","type":"FACT","subject":"s","content":"kept"}}`
	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte(record+"\n")...)
	if err := os.WriteFile(filepath.Join(memDir, "memory.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}

	if c := checkMemoryStoreFile(base, true); c.Status != statusOK || !strings.Contains(c.Detail, "BOM removed") {
		t.Fatalf("the BOM repair should report OK, got %+v", c)
	}

	raw, err := os.ReadFile(filepath.Join(memDir, "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("the repair left the BOM in place")
	}

	store, err := memory.Open(memDir)
	if err != nil {
		t.Fatalf("the daemon cannot open the repaired store: %v", err)
	}
	defer store.Close()

	got, ok, err := store.Get("r1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("the BOM repair lost the record")
	}
	if got.Content != "kept" {
		t.Fatalf("the record's content is %q, want %q", got.Content, "kept")
	}
}

// TestTheDoctorRepairLeavesNoTempFileBehind: the atomic writer's promise.
//
// A plain os.WriteFile would leave no temp either, so this does not prove
// the writer on its own — the GATE does that. What this holds is the
// writer's own promise: a repair never leaves a scratch file next to the
// store it just wrote.
func TestTheDoctorRepairLeavesNoTempFileBehind(t *testing.T) {
	base := t.TempDir()
	if c := checkMemoryStoreFile(base, true); c.Status != statusOK {
		t.Fatalf("the repair should report OK, got %+v", c)
	}

	entries, err := os.ReadDir(filepath.Join(base, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "memory.json" {
		t.Fatalf("the memory dir holds %v, want exactly [memory.json]", names)
	}
	for _, n := range names {
		if strings.Contains(n, ".tmp") {
			t.Errorf("the atomic writer left %q behind", n)
		}
	}
}
