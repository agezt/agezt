// SPDX-License-Identifier: MIT

package main

import (
	"maps"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/market"
)

// withMarketTool hands the composition root's `market` tool to the kernel as an
// ordinary tool.
//
// The kernel used to build this tool itself: it reached into kernel/market to
// do it, and that reach was the kernel's last word on the marketplace (W3.46).
// The kernel is a module and may not name a module's implementation — so the
// tool is the DAEMON's now: the daemon owns the manager (it wires it with
// SetMarket after Open, because the built-in catalogue installs into the
// kernel's own Forge and MCP), and the daemon hands the tool over like any
// other tool.
//
// The tool map is COPIED, never mutated: the primary kernel and every tenant
// share one map, and each kernel must see ITS OWN tool bound to ITS OWN
// marketplace port. A map written in place would hand the last kernel's tool
// to all of them — and a tenant would install packs into the primary's Forge.
func withMarketTool(tools map[string]toolapi.Tool, tool *market.Tool) map[string]toolapi.Tool {
	out := make(map[string]toolapi.Tool, len(tools)+1)
	maps.Copy(out, tools)
	out["market"] = tool
	return out
}
