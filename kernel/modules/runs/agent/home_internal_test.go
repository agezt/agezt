// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// The loop is the RUNS MODULE's engine (W3.76h). It used to live at
// kernel/agent — a top-level package the archcheck layer map had to classify
// by hand, because the tree did not say so. The move makes the tree the
// statement.
//
// These tests are the ratchet: they hold the move in place.

// repoRoot is the repository root, four levels up from
// kernel/modules/runs/agent.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve the repository root: %v", err)
	}
	return root
}

// TestTheLoopLivesInTheRunsModule pins the move itself: the old home is
// GONE, the new one is here.
func TestTheLoopLivesInTheRunsModule(t *testing.T) {
	root := repoRoot(t)

	old := filepath.Join(root, "kernel", "agent")
	if _, err := os.Stat(old); err == nil {
		t.Fatal("kernel/agent still exists; the loop is the runs module's")
	}

	here := filepath.Join(root, "kernel", "modules", "runs", "agent")
	if _, err := os.Stat(here); err != nil {
		t.Fatalf("the loop's home is missing: %v", err)
	}
}

// TestTheOldLoopPathIsGoneEverywhere pins the move in the IMPORT graph: no
// file in the repository may still name the old path. A stale import is a
// compile error, but a stale mention in a comment or a document is a lie
// about where the loop lives.
func TestTheOldLoopPathIsGoneEverywhere(t *testing.T) {
	root := repoRoot(t)
	// The needle carries its closing quote: kernel/agentgw is a DIFFERENT
	// package and must not be caught by the hunt.
	const oldPath = `"github.com/agezt/agezt/kernel/agent"`

	var offenders []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "vendor", ".project":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") {
			return nil
		}
		// This file names the old path ON PURPOSE — it is the ratchet that
		// hunts for it.
		if strings.HasSuffix(path, "home_internal_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(data), oldPath) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("the old loop path is still named in %v", offenders)
	}
}

// TestTheLoopNeverNamesTheDaemon pins the module's discipline: the loop is
// in the module, and the module never reaches back into the daemon. Its
// dependencies are the contract rows, the platform and the bus.
func TestTheLoopNeverNamesTheDaemon(t *testing.T) {
	forbidden := []string{
		"github.com/agezt/agezt/kernel/runtime",
		"github.com/agezt/agezt/kernel/app",
		"github.com/agezt/agezt/kernel/controlplane",
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the loop's directory: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		// This file names the forbidden paths ON PURPOSE — it is the ratchet
		// that hunts for them.
		if e.Name() == "home_internal_test.go" {
			continue
		}
		data, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatalf("read %s: %v", e.Name(), rerr)
		}
		for _, f := range forbidden {
			if strings.Contains(string(data), f) {
				t.Fatalf("%s imports %s; the module never names the daemon", e.Name(), f)
			}
		}
	}
}

// TestTheLoopKeepsItsContractAliases pins the contract half: the aliases
// here are the CONTRACT rows, with identical types. The contract half moved
// to kernel/contract/llm and kernel/contract/toolapi in W1.1; the loop keeps
// the aliases so every agent.X reference still compiles, but the identity
// must never drift.
func TestTheLoopKeepsItsContractAliases(t *testing.T) {
	var (
		_ Provider           = llm.Provider(nil)
		_ StreamingProvider  = llm.StreamingProvider(nil)
		_ Message            = llm.Message{}
		_ CompletionRequest  = llm.CompletionRequest{}
		_ CompletionResponse = llm.CompletionResponse{}
		_ Usage              = llm.Usage{}
		_ Tool               = toolapi.Tool(nil)
		_ ToolDef            = toolapi.ToolDef{}
		_ Result             = toolapi.Result{}
		_ ObservationTrust   = toolapi.ObservationTrust("")
	)
}
