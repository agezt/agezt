// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGo(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCountFileCalls(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a.go", `package a

import (
	"context"
	stdhttp "net/http"
	"os"
	"os/exec"
	_ "embed"
)

type svc struct {
	client *stdhttp.Client // injected: a type reference, NOT a violation
}

func f(ctx context.Context) {
	_ = exec.Command("git")                 // exec
	_ = exec.CommandContext(ctx, "git")     // exec
	var _ exec.Cmd                          // type reference: fine
	_ = &stdhttp.Client{}                   // http-client (construct)
	_ = new(stdhttp.Client)                 // http-client (construct)
	_ = stdhttp.DefaultClient               // http-client
	_, _ = stdhttp.Get("https://x")         // http-client
	_ = stdhttp.StatusOK                    // unrelated selector: fine
	_ = os.WriteFile("p", nil, 0o600)       // raw-write
	_, _ = os.Create("p")                   // raw-write
	_, _ = os.Open("p")                     // read: fine
}
`)
	counts := map[string]int{}
	if err := countFileCalls(filepath.Join(dir, "a.go"), counts); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{CallExec: 2, CallHTTPClient: 4, CallRawWrite: 2}
	for rule, n := range want {
		if counts[rule] != n {
			t.Errorf("%s = %d, want %d (all counts: %v)", rule, counts[rule], n, counts)
		}
	}
}

func TestCountFileCallsIgnoresUnwatchedNames(t *testing.T) {
	dir := t.TempDir()
	// A local package that happens to be called "exec" must not count: only
	// the os/exec import path is watched.
	writeGo(t, dir, "b.go", `package b

import exec "example.com/notexec"

func f() { exec.Command("x") }
`)
	counts := map[string]int{}
	if err := countFileCalls(filepath.Join(dir, "b.go"), counts); err != nil {
		t.Fatal(err)
	}
	if len(counts) != 0 {
		t.Fatalf("counts = %v, want none", counts)
	}
}

func TestScanCallsScopeAndAllow(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "x.go", `package x
import "os/exec"
func f() { exec.Command("x") }
`)
	cfg := testConfig(t)
	pkgs := []Pkg{
		{ImportPath: mod + "/kernel/modules/runs", Dir: dir, GoFiles: []string{"x.go"}},
		{ImportPath: mod + "/kernel/platform/sandbox", Dir: dir, GoFiles: []string{"x.go"}}, // allowed home
		{ImportPath: mod + "/internal/foo", Dir: dir, GoFiles: []string{"x.go"}},            // L0 exempt
		{ImportPath: mod + "/cmd/agt", Dir: dir, GoFiles: []string{"x.go"}},                 // L7 exempt
	}
	got, err := scanCalls(cfg, pkgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key() != "exec kernel/modules/runs" || got[0].N != 1 {
		t.Fatalf("scanCalls = %+v, want only the module package", got)
	}
}

func TestCompareCallsRatchet(t *testing.T) {
	found := []CallCount{
		{Rule: CallExec, Pkg: "a", N: 3}, // grew
		{Rule: CallExec, Pkg: "b", N: 1}, // shrank
		{Rule: CallExec, Pkg: "c", N: 2}, // equal
		{Rule: CallExec, Pkg: "d", N: 1}, // brand new
	}
	allowed := map[string]int{"exec a": 2, "exec b": 4, "exec c": 2, "exec gone": 5}
	d := compareCalls(found, allowed)
	if len(d.added) != 2 || !strings.Contains(strings.Join(d.added, "|"), "exec a: 3 call site(s), 2 allowlisted") {
		t.Errorf("added = %v", d.added)
	}
	if len(d.stale) != 2 {
		t.Errorf("stale = %v, want the shrunk and the vanished package", d.stale)
	}
	wantKeep := map[string]int{"exec a": 2, "exec b": 1, "exec c": 2}
	if len(d.stillAllowed) != len(wantKeep) {
		t.Errorf("stillAllowed = %v, want %v", d.stillAllowed, wantKeep)
	}
	for k, n := range wantKeep {
		if d.stillAllowed[k] != n {
			t.Errorf("stillAllowed[%s] = %d, want %d", k, d.stillAllowed[k], n)
		}
	}
}

func TestRunCountsCallSitesAgainstAllowlist(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "x.go", `package x
import "os/exec"
func f() { exec.Command("a"); exec.Command("b") }
`)
	pkgs := []Pkg{{ImportPath: mod + "/kernel/platform/bus", Dir: dir, GoFiles: []string{"x.go"}}}

	code, _, stderr, _ := runFixture(t, pkgs, "")
	if code != 1 || !strings.Contains(stderr, "exec kernel/platform/bus: 2 call site(s), 0 allowlisted") {
		t.Fatalf("unlisted call sites must fail; code=%d stderr=%s", code, stderr)
	}
}

func TestReadCallsAllowlistRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	for _, body := range []string{"exec pkg\n", "exec pkg zero\n", "exec pkg 0\n"} {
		p := filepath.Join(dir, "c.txt")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readCallsAllowlist(p); err == nil {
			t.Errorf("readCallsAllowlist accepted %q", body)
		}
	}
}
