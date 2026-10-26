// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The control plane's files are the daemon's IDENTITY on disk: the address a
// client dials, the token it authenticates with, and the prompt library the
// Chat view reads. They are written through the platform's atomic writer
// (internal/atomicfile — a unique temp file, fsync, chmod, then a rename over
// the target), so:
//
//   - a reader never observes a HALF-written file. The alternative — a
//     truncate-in-place write — leaves a window in which the file is short.
//     A short token reads as a valid-looking token that fails
//     authentication; a short address reads as an address that cannot be
//     dialled; a short prompt library reads as an EMPTY library, because the
//     persona app drops an unparseable file rather than failing the view.
//   - the file is REPLACED, so a reader that already holds it open keeps a
//     consistent view of a whole file.
//   - the mode is re-asserted on every write. os.WriteFile applies its mode
//     only when it CREATES the file, so a leftover file from an earlier run
//     kept whatever mode it had — and a leftover READ-ONLY file made the
//     daemon's own write fail outright.
//
// The tests below observe the writer's GUARANTEE, not its implementation: the
// file's identity changes when it is rewritten (a rename replaces the file, a
// truncate-in-place keeps it), the file ends up writable by its owner, and the
// content is whole.

// fileIdentity opens path and returns the identity of the OPEN file.
//
// The identity has to come from an open handle: os.Stat on Windows returns
// the same identity for a replaced path (the path stat does not carry the
// file index), while the handle's own stat does. Measured on this codebase:
// after a rename, SameFile(openStatBefore, openStatAfter) is false, and
// after a truncate-in-place write it is true.
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

