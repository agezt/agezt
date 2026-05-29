// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"github.com/agezt/agezt/plugins/tools/shell"
)

// TestPluginList_EmptyWhenNoneConfigured covers the common case
// — no AGEZT_PLUGINS env, no external plugins — and asserts the
// response is a valid empty array (not null) so jq pipelines on
// the operator side stay simple.
func TestPluginList_EmptyWhenNoneConfigured(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdPluginList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 0 {
		t.Errorf("count = %d want 0", got)
	}
	rows, ok := res["plugins"].([]any)
	if !ok {
		t.Fatalf("plugins wrong type: %T (want []any even when empty)", res["plugins"])
	}
	if len(rows) != 0 {
		t.Errorf("plugins should be empty, got %d rows", len(rows))
	}
}

// TestPluginList_ReturnsManifestSortedByPrefix covers the loaded
// path — kernel constructed with two manifest entries, response
// includes both and the order is sorted by prefix (not insertion
// order). Pins, allowlists, and args are surfaced as the CLI
// expects them.
func TestPluginList_ReturnsManifestSortedByPrefix(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{
		BaseDir:  dir,
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]agent.Tool{"shell": shell.New()},
		// Deliberately reverse-alphabetical insertion to make the
		// handler's sort visible — if it returned insertion order
		// we'd see "search" before "browser".
		Plugins: []runtime.PluginInfo{
			{
				Prefix:       "search",
				Path:         "/opt/agezt/search",
				Args:         []string{"--index", "/var/idx"},
				ToolCount:    3,
				HashPinned:   true,
				AllowedTools: []string{"search.query", "search.suggest"},
			},
			{
				Prefix:     "browser",
				Path:       "/opt/agezt/browser",
				ToolCount:  1,
				HashPinned: false,
				// AllowedTools nil → "unrestricted" in CLI render.
			},
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	srv := controlplane.NewServer(k, dir)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Stop() })

	client, err := dialUntilReady(t, dir)
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.Call(context.Background(), controlplane.CmdPluginList, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["count"]); got != 2 {
		t.Fatalf("count = %d want 2", got)
	}
	rows, _ := res["plugins"].([]any)
	if len(rows) != 2 {
		t.Fatalf("plugins len = %d want 2", len(rows))
	}
	// Sorted by prefix: browser, search.
	first, _ := rows[0].(map[string]any)
	second, _ := rows[1].(map[string]any)
	if first["prefix"] != "browser" || second["prefix"] != "search" {
		t.Fatalf("plugins not sorted: %v / %v", first["prefix"], second["prefix"])
	}
	// Spot-check that the rich fields round-trip correctly.
	if pinned, _ := second["hash_pinned"].(bool); !pinned {
		t.Error("search.hash_pinned should be true")
	}
	if pinned, _ := first["hash_pinned"].(bool); pinned {
		t.Error("browser.hash_pinned should be false")
	}
	if got := intOf(second["tool_count"]); got != 3 {
		t.Errorf("search.tool_count = %d want 3", got)
	}
	// browser had no AllowedTools — wire shape is null/empty,
	// which the CLI renders as "unrestricted". The CLI doesn't
	// distinguish "absent" from "explicit empty" — both mean no
	// restriction was wired — but the JSON must be navigable.
	if _, present := first["allowed_tools"]; !present {
		t.Error("allowed_tools key missing on browser entry")
	}
	// search has two allowed tools — verify array shape.
	allowed, _ := second["allowed_tools"].([]any)
	if len(allowed) != 2 {
		t.Errorf("search.allowed_tools len = %d want 2", len(allowed))
	}
}
