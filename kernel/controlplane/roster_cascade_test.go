// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// agentImpactKeys pins the WIRE payload of agent_impact — the teardown preview
// the console shows before retiring or removing an agent (Phase 3.5).
//
// The payload is now derived from the cascadeSubsystems table instead of a
// forty-entry map literal, which is a strict improvement but also means a table
// edit silently reshapes the wire. The console reads these names verbatim
// (frontend/src/components/agentdetail/lifecycle.tsx) and defaults a missing key
// to an empty list, so a renamed key does not error — it just makes a whole
// subsystem's impact vanish from the confirmation dialog the operator is relying
// on to decide whether the removal is safe.
//
// If this fails you either added a subsystem (add its four keys), removed one
// (delete them), or renamed one — in which case update the frontend interface in
// the same change.
var agentImpactKeys = []string{
	"slug",

	"standing_orders", "standing_count",
	"schedules", "schedule_count",
	"memories", "memory_count",
	"authored_shared_memories", "authored_shared_memory_count",
	"skills", "skill_count",
	"configs", "config_count",
	"workspaces", "workspace_count",
	"workflow_refs", "workflow_ref_count",
	"mailbox_messages", "mailbox_message_count",

	"subagents", "subagent_count",
	"subagent_standing_orders", "subagent_standing_count",
	"subagent_schedules", "subagent_schedule_count",
	"subagent_memories", "subagent_memory_count",
	"subagent_authored_shared_memories", "subagent_authored_shared_memory_count",
	"subagent_skills", "subagent_skill_count",
	"subagent_configs", "subagent_config_count",
	"subagent_workspaces", "subagent_workspace_count",
	"subagent_workflow_refs", "subagent_workflow_ref_count",
	"subagent_mailbox_messages", "subagent_mailbox_message_count",
}

func TestAgentImpact_PayloadKeysArePinned(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "doomed", "soul": "about to be torn down"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	res, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": "doomed"})
	if err != nil {
		t.Fatalf("agent impact: %v", err)
	}
	got := res

	want := map[string]bool{}
	for _, k := range agentImpactKeys {
		if want[k] {
			t.Fatalf("agentImpactKeys lists %q twice — fix the pinned list", k)
		}
		want[k] = true
	}
	var extra, missing []string
	for k := range got {
		if !want[k] {
			extra = append(extra, k)
		}
	}
	for _, k := range agentImpactKeys {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(extra)
	sort.Strings(missing)
	if len(extra) > 0 {
		t.Errorf("payload has unpinned key(s) %s — a subsystem shipped without a conscious ratchet update", strings.Join(extra, ", "))
	}
	if len(missing) > 0 {
		t.Errorf("payload is missing pinned key(s) %s — the console reads these names verbatim and shows an empty list when one disappears", strings.Join(missing, ", "))
	}
}

// Every subsystem must report BOTH halves of both pairs. A count without its
// list (or the reverse) reads as "nothing to clean up" in the dialog, which is
// the most dangerous way for this payload to be wrong.
func TestAgentImpact_EverySubsystemReportsListAndCount(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "doomed"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	res, err := c.Call(ctx, controlplane.CmdAgentImpact, map[string]any{"ref": "doomed"})
	if err != nil {
		t.Fatalf("agent impact: %v", err)
	}
	got := res

	for _, k := range agentImpactKeys {
		if k == "slug" {
			continue
		}
		v, present := got[k]
		if !present {
			continue // already reported by the pinning test
		}
		if strings.HasSuffix(k, "_count") {
			if _, ok := v.(float64); !ok {
				t.Errorf("%s = %#v, want a number", k, v)
			}
			continue
		}
		// A list may be null (no impact) or an array; anything else means the
		// table wired a non-list value under a list key.
		if v != nil {
			if _, ok := v.([]any); !ok {
				t.Errorf("%s = %#v, want a list or null", k, v)
			}
		}
	}
}
