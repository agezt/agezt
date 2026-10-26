// SPDX-License-Identifier: MIT

package codeexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/warden"
)

// The workspace files are what the sandbox RUNS: the entry script, the extra
// files the operator's code reads, and the stdin file the program reads. They
// are read by the interpreter that executes them — or, for the SSH/K8s/Modal/
// Daytona backends, by another MACHINE that receives them.
//
// A write that truncates in place leaves a window in which the file is
// short, and a short script is not an error the interpreter can report: it
// may be a valid prefix that runs and stops early. The platform's atomic
// writer (internal/atomicfile) replaces the file instead, so the child always
// reads a WHOLE file — the old one or the new one — and the mode is
// re-asserted on every write, so a file left behind by an earlier run (a
// project directory keeps its files) cannot keep a wider mode or stop the
// write outright.
//
// The tests below observe the writer's GUARANTEE, not its implementation: the
// file's identity changes when it is rewritten (a rename replaces the file, a
// truncate-in-place keeps it), the file ends up writable by its owner, and the
// content is whole.

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

// projectDir is the workspace of a named project: it survives across calls,
// which is what makes a leftover file possible.
func projectDir(t *testing.T, tl *Tool, project string) string {
	t.Helper()
	return filepath.Join(tl.SandboxRoot, "projects", slug(project))
}

// TestTheWorkspaceScriptIsReplacedNotRewritten: the script the child runs is
// REPLACED by a rename, not rewritten in place. A reader that has the file
// open keeps reading the file it opened — it never sees a torn one.
func TestTheWorkspaceScriptIsReplacedNotRewritten(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	if _, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(1)", "project": "demo"}); isErr {
		t.Fatalf("unexpected error result")
	}
	entry := filepath.Join(fw.last.WorkDir, "main.py")
	before := fileIdentity(t, entry)

	if _, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(2)", "project": "demo"}); isErr {
		t.Fatalf("unexpected error result")
	}
	after := fileIdentity(t, entry)

	if os.SameFile(before, after) {
		t.Fatal("the script was rewritten in place; the child can read a torn script")
	}
	got, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "print(2)" {
		t.Fatalf("script = %q, want %q", string(got), "print(2)")
	}
}

// TestTheWorkspaceScriptIsWholeWhileAReaderHoldsItOpen: the guarantee is not
// only about a reader that arrives after the write. A reader that ALREADY
// holds the file open keeps a consistent view: it never reads a truncated
// script.
//
// The OUTCOME of the rewrite differs by platform, and both outcomes are
// correct:
//
//   - POSIX: the rename succeeds over the open file, the path carries the new
//     content and the held handle keeps the old one.
//   - Windows: the platform's atomic writer REFUSES to replace a file another
//     handle holds open (rename fails with "Access is denied", and its
//     remove-then-rename fallback cannot remove a file that is open either),
//     so the write is refused and the old file stays whole.
//
// The invariant both share is the one the child depends on: the held handle
// reads a COMPLETE file, and the path holds a COMPLETE file.
func TestTheWorkspaceScriptIsWholeWhileAReaderHoldsItOpen(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	if _, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(1)", "project": "demo"}); isErr {
		t.Fatalf("unexpected error result")
	}
	entry := filepath.Join(fw.last.WorkDir, "main.py")

	held, err := os.Open(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	_, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(2)", "project": "demo"})
	if isErr {
		// Windows refuses the replacement while a reader holds the file;
		// the old file stays whole, which is the invariant.
		t.Log("the rewrite was refused while a reader held the file open")
	}

	// The held handle reads a whole file — never a truncated one.
	buf := make([]byte, 64)
	n, err := held.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "print(1)" {
		t.Fatalf("the held handle reads %q; the child can read a torn script", got)
	}
	// The path holds a whole file too: the new one where the rename went
	// through, the old one where the write was refused.
	onPath, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	switch string(onPath) {
	case "print(1)":
	case "print(2)":
	default:
		t.Fatalf("the script is torn: %q", string(onPath))
	}
}

// TestTheWorkspaceFilesAreWritableByTheirOwner: a project workspace keeps its
// files, so a leftover file from an earlier run must not be able to stop the
// next write. os.WriteFile applies its mode only when it CREATES the file, and
// opening a read-only file for writing FAILS — on POSIX with EACCES, on
// Windows with "Access is denied" — so a leftover read-only script would stop
// the daemon from running the operator's code at all.
//
// The atomic writer writes a fresh temp file, chmods it to the requested mode
// and renames it over the target, so the result is always writable by its
// owner — and the write never depends on the old file's mode.
func TestTheWorkspaceFilesAreWritableByTheirOwner(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	dir := projectDir(t, tl, "demo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A leftover script from an earlier run that nobody may write.
	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte("print(0)"), 0o400); err != nil {
		t.Fatal(err)
	}

	out, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(1)", "project": "demo"})
	if isErr {
		t.Fatalf("a leftover read-only script stopped the run: %s", out)
	}
	entry := filepath.Join(fw.last.WorkDir, "main.py")
	if !ownerWritable(t, entry) {
		t.Fatal("the script is not writable by its owner")
	}
	got, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "print(1)" {
		t.Fatalf("script = %q, want %q", string(got), "print(1)")
	}
}

