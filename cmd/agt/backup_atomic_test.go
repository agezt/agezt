// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The backup archive is a STREAM (tar + gzip), so it cannot go through the
// atomic writer's []byte API. The platform's streaming writer gives it the
// same guarantee as every other file the CLI writes: the archive on disk is
// either the whole thing or the previous good one.
//
// That guarantee is what makes the old "remove the output when the backup
// fails" cleanup wrong rather than merely redundant: under an atomic write
// the previous archive is still there after a failure, and deleting it would
// destroy exactly what the operator needs.

// TestAFailedBackupLeavesThePreviousArchiveIntact: the operator's last good
// archive survives a failed backup.
//
// The failure here is a catalog entry that cannot be read — a symlink to a
// path that does not exist. The walk Lstats it (the link itself exists), sees
// it is not a directory, and then fails to READ it, mid-stream. The journal
// is intact, so the backup gets all the way to the streaming write and fails
// there.
//
// A symlink is the portable way to make a read fail: a permission trick
// would not work, because the mode bits do not exist on Windows. Where
// symlinks are not available the test says so instead of pretending.
func TestAFailedBackupLeavesThePreviousArchiveIntact(t *testing.T) {
	home := makeHomeWithJournal(t)
	broken := filepath.Join(home, "catalog", "broken.json")
	if err := os.Symlink(filepath.Join(home, "catalog", "does-not-exist.json"), broken); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "agezt-backup.tar.gz")
	const previous = "the previous good archive"
	if err := os.WriteFile(outPath, []byte(previous), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	code := cmdBackup([]string{"--home", home, "--out", outPath}, &out, &errb)
	if code == 0 {
		t.Fatalf("the backup should have failed, stdout: %s", out.String())
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("the failed backup removed the previous archive: %v", err)
	}
	if string(got) != previous {
		t.Fatalf("the failed backup changed the archive: %q", got)
	}

	// And no scratch file was left behind next to it.
	entries, err := os.ReadDir(filepath.Dir(outPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "agezt-backup.tar.gz" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("the directory holds %v, want exactly [agezt-backup.tar.gz]", names)
	}
}

// TestTheBackupArchiveIsWholeAndPrivate: a successful backup publishes a
// complete archive, and the archive carries the store framework's posture —
// 0600.
//
// The mode was 0644 before (os.Create applies the umask and no mode at all);
// the journal is not secret, but a backup is the operator's whole activity
// history, and 0600 is the posture every other file in the product writes
// with. The mode bits do not exist on Windows, so that part is POSIX-only.
func TestTheBackupArchiveIsWholeAndPrivate(t *testing.T) {
	home := makeHomeWithJournal(t)
	outPath := filepath.Join(t.TempDir(), "agezt-backup.tar.gz")

	var out, errb bytes.Buffer
	if code := cmdBackup([]string{"--home", home, "--out", outPath}, &out, &errb); code != 0 {
		t.Fatalf("backup exit=%d stderr=%s", code, errb.String())
	}

	// The archive is readable by the inspector: it is complete, not a
	// truncated prefix.
	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	man, entries, err := inspectBackup(f)
	if err != nil {
		t.Fatalf("the published archive does not read back: %v", err)
	}
	if man.Tool == "" || len(entries) == 0 {
		t.Fatalf("the archive holds no manifest or entries: %+v / %+v", man, entries)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("the archive's mode is %o, want 600", perm)
		}
	}
}
