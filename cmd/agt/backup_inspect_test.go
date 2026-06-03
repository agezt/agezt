// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// inspectBackup reads back a real createBackup archive: the manifest round-trips
// and the journal segment is reported as a within-subtree entry (M266).
func TestBackupInspect_ReadsManifestAndEntries(t *testing.T) {
	home := t.TempDir()
	seg := filepath.Join(home, "journal", "00000001.jsonl")
	if err := os.MkdirAll(filepath.Dir(seg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seg, []byte("{\"seq\":1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	man, err := createBackup(home, &buf, 7, "deadbeefcafef00d", time.UnixMilli(1_700_000_000_000))
	if err != nil {
		t.Fatalf("createBackup: %v", err)
	}
	if len(man.Includes) == 0 || man.Includes[0] != "journal" {
		t.Fatalf("manifest includes = %v, want [journal]", man.Includes)
	}

	gotMan, entries, err := inspectBackup(&buf)
	if err != nil {
		t.Fatalf("inspectBackup: %v", err)
	}
	if gotMan.JournalHeadSeq != 7 || gotMan.JournalHeadHsh != "deadbeefcafef00d" {
		t.Errorf("manifest head = seq %d hash %q, want 7/deadbeefcafef00d", gotMan.JournalHeadSeq, gotMan.JournalHeadHsh)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Name != "journal/00000001.jsonl" || !entries[0].OK {
		t.Errorf("entry = %+v, want the journal segment within a known subtree", entries[0])
	}
}

// A bundle carrying an entry outside the known subtrees is flagged: the entry's
// OK is false and `agt backup inspect` exits non-zero with a tamper notice.
func TestBackupInspect_FlagsSuspiciousEntry(t *testing.T) {
	var raw bytes.Buffer
	gz := gzip.NewWriter(&raw)
	tw := tar.NewWriter(gz)
	man, _ := json.Marshal(backupManifest{Tool: "agt", FormatVersion: 1, Includes: []string{"journal"}})
	if err := writeTarFile(tw, backupManifestName, man); err != nil {
		t.Fatal(err)
	}
	if err := writeTarFile(tw, "secret/oops.txt", []byte("not a journal file")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	// inspectBackup marks the foreign entry.
	_, entries, err := inspectBackup(bytes.NewReader(raw.Bytes()))
	if err != nil {
		t.Fatalf("inspectBackup: %v", err)
	}
	if len(entries) != 1 || entries[0].OK {
		t.Fatalf("entries = %+v, want one entry flagged not-OK", entries)
	}

	// The command surfaces it and exits non-zero.
	path := filepath.Join(t.TempDir(), "tampered.tar.gz")
	if err := os.WriteFile(path, raw.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := cmdBackupInspect([]string{path}, &out, &errb); code == 0 {
		t.Errorf("exit code = 0, want non-zero for a tampered bundle")
	}
	if !strings.Contains(errb.String(), "tampered") {
		t.Errorf("stderr = %q, want a tamper notice", errb.String())
	}
}
