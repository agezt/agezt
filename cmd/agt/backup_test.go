// SPDX-License-Identifier: MIT

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
)

// makeHomeWithJournal builds a minimal home with a valid 1-event journal, a
// catalog file, and a root-level secret that must never reach a backup.
func makeHomeWithJournal(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	jdir := filepath.Join(home, "journal")
	if err := os.MkdirAll(jdir, 0o755); err != nil {
		t.Fatal(err)
	}
	e := mkChainEvent(t, 0, event.GenesisHash, map[string]any{"n": 0})
	line, _ := json.Marshal(e)
	line = append(line, '\n')
	if err := os.WriteFile(filepath.Join(jdir, "00000001.jsonl"), line, 0o644); err != nil {
		t.Fatal(err)
	}
	cdir := filepath.Join(home, "catalog")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "api.json"), []byte(`{"anthropic":{"id":"anthropic"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "creds.json"), []byte(`{"AGEZT_API_KEY":"sk-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestBackupRestore_RoundTrip(t *testing.T) {
	home := makeHomeWithJournal(t)
	seq, hash, err := verifyHomeJournal(home)
	if err != nil {
		t.Fatalf("verify source: %v", err)
	}

	var buf bytes.Buffer
	man, err := createBackup(home, &buf, seq, hash, time.UnixMilli(1_700_000_000_000))
	if err != nil {
		t.Fatalf("createBackup: %v", err)
	}
	if len(man.Includes) != 2 {
		t.Errorf("includes = %v, want journal+catalog", man.Includes)
	}

	if archiveContains(t, buf.Bytes(), "creds.json") {
		t.Fatal("SECURITY: backup archive contains creds.json")
	}
	if !archiveContains(t, buf.Bytes(), "00000001.jsonl") {
		t.Error("archive missing the journal segment")
	}

	dest := t.TempDir()
	rman, err := restoreBackup(bytes.NewReader(buf.Bytes()), dest)
	if err != nil {
		t.Fatalf("restoreBackup: %v", err)
	}
	if rman.JournalHeadSeq != seq {
		t.Errorf("restored manifest head seq=%d want %d", rman.JournalHeadSeq, seq)
	}
	rseq, rhash, err := verifyHomeJournal(dest)
	if err != nil {
		t.Fatalf("restored journal does not verify: %v", err)
	}
	if rseq != seq || rhash != hash {
		t.Errorf("restored head (%d,%s) != source (%d,%s)", rseq, rhash, seq, hash)
	}
	if _, err := os.Stat(filepath.Join(dest, "catalog", "api.json")); err != nil {
		t.Errorf("catalog not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "creds.json")); !os.IsNotExist(err) {
		t.Errorf("creds.json should not exist after restore")
	}
}

func TestIsAllowedBackupPath(t *testing.T) {
	for _, g := range []string{"journal/00000001.jsonl", "catalog/api.json"} {
		if !isAllowedBackupPath(g) {
			t.Errorf("%q should be allowed", g)
		}
	}
	for _, b := range []string{"../etc/passwd", "/etc/passwd", "journal/../../x", "runtime/control.token", "creds.json", `..\win`} {
		if isAllowedBackupPath(b) {
			t.Errorf("%q should be rejected", b)
		}
	}
}

func TestRestore_RejectsTraversalArchive(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = writeTarFile(tw, "../escape.txt", []byte("evil"))
	tw.Close()
	gz.Close()

	dest := t.TempDir()
	if _, err := restoreBackup(bytes.NewReader(buf.Bytes()), dest); err == nil {
		t.Fatal("restore should reject a path-traversal archive")
	}
}

func archiveContains(t *testing.T, data []byte, sub string) bool {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		if filepath.Base(hdr.Name) == sub || hdr.Name == sub {
			return true
		}
	}
	return false
}
