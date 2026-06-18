// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/plugins/builtinmarket"
)

// newTestMarketManager builds a manager over the built-in Official catalogue
// with a temp store and no live Forge/MCP — enough for discovery (search/show).
func newTestMarketManager(t *testing.T) *market.Manager {
	t.Helper()
	store := market.NewStore(t.TempDir())
	return market.NewManager(market.Config{
		Library: market.NewCompositeLibrary(builtinmarket.New(), store),
		Store:   store,
	})
}

func TestMarketToolSearch(t *testing.T) {
	mgr := newTestMarketManager(t)
	tool := &marketTool{manager: func() *market.Manager { return mgr }}
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"search","query":"web"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %s", res.Output)
	}
	// The built-in catalogue carries web-research packs; a "web" query must hit.
	if !strings.Contains(res.Output, "web-research") {
		t.Fatalf("search output missing web-research pack:\n%s", res.Output)
	}
	if !strings.Contains(res.Output, "op=install") {
		t.Fatalf("search output should guide toward install:\n%s", res.Output)
	}
}

func TestMarketToolShow(t *testing.T) {
	mgr := newTestMarketManager(t)
	tool := &marketTool{manager: func() *market.Manager { return mgr }}
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"show","pack":"git-workshop"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "git-workshop") || !strings.Contains(strings.ToLower(res.Output), "skill") {
		t.Fatalf("show output unexpected:\n%s", res.Output)
	}
}

func TestMarketToolUnavailable(t *testing.T) {
	tool := &marketTool{} // no manager wired (daemon never called SetMarket)
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"search"}`))
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not available") {
		t.Fatalf("expected unavailable error, got: %+v", res)
	}
}

func TestMarketToolUnknownOp(t *testing.T) {
	mgr := newTestMarketManager(t)
	tool := &marketTool{manager: func() *market.Manager { return mgr }}
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"frobnicate"}`))
	if !res.IsError {
		t.Fatalf("expected error for unknown op, got: %s", res.Output)
	}
}
