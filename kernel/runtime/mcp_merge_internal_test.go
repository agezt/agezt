// SPDX-License-Identifier: MIT

package runtime

// The daemon's TOOL MERGE: the name it keeps and the body it gave away.
//
// The bridge (which folds an attached MCP server's tools into a run's tool
// map) moved into the extensions module with the live attachments it answers
// for (W3.71). The daemon's merge is a forwarder now — and it must stay a
// forwarder: a daemon that merged on its own would offer tools the module
// does not know about.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// mergeStubTool is a registered tool: the run's own map.
type mergeStubTool struct{ name string }

func (s mergeStubTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: s.name, Description: "registered"}
}

func (s mergeStubTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "registered"}, nil
}

// countingRegistry is a fake extensions module that counts the merges it was
// asked for.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type countingRegistry struct {
	extapi.MCPRegistryService
	merged int
}

func (r *countingRegistry) MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	r.merged++
	out := make(map[string]toolapi.Tool, len(tools)+1)
	for name, t := range tools {
		out[name] = t
	}
	out["mcp_owned_greet"] = mergeStubTool{"mcp_owned_greet"}
	return out
}

// TestTheDaemonForwardsTheToolMergeToTheModule: the merge is the module's.
func TestTheDaemonForwardsTheToolMergeToTheModule(t *testing.T) {
	k := &Kernel{}
	tools := map[string]toolapi.Tool{"own": mergeStubTool{"own"}}

	// Without the module there is nothing to merge: the map comes back
	// untouched (a host with no registry has no attachments).
	got := k.mergeMCPTools(tools)
	if len(got) != 1 {
		t.Fatalf("a daemon with no module must not merge: %v", got)
	}

	fake := &countingRegistry{}
	k.SetMCPRegistry(fake)
	got = k.mergeMCPTools(tools)
	if len(got) != 2 {
		t.Fatalf("the merge did not reach the module: %v", got)
	}
	if fake.merged != 1 {
		t.Fatalf("the daemon merged on its own: %d merges", fake.merged)
	}
	if _, ok := got["mcp_owned_greet"]; !ok {
		t.Fatalf("the module's tool is not in the merged map: %v", got)
	}
}