// startRuntimeFiles opens a kernel over baseDir and returns a control plane
// over it, with the runtime files written.
func startRuntimeFiles(t *testing.T, baseDir string) *Server {
	t.Helper()
	k, err := runtime.Open(runtime.Config{BaseDir: filepath.Join(baseDir, "kernel"), Provider: mock.New()})
	if err != nil {
		t.Fatalf("open kernel: %v", err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, baseDir)
	s.token = "primary-token"
	return s
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

// TestTheRuntimeFilesAreReplacedNotRewritten: the daemon's address and token
// files are REPLACED by a rename, not rewritten in place. A reader that has
// the file open keeps reading the file it opened — it never sees a torn one.
func TestTheRuntimeFilesAreReplacedNotRewritten(t *testing.T) {
	dir := t.TempDir()
	s := startRuntimeFiles(t, dir)
	addrPath := filepath.Join(dir, "runtime", addrFile)
	tokenPath := filepath.Join(dir, "runtime", tokenFile)

	if err := s.writeRuntimeFiles("127.0.0.1:1111"); err != nil {
		t.Fatal(err)
	}
	addrBefore := fileIdentity(t, addrPath)
	tokenBefore := fileIdentity(t, tokenPath)

	if err := s.writeRuntimeFiles("127.0.0.1:2222"); err != nil {
		t.Fatal(err)
	}
	addrAfter := fileIdentity(t, addrPath)
	tokenAfter := fileIdentity(t, tokenPath)

	if os.SameFile(addrBefore, addrAfter) {
		t.Fatal("the address file was rewritten in place; a reader can see a torn address")
	}
	if os.SameFile(tokenBefore, tokenAfter) {
		t.Fatal("the token file was rewritten in place; a reader can see a torn token")
	}
	// The content is the NEW one, and the new address is the one the daemon
	// is serving on.
	if got, err := os.ReadFile(addrPath); err != nil || string(got) != "127.0.0.1:2222\n" {
		t.Fatalf("address file = %q, %v", string(got), err)
	}
	if got, err := os.ReadFile(tokenPath); err != nil || string(got) != "primary-token\n" {
		t.Fatalf("token file = %q, %v", string(got), err)
	}
}

// TestTheRuntimeFilesAreReplacedWhileAReaderHoldsThemOpen: the guarantee is
// not only about a reader that arrives after the write. A reader that ALREADY
// holds the file open keeps a consistent view: it never reads a truncated
// file.
//
// The OUTCOME of the rewrite differs by platform, and both outcomes are
// correct:
//
//   - POSIX: the rename succeeds over the open file, the path carries the new
//     content and the held handle keeps the old one.
//   - Windows: the platform's atomic writer REFUSES to replace a file another
//     handle holds open (internal/atomicfile: rename fails with "Access is
//     denied", and its remove-then-rename fallback cannot remove a file that
//     is open either), so the write is refused and the old file stays whole.
//
// The invariant both share is the one a reader depends on: the held handle
// reads a COMPLETE file, and the path holds a COMPLETE file.
func TestTheRuntimeFilesAreReplacedWhileAReaderHoldsThemOpen(t *testing.T) {
	dir := t.TempDir()
	s := startRuntimeFiles(t, dir)
	addrPath := filepath.Join(dir, "runtime", addrFile)
	if err := s.writeRuntimeFiles("127.0.0.1:1111"); err != nil {
		t.Fatal(err)
	}

	held, err := os.Open(addrPath)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	writeErr := s.writeRuntimeFiles("127.0.0.1:2222")

	// The held handle reads a whole file — never a truncated one.
	buf := make([]byte, 64)
	n, err := held.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "127.0.0.1:1111\n" {
		t.Fatalf("the held handle reads %q; a reader can see a torn address", got)
	}
	// The path holds a whole file too: the new one where the rename went
	// through, the old one where the write was refused.
	onPath, err := os.ReadFile(addrPath)
	if err != nil {
		t.Fatal(err)
	}
	switch string(onPath) {
	case "127.0.0.1:1111\n":
		if writeErr == nil {
			t.Fatal("the write reported success but the address did not change")
		}
	case "127.0.0.1:2222\n":
		if writeErr != nil {
			t.Fatalf("the write failed: %v", writeErr)
		}
	default:
		t.Fatalf("the address file is torn: %q", string(onPath))
	}
}

// TestTheRuntimeFilesAreWritableByTheirOwner: the daemon writes its own
// identity files on every start, so a leftover file must not be able to stop
// the write. os.WriteFile applies its mode only when it CREATES the file: a
// leftover READ-ONLY file therefore kept its mode, and the next write either
// failed outright (the file could not be opened for writing) or left the
// wider mode in place.
//
// The atomic writer writes a fresh temp file, chmods it to the requested
// mode and renames it over the target, so the result is always writable by
// its owner — and the write never depends on the old file's mode.
func TestTheRuntimeFilesAreWritableByTheirOwner(t *testing.T) {
	dir := t.TempDir()
	s := startRuntimeFiles(t, dir)
	addrPath := filepath.Join(dir, "runtime", addrFile)
	tokenPath := filepath.Join(dir, "runtime", tokenFile)
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		t.Fatal(err)
	}

	// Leftover files from an earlier run that nobody may write.
	if err := os.WriteFile(addrPath, []byte("stale\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("stale\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := s.writeRuntimeFiles("127.0.0.1:1111"); err != nil {
		t.Fatalf("a leftover read-only file stopped the daemon's own write: %v", err)
	}
	if !ownerWritable(t, addrPath) {
		t.Fatal("the address file is not writable by its owner")
	}
	if !ownerWritable(t, tokenPath) {
		t.Fatal("the token file is not writable by its owner")
	}
	if got, err := os.ReadFile(addrPath); err != nil || string(got) != "127.0.0.1:1111\n" {
		t.Fatalf("address file = %q, %v", string(got), err)
	}
	if got, err := os.ReadFile(tokenPath); err != nil || string(got) != "primary-token\n" {
		t.Fatalf("token file = %q, %v", string(got), err)
	}
}

// TestTheRuntimeTokenIsPrivateToItsOwner: the token is the daemon's primary
// credential — it authorizes everything, on any tenant — so the file must not
// be readable by anyone else.
//
// The permission BITS are a POSIX property; Windows has no group/other
// distinction, so this check runs where the bits exist.
func TestTheRuntimeTokenIsPrivateToItsOwner(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission bits")
	}
	dir := t.TempDir()
	s := startRuntimeFiles(t, dir)
	tokenPath := filepath.Join(dir, "runtime", tokenFile)
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// A leftover file from an earlier run, with a wider mode.
	if err := os.WriteFile(tokenPath, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.writeRuntimeFiles("127.0.0.1:1111"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("the token file is readable by others: %v", mode)
	}
}

// TestThePromptLibraryIsReplacedNotRewritten: the prompt library is read
// back on every Chat view, and an unparseable library reads as an EMPTY
// library rather than as an error. A truncate-in-place write therefore loses
// the operator's prompts silently; the atomic writer replaces the file, so
// the reader sees the old library or the new one.
func TestThePromptLibraryIsReplacedNotRewritten(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: filepath.Join(dir, "kernel"), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"

	library := filepath.Join(dir, "chat_prompts.json")
	save := func(prompts string) {
		t.Helper()
		resp := callAppHost(t, s, Request{ID: "p", Cmd: CmdPromptsSet, Token: "primary", Args: map[string]any{"prompts": json.RawMessage(prompts)}})[0]
		if resp.Type != RespResult {
			t.Fatal(resp.Error)
		}
	}
	save(`[{"title":"a","text":"first"}]`)
	before := fileIdentity(t, library)
	save(`[{"title":"a","text":"second"}]`)
	after := fileIdentity(t, library)

	if os.SameFile(before, after) {
		t.Fatal("the prompt library was rewritten in place; a reader can see a torn library")
	}
	got, err := os.ReadFile(library)
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]string
	if err := json.Unmarshal(got, &items); err != nil {
		t.Fatalf("the library is not parseable: %v", err)
	}
	if len(items) != 1 || items[0]["text"] != "second" {
		t.Fatalf("the library = %v", items)
	}
}

// TestThePromptLibraryIsWritableByItsOwner: the library holds the operator's
// own prompt text and is written 0600, so a leftover file must not stop the
// save — and the result must be writable by its owner.
func TestThePromptLibraryIsWritableByItsOwner(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: filepath.Join(dir, "kernel"), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	library := filepath.Join(dir, "chat_prompts.json")
	if err := os.WriteFile(library, []byte("[]"), 0o400); err != nil {
		t.Fatal(err)
	}
	resp := callAppHost(t, s, Request{ID: "p", Cmd: CmdPromptsSet, Token: "primary", Args: map[string]any{"prompts": json.RawMessage(`[{"title":"a","text":"b"}]`)}})[0]
	if resp.Type != RespResult {
		t.Fatal(resp.Error)
	}
	if !ownerWritable(t, library) {
		t.Fatal("the prompt library is not writable by its owner")
	}
}

// TestThePromptLibraryIsPrivateToItsOwner: the library holds the operator's
// own text and is written 0600. The permission BITS are a POSIX property.
func TestThePromptLibraryIsPrivateToItsOwner(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission bits")
	}
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: filepath.Join(dir, "kernel"), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	library := filepath.Join(dir, "chat_prompts.json")
	if err := os.WriteFile(library, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp := callAppHost(t, s, Request{ID: "p", Cmd: CmdPromptsSet, Token: "primary", Args: map[string]any{"prompts": json.RawMessage(`[{"title":"a","text":"b"}]`)}})[0]
	if resp.Type != RespResult {
		t.Fatal(resp.Error)
	}
	info, err := os.Stat(library)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("the library is readable by others: %v", mode)
	}
}
