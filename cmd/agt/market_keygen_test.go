// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/market"
)

// TestTheMarketKeygenWritesAKeyThePublishPathAccepts: the key the keygen
// writes must be a key the publish path can use.
//
// `agt market publish --key <file>` reads the file and hands it to
// market.PrivateKeyFromHex. A keygen that wrote something the publish path
// rejects would leave the operator with a signing identity they cannot use,
// and the two commands are only ever run together.
func TestTheMarketKeygenWritesAKeyThePublishPathAccepts(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "operator")

	var stdout, stderr bytes.Buffer
	if code := cmdMarketKeygen([]string{"--out", prefix}, &stdout, &stderr); code != 0 {
		t.Fatalf("market keygen exited %d: %s", code, stderr.String())
	}

	keyBytes, err := os.ReadFile(prefix + ".key")
	if err != nil {
		t.Fatalf("read the key: %v", err)
	}
	if _, err := market.PrivateKeyFromHex(string(keyBytes)); err != nil {
		t.Fatalf("the publish path rejects the written key: %v", err)
	}

	pubBytes, err := os.ReadFile(prefix + ".pub")
	if err != nil {
		t.Fatalf("read the public key: %v", err)
	}
	pub := strings.TrimSpace(string(pubBytes))
	if len(pub) != 64 {
		t.Errorf("the public key is %d hex chars, want 64", len(pub))
	}
}

// TestTheMarketKeygenLeavesNoTempFileBehind: the write goes through the
// platform's atomic writer — a unique temp, fsynced, renamed into place.
//
// A plain os.WriteFile would leave no temp either, so this does not prove
// the writer on its own: the GATE does that (the site is a raw-write
// violation if it ever goes back to os.WriteFile). What this holds is the
// writer's own promise: no temp file survives the write.
func TestTheMarketKeygenLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "operator")

	var stdout, stderr bytes.Buffer
	if code := cmdMarketKeygen([]string{"--out", prefix}, &stdout, &stderr); code != 0 {
		t.Fatalf("market keygen exited %d: %s", code, stderr.String())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"operator.key", "operator.pub"}
	if len(names) != len(want) {
		t.Fatalf("the directory holds %v, want exactly %v", names, want)
	}
	for _, n := range names {
		if strings.Contains(n, ".tmp") {
			t.Errorf("the atomic writer left %q behind", n)
		}
	}
}

// TestTheMarketKeygenKeepsTheKeyPrivate: the key file carries the store
// framework's posture — 0600 — and the public key is world-readable.
//
// The mode bits do not exist on Windows (Go maps only the read-only bit), so
// the assertion is POSIX-only. What Windows gets instead is the directory's
// own protection, and the repo's answer to a secret that must not be
// readable at all is the ENCRYPTED vault — not a mode bit.
func TestTheMarketKeygenKeepsTheKeyPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits do not apply on Windows")
	}
	dir := t.TempDir()
	prefix := filepath.Join(dir, "operator")

	var stdout, stderr bytes.Buffer
	if code := cmdMarketKeygen([]string{"--out", prefix}, &stdout, &stderr); code != 0 {
		t.Fatalf("market keygen exited %d: %s", code, stderr.String())
	}

	keyInfo, err := os.Stat(prefix + ".key")
	if err != nil {
		t.Fatal(err)
	}
	if perm := keyInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("the signing key's mode is %o, want 600", perm)
	}
	pubInfo, err := os.Stat(prefix + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if perm := pubInfo.Mode().Perm(); perm != 0o644 {
		t.Errorf("the public key's mode is %o, want 644", perm)
	}
}

// TestTheMarketKeygenRejectsAnUnknownArgument: an unexpected argument is an
// error, not a silent success.
func TestTheMarketKeygenRejectsAnUnknownArgument(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := cmdMarketKeygen([]string{"--nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("the exit code is %d, want 2", code)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the failed keygen wrote %v", entries)
	}
}
