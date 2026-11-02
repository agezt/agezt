// SPDX-License-Identifier: MIT

package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The ratchet below holds the W3.76j move in place. Each test is a claim
// about the tree; together they say "the run engine lives in the runs module
// and the daemon does not reach back into it".

// TestTheRunnerLivesInTheRunsModule pins the move: the package that owns the
// run engine is a child of the runs module.
func TestTheRunnerLivesInTheRunsModule(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	dir := filepath.ToSlash(wd)
	want := "/kernel/modules/runs/runner"
	if !strings.HasSuffix(dir, want) {
		t.Fatalf("the runner lives in %q, want a path ending in %q", dir, want)
	}
}

// TestTheOldRunnerPathIsGoneEverywhere scans the whole repository for the old
// import path. The needle is closed by a quote so that a longer path that
// merely starts with the same letters cannot trip it.
func TestTheOldRunnerPathIsGoneEverywhere(t *testing.T) {
	const oldPath = `"github.com/agezt/agezt/kernel/runtime/runexec"`
	repo := repoRoot(t)
	err := filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir // .dev-home, .temp_files, ...
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "home_internal_test.go") {
			return nil // this file names the old path on purpose
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(data), oldPath) {
			rel, _ := filepath.Rel(repo, path)
			t.Fatalf("%s still imports the old runner path", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan the repository: %v", err)
	}
}

// TestTheRunnerNeverNamesTheDaemon pins the circular-import guard: the
// run engine reaches its host through the KernelAPI port, so no file in this
// package may import the daemon.
func TestTheRunnerNeverNamesTheDaemon(t *testing.T) {
	const daemonPath = `"github.com/agezt/agezt/kernel/runtime"`
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the runner's directory: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			continue // this file names the daemon path on purpose
		}
		data, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		if strings.Contains(string(data), daemonPath) {
			t.Fatalf("%s imports the daemon; the runner speaks to its host through the KernelAPI port", e.Name())
		}
	}
}

// TestTheHostStillImplementsThePort pins the other half of the seam: the
// daemon is the runner's host, so the port must keep its shape. The
// assertion lives here because the port is declared here.
func TestTheHostStillImplementsThePort(t *testing.T) {
	port := reflect.TypeOf((*KernelAPI)(nil)).Elem()
	if port.NumMethod() == 0 {
		t.Fatal("the KernelAPI port is empty")
	}
	// The runner's entry points must be reachable through the port: a host
	// that does not carry them cannot drive a run.
	//
	// AgentSlugFromCtx is NOT one of them any more (W3.76m): it was the run
	// context's vocabulary, and the run context's vocabulary is the module's.
	// The run engine reads it through the module, and the host's port no
	// longer promises it — see the runner's own runctx ratchet.
	for _, name := range []string{"ClaimResumeTicket", "FinalizeResumeTicket"} {
		if _, ok := port.MethodByName(name); !ok {
			t.Fatalf("the KernelAPI port has no %s method", name)
		}
	}
	for _, gone := range []string{"AgentSlugFromCtx", "AgentRetryPolicyFromCtx"} {
		if _, ok := port.MethodByName(gone); ok {
			t.Errorf("the KernelAPI port has a %s method again — the run context's vocabulary is the module's", gone)
		}
	}
	// The ROSTER's WRITES left the port in W3.76o: they are the agents
	// module's operations, and the run engine asks the module for them (see
	// the Runner's roster field). The two roster entries nobody in this
	// module ever opened went with them — a port entry that is never opened
	// is a promise the host does not have to keep.
	for _, gone := range []string{"UpdateProfile", "SetProfileRetired", "Roster", "RosterList"} {
		if _, ok := port.MethodByName(gone); ok {
			t.Errorf("the KernelAPI port has a %s method again — the roster's writes are the agents module's, and nobody in this module opened the store", gone)
		}
	}
	// W3.76p: thirteen more promises died. Nobody in this module ever opened
	// them — the daemon keeps every one of those verbs for its other callers
	// (the application layer, the composition root, the plugins), but a PORT
	// entry is a promise to THIS module, and this module never asked.
	for _, gone := range []string{
		"Approvals", "ArtifactIndex", "CatalogStore", "ConfigCenter", "DataLake",
		"Edict", "Reflect", "RunAssured", "Schedules", "Standing", "StandingList",
		"Voice", "Warden",
	} {
		if _, ok := port.MethodByName(gone); ok {
			t.Errorf("the KernelAPI port has a %s method again — the run engine never opened it, so the host never had to promise it", gone)
		}
	}
}

// repoRoot walks up from the package's directory to the repository root (the
// directory that holds go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root")
		}
		dir = parent
	}
}
