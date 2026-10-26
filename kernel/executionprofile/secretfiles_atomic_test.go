// SPDX-License-Identifier: MIT

package executionprofile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/creds"
)

// The secret file mount is the VAULT's VALUE written into a file the CHILD
// PROCESS reads (for the container profile, a file that is mounted into the
// container). A write that truncates in place leaves a window in which the
// child can read a PARTIAL credential — an API key cut in half reads as an
// authentication failure, not as a broken file.
//
// The platform's atomic writer (internal/atomicfile) replaces the file
// instead, so the child always reads a whole secret, and the mode is
// re-asserted on every write, so a file left behind by an earlier run cannot
// keep a wider mode — and a leftover READ-ONLY file cannot stop the write
// (opening a read-only file for writing fails outright).

// fileIdentity opens path and returns the identity of the OPEN file.
//
// The identity has to come from an open handle: os.Stat on Windows returns the
// same identity for a replaced path (the path stat does not carry the file
// index), while the handle's own stat does.
func fileIdentity(t *testing.T, path string) os.FileInfo {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info
}

// ownerWritable reports whether the file at path may be written by its owner.
//
// It is the portable form of "the mode was re-asserted": on POSIX the owner
// write bit, and on Windows the read-only attribute (Go maps a mode without
// the owner write bit onto the read-only attribute).
func ownerWritable(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()&0o200 != 0
}

// vaultWithKey writes one secret into a vault and returns the base dir.
func vaultWithKey(t *testing.T, key, value string) string {
	t.Helper()
	baseDir := t.TempDir()
	vault := creds.NewStore(baseDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set(key, value); err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(); err != nil {
		t.Fatal(err)
	}
	return baseDir
}

// TestTheSecretFileIsReplacedNotRewritten: the secret file the child reads is
// REPLACED by a rename, not rewritten in place. A reader that has the file
// open keeps reading the file it opened — it never sees a half-written
// credential.
func TestTheSecretFileIsReplacedNotRewritten(t *testing.T) {
	baseDir := vaultWithKey(t, "OPENAI_API_KEY", "sk-first")
	workDir := t.TempDir()
	t.Setenv(SecretFilesLocal, "OPENAI_API_KEY:openai.key")

	_, cleanup, _, err := PrepareSecretFileMounts(baseDir, IDLocal, workDir)
	if err != nil {
		t.Fatalf("PrepareSecretFileMounts: %v", err)
	}
	defer cleanup()
	hostFile := filepath.Join(workDir, secretFilesDir, "openai.key")
	before := fileIdentity(t, hostFile)

	// A new value for the same key: the file is written again.
	vault := creds.NewStore(baseDir)
	if err := vault.Load(); err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("OPENAI_API_KEY", "sk-second"); err != nil {
		t.Fatal(err)
	}
	if err := vault.Save(); err != nil {
		t.Fatal(err)
	}
	_, cleanup2, _, err := PrepareSecretFileMounts(baseDir, IDLocal, workDir)
	if err != nil {
		t.Fatalf("PrepareSecretFileMounts: %v", err)
	}
	defer cleanup2()
	after := fileIdentity(t, hostFile)

	if os.SameFile(before, after) {
		t.Fatal("the secret file was rewritten in place; the child can read a partial credential")
	}
	got, err := os.ReadFile(hostFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sk-second" {
		t.Fatalf("secret file = %q, want %q", string(got), "sk-second")
	}
}

// TestTheSecretFileIsWritableByTheirOwner: a secret file left behind by an
// earlier run must not be able to stop the next write. os.WriteFile applies
// its mode only when it CREATES the file, and opening a read-only file for
// writing FAILS — on POSIX with EACCES, on Windows with "Access is denied" —
// so a leftover read-only secret would stop the sandbox from running with the
// operator's credential at all.
func TestTheSecretFileIsWritableByTheirOwner(t *testing.T) {
	baseDir := vaultWithKey(t, "OPENAI_API_KEY", "sk-test-secret")
	workDir := t.TempDir()
	t.Setenv(SecretFilesLocal, "OPENAI_API_KEY:openai.key")

	// A leftover file from an earlier run that nobody may write.
	hostFile := filepath.Join(workDir, secretFilesDir, "openai.key")
	if err := os.MkdirAll(filepath.Dir(hostFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostFile, []byte("stale\n"), 0o400); err != nil {
		t.Fatal(err)
	}

	_, cleanup, _, err := PrepareSecretFileMounts(baseDir, IDLocal, workDir)
	if err != nil {
		t.Fatalf("a leftover read-only secret file stopped the write: %v", err)
	}
	defer cleanup()
	if !ownerWritable(t, hostFile) {
		t.Fatal("the secret file is not writable by its owner")
	}
	got, err := os.ReadFile(hostFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sk-test-secret" {
		t.Fatalf("secret file = %q, want %q", string(got), "sk-test-secret")
	}
}

// TestTheSecretFileIsPrivateToItsOwner: the secret file holds the vault's
// value, so it must not be readable by anyone else.
//
// The permission BITS are a POSIX property; Windows has no group/other
// distinction, so this check runs where the bits exist.
func TestTheSecretFileIsPrivateToItsOwner(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission bits")
	}
	baseDir := vaultWithKey(t, "OPENAI_API_KEY", "sk-test-secret")
	workDir := t.TempDir()
	t.Setenv(SecretFilesLocal, "OPENAI_API_KEY:openai.key")

	// A leftover file from an earlier run, with a wider mode.
	hostFile := filepath.Join(workDir, secretFilesDir, "openai.key")
	if err := os.MkdirAll(filepath.Dir(hostFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostFile, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, cleanup, _, err := PrepareSecretFileMounts(baseDir, IDLocal, workDir)
	if err != nil {
		t.Fatalf("PrepareSecretFileMounts: %v", err)
	}
	defer cleanup()
	info, err := os.Stat(hostFile)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("the secret file is readable by others: %v", mode)
	}
}