// TestTheWorkspaceFilesArePrivateToTheirOwner: the workspace holds the
// operator's own code and the secret mounts, and it is written 0600.
//
// The permission BITS are a POSIX property; Windows has no group/other
// distinction, so this check runs where the bits exist.
func TestTheWorkspaceFilesArePrivateToTheirOwner(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX permission bits")
	}
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	if _, isErr := run(t, tl, map[string]any{"language": "python", "code": "print(1)", "project": "demo"}); isErr {
		t.Fatalf("unexpected error result")
	}
	entry := filepath.Join(fw.last.WorkDir, "main.py")
	info, err := os.Stat(entry)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("the script is readable by others: %v", mode)
	}
}

// TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten: the extra files
// and the stdin file are REPLACED by a rename, not rewritten in place. The
// content check alone cannot tell the two apart — a reader that arrives
// after the write sees whole bytes either way — so the guarantee is measured
// where it lives: in the file's identity.
func TestTheWorkspaceExtraFilesAndStdinAreReplacedNotRewritten(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	first := map[string]any{
		"language": "python",
		"code":     "print(1)",
		"project":  "demo",
		"files":    map[string]any{"notes.txt": "hello"},
		"stdin":    "input-line",
	}
	if _, isErr := run(t, tl, first); isErr {
		t.Fatalf("unexpected error result")
	}
	dir := fw.last.WorkDir
	extraBefore := fileIdentity(t, filepath.Join(dir, "notes.txt"))
	stdinBefore := fileIdentity(t, filepath.Join(dir, "stdin.txt"))

	second := map[string]any{
		"language": "python",
		"code":     "print(2)",
		"project":  "demo",
		"files":    map[string]any{"notes.txt": "world"},
		"stdin":    "other-line",
	}
	if _, isErr := run(t, tl, second); isErr {
		t.Fatalf("unexpected error result")
	}
	if os.SameFile(extraBefore, fileIdentity(t, filepath.Join(dir, "notes.txt"))) {
		t.Fatal("the extra file was rewritten in place; the child can read a torn file")
	}
	if os.SameFile(stdinBefore, fileIdentity(t, filepath.Join(dir, "stdin.txt"))) {
		t.Fatal("the stdin file was rewritten in place; the child can read a torn file")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "notes.txt")); err != nil || string(got) != "world" {
		t.Fatalf("extra file = %q, %v", string(got), err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "stdin.txt")); err != nil || string(got) != "other-line" {
		t.Fatalf("stdin file = %q, %v", string(got), err)
	}
}

// TestTheWorkspaceFilesTheChildReadsAreWhole: the extra files and the stdin
// file are read by the PROGRAM, so the test reads them at the moment the
// child would read them — the fake warden stands in for the interpreter.
func TestTheWorkspaceFilesTheChildReadsAreWhole(t *testing.T) {
	tl, fw := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	var seenScript, seenExtra, seenStdin string
	fw.inspect = func(s warden.Spec) {
		read := func(name string) string {
			b, err := os.ReadFile(filepath.Join(s.WorkDir, name))
			if err != nil {
				t.Errorf("the child could not read %s: %v", name, err)
				return ""
			}
			return string(b)
		}
		seenScript = read("main.py")
		seenExtra = read("notes.txt")
		seenStdin = read("stdin.txt")
	}

	_, isErr := run(t, tl, map[string]any{
		"language": "python",
		"code":     "print(1)",
		"files":    map[string]any{"notes.txt": "hello"},
		"stdin":    "input-line",
	})
	if isErr {
		t.Fatalf("unexpected error result")
	}
	if seenScript != "print(1)" {
		t.Fatalf("the child reads the script %q", seenScript)
	}
	if seenExtra != "hello" {
		t.Fatalf("the child reads the extra file %q", seenExtra)
	}
	if seenStdin != "input-line" {
		t.Fatalf("the child reads stdin %q", seenStdin)
	}
}

// TestAFailedStdinWriteIsReported: the program reads the stdin file, so a
// write that fails must be REPORTED. A program that silently receives no
// stdin reads EOF and behaves as if the operator had typed nothing — the
// write error used to be swallowed.
func TestAFailedStdinWriteIsReported(t *testing.T) {
	tl, _ := newTool(t, map[string]string{LangPython: "/usr/bin/python3"}, true)
	dir := projectDir(t, tl, "demo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory where the stdin file belongs: the write cannot
	// succeed (an empty one would simply be removed by the writer's
	// remove-then-rename fallback).
	if err := os.MkdirAll(filepath.Join(dir, "stdin.txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt", "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, isErr := run(t, tl, map[string]any{
		"language": "python",
		"code":     "print(1)",
		"project":  "demo",
		"stdin":    "input-line",
	})
	if !isErr {
		t.Fatalf("a failed stdin write was not reported: %s", out)
	}
	if !strings.Contains(out, "stdin") {
		t.Fatalf("the failure should name the stdin file: %s", out)
	}
}
