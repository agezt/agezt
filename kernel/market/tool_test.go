// SPDX-License-Identifier: MIT

package market_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/marketapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/market"
	"github.com/agezt/agezt/kernel/modules/skills/skill"
	"github.com/agezt/agezt/plugins/builtinmarket"
)

// newTestMarketManager builds a manager over the built-in Official catalogue
// with a temp store and no live Forge/MCP — enough for discovery (search/show).
func newTestMarketManager(t *testing.T) *market.Manager {
	t.Helper()
	store := market.NewStore(t.TempDir())
	return market.NewManager(market.Config{
		Library: market.NewCompositeLibrary(builtinmarket.New(skill.NewBuiltins()), store),
		Store:   store,
	})
}

func TestMarketToolSearch(t *testing.T) {
	mgr := newTestMarketManager(t)
	tool := &market.Tool{Manager: func() marketapi.Manager { return mgr }}
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
	tool := &market.Tool{Manager: func() marketapi.Manager { return mgr }}
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
	tool := &market.Tool{} // no manager wired (daemon never called SetMarket)
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
	tool := &market.Tool{Manager: func() marketapi.Manager { return mgr }}
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"frobnicate"}`))
	if !res.IsError {
		t.Fatalf("expected error for unknown op, got: %s", res.Output)
	}
}

// recordingPort is a marketplace port that RECORDS what the tool asked for.
// The tool drives the CONTRACT's port, so what reaches it is the tool's own
// doing — and a tool that dropped the query or the pack name would still look
// fine over a real catalogue.
type recordingPort struct {
	marketapi.Manager // the verbs the tool does not drive are not its business
	query             string
	pack              string
	marketplace       string
	corr              string
}

func (p *recordingPort) List(query string) ([]marketapi.Listing, error) {
	p.query = query
	return []marketapi.Listing{{MarketplaceEntry: marketapi.MarketplaceEntry{Name: "recorded-pack"}}}, nil
}

func (p *recordingPort) InstallContext(ctx context.Context, corr, marketplace, name, version string, emit func(marketapi.Event) error) (marketapi.InstalledPack, error) {
	p.corr = corr
	p.marketplace = marketplace
	p.pack = name
	return marketapi.InstalledPack{Name: name, Version: "1.0.0", SkillIDs: []string{"skill-1"}, MCPServers: []string{"fetch"}}, nil
}

// TestTheToolDrivesThePortItWasGiven: the tool's search passes the agent's
// query through, and its install passes the pack, the marketplace and the
// run's correlation — the port is where the request goes, so a tool that
// dropped a field would silently install the wrong pack.
func TestTheToolDrivesThePortItWasGiven(t *testing.T) {
	port := &recordingPort{}
	tool := &market.Tool{Manager: func() marketapi.Manager { return port }}

	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"search","query":"web"}`))
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.IsError {
		t.Fatalf("search errored: %s", res.Output)
	}
	if port.query != "web" {
		t.Fatalf("the tool asked the port for %q, want the agent's query %q", port.query, "web")
	}
	if !strings.Contains(res.Output, "recorded-pack") {
		t.Fatalf("the search output does not carry the port's row:\n%s", res.Output)
	}

	ctx := toolapi.WithCorrelation(context.Background(), "corr-42")
	res, err = tool.Invoke(ctx, json.RawMessage(`{"op":"install","pack":"git-workshop","marketplace":"official"}`))
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if res.IsError {
		t.Fatalf("install errored: %s", res.Output)
	}
	if port.pack != "git-workshop" {
		t.Fatalf("the tool installed %q, want the agent's pack %q", port.pack, "git-workshop")
	}
	if port.marketplace != "official" {
		t.Fatalf("the tool installed from %q, want the agent's marketplace %q", port.marketplace, "official")
	}
	if port.corr != "corr-42" {
		t.Fatalf("the tool installed under correlation %q, want the run's %q", port.corr, "corr-42")
	}
	if !strings.Contains(res.Output, "installed git-workshop") {
		t.Fatalf("the install output does not name the pack:\n%s", res.Output)
	}
}
