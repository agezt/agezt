// SPDX-License-Identifier: MIT

package runtime

// Agent-facing capability marketplace tool: lets a running agent discover and
// install capability packs (skills + MCP servers + CLI-tool requirements) mid-
// task, when it realizes it lacks a capability the task needs. It drives the
// same market.Manager the `agt market` CLI and Web UI use, so an install
// materializes skills into the Forge and MCP servers into the registry — which
// the very next agent turn can then use. Discovery is read-only; install is
// effectful and journaled by the loop (tool.invoked/tool.result) plus the
// manager's own provenance record.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/market"
)

// marketTool implements agent.Tool. It holds a lazy getter for the marketplace
// manager because the daemon wires the manager (SetMarket) AFTER Open — the
// getter is bound to k.Market once the Kernel exists, and resolves at Invoke.
type marketTool struct {
	manager func() *market.Manager
}

func newMarketTool() *marketTool { return &marketTool{} }

// maxMarketSearchRows bounds how many catalogue rows one search returns to the
// model, so a large catalogue can't flood the context.
const maxMarketSearchRows = 25

func (t *marketTool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "market",
		Description: "Discover and install capability packs from the marketplace when you lack a capability the task needs. " +
			"A pack bundles skills, MCP servers, and CLI tools; installing it makes those available to you from your next step on. " +
			"op=search lists packs (optional query); op=show inspects one pack's contents; op=install materializes a pack. " +
			"Prefer searching first, then install only the pack you actually need.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "op": {"type": "string", "enum": ["search", "show", "install"], "description": "search the catalogue, show one pack, or install one pack"},
    "query": {"type": "string", "description": "op=search: free-text filter over name/description/category/tags (empty lists all)"},
    "pack": {"type": "string", "description": "op=show/install: the pack name"},
    "marketplace": {"type": "string", "description": "optional: restrict to a named marketplace (e.g. a synced remote); defaults to all"}
  },
  "required": ["op"]
}`),
	}
}

type marketToolInput struct {
	Op          string `json:"op"`
	Query       string `json:"query"`
	Pack        string `json:"pack"`
	Marketplace string `json:"marketplace"`
}

func (t *marketTool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	mgr := t.resolve()
	if mgr == nil {
		return agent.Result{Output: "marketplace is not available on this daemon", IsError: true}, nil
	}
	var in marketToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	switch strings.TrimSpace(in.Op) {
	case "search", "":
		return t.search(mgr, in.Query)
	case "show":
		return t.show(mgr, in.Marketplace, in.Pack)
	case "install":
		return t.install(ctx, mgr, in.Marketplace, in.Pack)
	default:
		return agent.Result{Output: fmt.Sprintf("unknown op %q (use search|show|install)", in.Op), IsError: true}, nil
	}
}

func (t *marketTool) resolve() *market.Manager {
	if t.manager == nil {
		return nil
	}
	return t.manager()
}

func (t *marketTool) search(mgr *market.Manager, query string) (agent.Result, error) {
	listings, err := mgr.List(query)
	if err != nil {
		return agent.Result{Output: "search failed: " + err.Error(), IsError: true}, nil
	}
	if len(listings) == 0 {
		return agent.Result{Output: "no packs match. Try op=search with an empty query to see the full catalogue."}, nil
	}
	var b strings.Builder
	shown := listings
	if len(shown) > maxMarketSearchRows {
		shown = shown[:maxMarketSearchRows]
	}
	fmt.Fprintf(&b, "%d pack(s)", len(listings))
	if len(shown) < len(listings) {
		fmt.Fprintf(&b, " (showing first %d — narrow with a query)", len(shown))
	}
	b.WriteString(":\n")
	for _, l := range shown {
		state := ""
		if l.Installed {
			state = " [installed]"
		}
		fmt.Fprintf(&b, "- %s (%s) — %d skill/%d mcp/%d tool%s: %s\n",
			l.Name, nonEmpty(l.Category, "uncategorized"), l.SkillCount, l.MCPCount, l.ToolCount, state, l.Description)
	}
	b.WriteString("Install one with op=install, pack=<name>.")
	return agent.Result{Output: b.String()}, nil
}

func (t *marketTool) show(mgr *market.Manager, marketplace, pack string) (agent.Result, error) {
	if strings.TrimSpace(pack) == "" {
		return agent.Result{Output: "op=show needs a pack name", IsError: true}, nil
	}
	p, _, installed, err := mgr.Show(marketplace, pack)
	if err != nil {
		return agent.Result{Output: "show failed: " + err.Error(), IsError: true}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s @ %s [%s]%s\n", p.Name, p.Version, nonEmpty(p.Category, "uncategorized"), iff(installed, " (installed)", ""))
	if p.Description != "" {
		fmt.Fprintf(&b, "%s\n", p.Description)
	}
	for _, s := range p.Skills {
		if md, perr := market.SkillSummary(s); perr == nil {
			fmt.Fprintf(&b, "- skill: %s\n", md)
		}
	}
	for _, m := range p.MCPServers {
		fmt.Fprintf(&b, "- mcp server: %s\n", m.Name)
	}
	if len(p.ToolRequirements) > 0 {
		fmt.Fprintf(&b, "- CLI tools needed: %s (install via the Toolbox)\n", strings.Join(p.ToolRequirements, ", "))
	}
	return agent.Result{Output: b.String()}, nil
}

func (t *marketTool) install(ctx context.Context, mgr *market.Manager, marketplace, pack string) (agent.Result, error) {
	if strings.TrimSpace(pack) == "" {
		return agent.Result{Output: "op=install needs a pack name", IsError: true}, nil
	}
	corr := agent.CorrelationFromContext(ctx)
	rec, err := mgr.Install(corr, marketplace, pack, "", nil)
	if err != nil {
		return agent.Result{Output: "install failed: " + err.Error(), IsError: true}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "installed %s: %d skill(s) now active, %d MCP server(s) registered",
		rec.Name, len(rec.SkillIDs), len(rec.MCPServers))
	if len(rec.ToolReqs) > 0 {
		fmt.Fprintf(&b, ". This pack also needs CLI tools (%s) — ask the operator to install them in the Toolbox", strings.Join(rec.ToolReqs, ", "))
	}
	b.WriteString(". The new skills are available from your next step.")
	return agent.Result{Output: b.String()}, nil
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func iff(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
