// SPDX-License-Identifier: MIT

package runtime

// The daemon's SCRIPT-TOOL MERGE: the name it keeps and the body it gave
// away.
//
// The bridge (which folds the forge's ACTIVE scripts into a run's tool map)
// moved into the extensions module with the catalogue it answers for
// (W3.72). The daemon's merge is a forwarder now — and it must stay a
// forwarder: a daemon that merged on its own would offer scripts the module
// does not know about.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	extapi "github.com/agezt/agezt/kernel/modules/extensions/api"
)

// forgeStubTool is a registered tool: the run's own map.
type forgeStubTool struct{ name string }

func (s forgeStubTool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{Name: s.name, Description: "registered"}
}

func (s forgeStubTool) Invoke(context.Context, json.RawMessage) (toolapi.Result, error) {
	return toolapi.Result{Output: "registered"}, nil
}

// countingForge is a fake extensions module that counts the merges it was
// asked for.
//
// The embedded interface is nil on purpose: a verb the daemon must not reach
// for panics instead of silently answering.
type countingForge struct {
	extapi.ForgeService
	merged int
}

func (r *countingForge) MergeTools(tools map[string]toolapi.Tool) map[string]toolapi.Tool {
	r.merged++
	out := make(map[string]toolapi.Tool, len(tools)+1)
	for name, t := range tools {
		out[name] = t
	}
	out["forge_owned"] = forgeStubTool{"forge_owned"}
	return out
}

// TestTheDaemonForwardsTheForgeMergeToTheModule: the merge is the module's.
func TestTheDaemonForwardsTheForgeMergeToTheModule(t *testing.T) {
	k := &Kernel{}
	tools := map[string]toolapi.Tool{"own": forgeStubTool{"own"}}

	// Without the module there is nothing to merge: the map comes back
	// untouched (a host with no forge has no promoted scripts).
	got := k.mergeScriptTools(tools)
	if len(got) != 1 {
		t.Fatalf("a daemon with no module must not merge: %v", got)
	}

	fake := &countingForge{}
	k.SetForge(fake)
	got = k.mergeScriptTools(tools)
	if len(got) != 2 {
		t.Fatalf("the merge did not reach the module: %v", got)
	}
	if fake.merged != 1 {
		t.Fatalf("the daemon merged on its own: %d merges", fake.merged)
	}
	if _, ok := got["forge_owned"]; !ok {
		t.Fatalf("the module's tool is not in the merged map: %v", got)
	}
}
