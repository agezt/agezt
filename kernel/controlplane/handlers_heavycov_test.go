// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestACPAgents_Discovery drives handleACPAgents. Detection returns an inventory
// struct regardless of whether any ACP agent is installed, so the call always
// succeeds and yields a map.
func TestACPAgents_Discovery(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdACPAgents, map[string]any{})
	if err != nil {
		t.Fatalf("acp_agents: %v", err)
	}
	if res == nil {
		t.Fatal("acp_agents returned nil result")
	}
}

// TestCatalogList exercises handleCatalogList: it returns the provider catalog
// with credential presence flags. Always succeeds with a non-nil providers list.
func TestCatalogList(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdCatalogList, map[string]any{})
	if err != nil {
		t.Fatalf("catalog_list: %v", err)
	}
	if _, ok := res["providers"]; !ok {
		t.Errorf("catalog_list result missing providers key: %v", res)
	}
}

// TestCatalogDiscover_BadEndpoint drives handleCatalogDiscover down its error /
// empty path by pointing it at an unreachable endpoint. It must not panic and
// must return either an error or a result (no models discovered).
func TestCatalogDiscover_BadEndpoint(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// A syntactically valid but unreachable endpoint — Ollama /api/tags probe
	// fails fast. We only assert the handler is exercised without panicking.
	_, _ = c.Call(context.Background(), controlplane.CmdCatalogDiscover, map[string]any{
		"endpoint": "http://127.0.0.1:1/",
	})
}

// TestAutonomyFeed drives handleAutonomyFeed — the newest-first daemon timeline.
// With a fresh kernel the feed is empty but the handler still returns a result.
func TestAutonomyFeed(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdAutonomyFeed, map[string]any{})
	if err != nil {
		t.Fatalf("autonomy_feed: %v", err)
	}
	if res == nil {
		t.Fatal("autonomy_feed returned nil result")
	}
}

// TestBoardHelp drives handleBoardHelp — the open help requests view. Fresh
// kernel has none, but the handler returns successfully.
func TestBoardHelp(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdBoardHelp, map[string]any{})
	if err != nil {
		t.Fatalf("board_help: %v", err)
	}
	if res == nil {
		t.Fatal("board_help returned nil result")
	}
}

// TestChainsSet_Errors covers handleChainsSet validation branches: missing
// chains arg, invalid chain name, and a default that names a non-existent chain.
func TestChainsSet_Errors(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	// Missing chains → required error.
	if _, err := c.Call(context.Background(), controlplane.CmdChainsSet, map[string]any{}); err == nil ||
		!strings.Contains(err.Error(), "chains") {
		t.Errorf("missing chains: err = %v, want chains-required", err)
	}

	// Invalid chain name (upper-case) → rejected.
	if _, err := c.Call(context.Background(), controlplane.CmdChainsSet, map[string]any{
		"chains": map[string]any{"BadName": []any{"mock/model"}},
	}); err == nil {
		t.Error("invalid chain name: expected error")
	}

	// default names an undefined chain → rejected.
	if _, err := c.Call(context.Background(), controlplane.CmdChainsSet, map[string]any{
		"chains":  map[string]any{"fast": []any{"mock/model"}},
		"default": "does-not-exist",
	}); err == nil {
		t.Error("bad default chain: expected error")
	}
}

// TestChainsSet_OK covers the happy path: a valid chain definition persists.
func TestChainsSet_OK(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdChainsSet, map[string]any{
		"chains":  map[string]any{"fast": []any{"mock/model", "mock/model2"}},
		"default": "fast",
	})
	if err != nil {
		t.Fatalf("chains_set ok: %v", err)
	}
	if res == nil {
		t.Fatal("chains_set returned nil result")
	}
}
