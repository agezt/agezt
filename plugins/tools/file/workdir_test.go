// SPDX-License-Identifier: MIT

package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

// TestWorkdir_RebasesRelativePaths: a run carrying a per-agent workdir (M792)
// reads and writes inside <root>/<workdir>; an unscoped run is unchanged; an
// empty list path means "my directory"; containment still holds.
func TestWorkdir_RebasesRelativePaths(t *testing.T) {
	root := t.TempDir()
	tool, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := agent.WithWorkdir(context.Background(), "research")

	invoke := func(ctx context.Context, in map[string]any) agent.Result {
		t.Helper()
		raw, _ := json.Marshal(in)
		res, err := tool.Invoke(ctx, raw)
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		return res
	}

	// Write lands under root/research, not the root.
	if res := invoke(ctx, map[string]any{"op": "write", "path": "notes.txt", "content": "dug deep"}); res.IsError {
		t.Fatalf("write errored: %s", res.Output)
	}
	b, err := os.ReadFile(filepath.Join(root, "research", "notes.txt"))
	if err != nil || string(b) != "dug deep" {
		t.Fatalf("file not in the workdir: %v %q", err, b)
	}

	// Read resolves the same way.
	if res := invoke(ctx, map[string]any{"op": "read", "path": "notes.txt"}); res.IsError || !strings.Contains(res.Output, "dug deep") {
		t.Fatalf("scoped read wrong: %s", res.Output)
	}

	// An empty list path means the agent's own directory.
	if res := invoke(ctx, map[string]any{"op": "list", "path": ""}); res.IsError || !strings.Contains(res.Output, "notes.txt") {
		t.Fatalf("empty-path list should show the workdir: %s", res.Output)
	}

	// An UNscoped run still operates at the root (no leak between identities).
	if res := invoke(context.Background(), map[string]any{"op": "read", "path": "notes.txt"}); !res.IsError {
		t.Fatalf("root read of a workdir file should miss, got: %s", res.Output)
	}

	// Containment survives the rebase: ../ out of the workdir stays inside
	// root rules — escaping ROOT is still refused.
	if res := invoke(ctx, map[string]any{"op": "write", "path": "../../outside.txt", "content": "x"}); !res.IsError {
		t.Fatal("root escape via workdir-relative .. was allowed")
	}
}

// TestWithWorkdir_RefusesEscapes: the ctx setter is defense-in-depth — abs and
// any `..` shape leave the context unset.
func TestWithWorkdir_RefusesEscapes(t *testing.T) {
	for _, w := range []string{"/abs", "..", "../up", "a/../../b", "a/.."} {
		if got := agent.WorkdirFromContext(agent.WithWorkdir(context.Background(), w)); got != "" {
			t.Errorf("escaping workdir %q accepted as %q", w, got)
		}
	}
	if got := agent.WorkdirFromContext(agent.WithWorkdir(context.Background(), "team/research")); got != "team/research" {
		t.Errorf("clean workdir mangled: %q", got)
	}
}
