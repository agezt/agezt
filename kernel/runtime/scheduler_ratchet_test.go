// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The plan DAG executor — the scheduler — moved into the runs module in
// W3.78 (kernel/modules/runs/scheduler).
//
// The architecture gate had ALREADY placed kernel/scheduler in the runs module
// (tools/archcheck/layers.json), so the package's home was drawn long before
// the move: the path was the last thing out of place. This slice moved the
// path and nothing else — the package's body, its names, its callers and its
// own tests are exactly as they were.
//
// The gate's rule for the old path is GONE with it, so the architecture is
// fail-closed: a package that reappears at kernel/scheduler matches no
// placement rule and the gate says so instead of waving it through.
//
// These tests are the ratchet.

// TestTheSchedulerLivesInTheRunsModule pins the move itself: the old path is
// GONE, the module's is there.
func TestTheSchedulerLivesInTheRunsModule(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "scheduler")); err == nil {
		t.Fatal("the daemon kept kernel/scheduler; the plan executor moved into the runs module (W3.78)")
	}
	for _, want := range []string{
		"doc.go", "scheduler.go", "nodes.go", "scheduler_publish.go", "scheduler_run.go",
	} {
		if _, err := os.Stat(filepath.Join("..", "modules", "runs", "scheduler", want)); err != nil {
			t.Fatalf("the runs module must carry the scheduler's %s: %v", want, err)
		}
	}
}

// TestTheGateHasNoRuleForTheOldPath pins the FAIL-CLOSED side: the
// architecture's placement rule for kernel/scheduler is gone with the
// package, so a package that reappears at that path matches no rule and the
// gate says so instead of waving it through.
func TestTheGateHasNoRuleForTheOldPath(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "tools", "archcheck", "layers.json"))
	if err != nil {
		t.Fatalf("read the architecture gate: %v", err)
	}
	if strings.Contains(string(src), `"pattern": "kernel/scheduler"`) {
		t.Fatal("the architecture gate still places a package at kernel/scheduler; the plan executor lives in the runs module (W3.78)")
	}
}

// TestTheDaemonAsksTheModulesScheduler pins the crossing: no production file
// in the kernel imports the old path. The compiler would refuse it anyway —
// the package is not there — but the ratchet says it out loud, so the path
// does not creep back under another name.
//
// The needle is assembled from two pieces ON PURPOSE. A probe that rewrites
// the import path across the tree rewrites every literal spelling of it, and
// that includes this file. A single literal would come back rewritten into
// the path the ratchet is looking for, turning the check into a tautology:
// it would then flag every legal importer and miss the illegal one. Two
// pieces leave no contiguous spelling to rewrite.
func TestTheDaemonAsksTheModulesScheduler(t *testing.T) {
	const (
		needlePrefix = "agezt/kernel/"
		needleSuffix = `scheduler"`
	)
	needle := needlePrefix + needleSuffix

	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(b), needle) {
			t.Errorf("%s imports the old scheduler path; the plan executor lives in the runs module (W3.78)", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the kernel tree: %v", err)
	}
}
