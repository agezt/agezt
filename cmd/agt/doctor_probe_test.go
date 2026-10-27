// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The doctor's writability probe writes a file into the daemon's home
// directory and removes it again. It is the CLI's LAST raw write — the one
// site the forbidden-call ratchet used to name — and it is now a write through
// the platform's canonical atomic writer like every other file the CLI
// writes.
//
// The probe is the one write whose PURPOSE is to be a faithful sample of what
// a write to that directory does: it must fail when the directory cannot take
// a file, and it must leave nothing behind when it succeeds or when it fails.

// TestTheDoctorProbeLeavesNothingBehind: a successful probe leaves the home
// directory exactly as it found it — no probe file, no scratch file.
//
// The scratch file matters: the atomic writer creates it in the TARGET's
// directory, which here is the daemon's home. A probe that left a temp behind
// would litter the directory it is inspecting.
func TestTheDoctorProbeLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	c := checkBaseDir(dir, nil)
	if c.Status != statusOK {
		t.Fatalf("writable dir should be OK, got %s: %s", c.Status.label(), c.Detail)
	}

	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		t.Errorf("the probe left %q in the directory it inspected", e.Name())
	}
}

// TestTheDoctorProbeFailsWhenTheTargetIsBlocked: the probe must FAIL — not
// pass — when the file it writes cannot land.
//
// The blocker is a NON-EMPTY DIRECTORY at the probe's path. A directory is the
// cross-platform blocker: on POSIX the rename fails with EISDIR, and on
// Windows the writer's fallback cannot remove it either (a non-empty
// directory cannot be removed), so the write fails on both. A permission
// trick would not do: the mode bits do not exist on Windows, and a read-only
// directory there does not stop the write.
func TestTheDoctorProbeFailsWhenTheTargetIsBlocked(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, ".doctor-probe")
	if err := os.MkdirAll(filepath.Join(blocker, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	c := checkBaseDir(dir, nil)
	if c.Status != statusFail {
		t.Fatalf("a blocked probe should FAIL, got %s: %s", c.Status.label(), c.Detail)
	}
	if !strings.Contains(c.Detail, "not writable") {
		t.Errorf("detail = %q, want it to name the unwritable directory", c.Detail)
	}

	// A failed probe leaves no scratch file behind either.
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("the failed probe left %q behind", e.Name())
		}
	}
}
