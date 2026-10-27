// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The CLI writes files other components read back: a skill bundle an importer
// verifies, a registry index a remote host serves, an artifact the operator
// trusts, a backup archive the restore reads. A half-written file is a broken
// file — and for two of them (the artifact, the peer artifact) it is a
// SILENT one, because nothing verifies what was written.
//
// Every one of those writes goes through the platform's atomic writer: a
// unique temp, fsynced, chmodded, renamed into place. The file on disk is
// either the whole thing or not there.
//
// The GATE holds this at PACKAGE granularity (any raw write in cmd/agt is a
// violation). The test below holds it at FILE granularity, so a package that
// stays under the count cannot hide a new raw write in a file the count no
// longer names.

// TestTheCLIHasNoRawWrites: the CLI writes no file outside the platform's
// atomic writer.
//
// The GATE holds this at PACKAGE granularity (any raw write in cmd/agt is a
// violation). The test below holds it at FILE granularity, so a package that
// stays under the count cannot hide a new raw write in a file the count no
// longer names.
//
// The rule counts three calls: os.WriteFile, os.Create and os.CreateTemp.
// The third one is the interesting one. A package that creates its OWN temp
// file is hand-rolling the atomic writer — the mechanism (temp, fsync,
// chmod, rename) that belongs in exactly one place. Before os.CreateTemp was
// in the set, a package could move its write to a temp file and the count
// would DROP without any new guarantee being enforced: the count was
// measuring less than it claimed to measure.
func TestTheCLIHasNoRawWrites(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		n := strings.Count(src, "os.WriteFile(") +
			strings.Count(src, "os.Create(") +
			strings.Count(src, "os.CreateTemp(")
		if n > 0 {
			t.Errorf("%s has %d raw write(s) — route it through the atomic writer", name, n)
		}
	}
}

// TestThePluginScaffoldIsWholeAndLeavesNoTempFile: the scaffold's four files
// are written one by one, so a crash in the middle used to leave a project
// whose `main.go` was half a program — and a retry refused to run, because
// the directory was no longer empty.
//
// The atomic writer makes each file whole or absent, and leaves no scratch
// file behind.
//
// The scaffold is also the one place in the CLI where a GENERATOR writes its
// own output: the bytes are fully determined by the templates and no
// component in the platform depends on them. It still goes through the
// atomic writer, because the gate's exemption unit is the PACKAGE — "exempt
// the generator" would have meant "exempt all of cmd/agt".
func TestThePluginScaffoldIsWholeAndLeavesNoTempFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myplugin")
	var out, errb bytes.Buffer
	if code := cmdPluginNew([]string{"myplugin", "--dir", dir}, &out, &errb); code != 0 {
		t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errb.String())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	want := []string{".gitignore", "README.md", "go.mod", "main.go"}
	if len(names) != len(want) {
		t.Fatalf("the scaffold holds %v, want exactly %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("the scaffold holds %v, want exactly %v", names, want)
		}
	}
	for _, n := range names {
		if strings.Contains(n, ".tmp") {
			t.Errorf("the atomic writer left %q behind", n)
		}
	}
}
