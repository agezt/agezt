// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestConfigCenterSet_AgentAllowDenyPoliciesRoundTrip(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	res, err := c.Call(ctx, controlplane.CmdConfigCenterSet, map[string]any{
		"key":             "agent/ops/runtime",
		"value":           "mode=careful",
		"rating":          "internal",
		"description":     "ops-only runtime config",
		"allowed_agents":  []any{"ops", "ops", "planner"},
		"excluded_agents": "blocked",
	})
	if err != nil {
		t.Fatalf("configcenter set: %v", err)
	}
	entry, _ := res["entry"].(map[string]any)
	if entry["key"] != "agent/ops/runtime" {
		t.Fatalf("entry = %+v", entry)
	}
	if got := anyStrings(entry["allowed_agents"]); strings.Join(got, ",") != "ops,planner" {
		t.Fatalf("allowed_agents = %v, want ops,planner", got)
	}
	if got := anyStrings(entry["excluded_agents"]); strings.Join(got, ",") != "blocked" {
		t.Fatalf("excluded_agents = %v, want blocked", got)
	}

	got, err := k.ConfigCenter().Get(ctx, configcenter.ConfigAccessRequest{
		AgentID: "ops",
		Key:     "agent/ops/runtime",
		Reason:  "agent runtime boot",
	})
	if err != nil || got != "mode=careful" {
		t.Fatalf("ops access got value=%q err=%v", got, err)
	}
	if _, err := k.ConfigCenter().Get(ctx, configcenter.ConfigAccessRequest{
		AgentID: "other",
		Key:     "agent/ops/runtime",
		Reason:  "cross-agent read",
	}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("other access err = %v, want not allowed", err)
	}
	if _, err := k.ConfigCenter().Get(ctx, configcenter.ConfigAccessRequest{
		AgentID: "blocked",
		Key:     "agent/ops/runtime",
		Reason:  "blocked read",
	}); err == nil {
		t.Fatalf("blocked agent access unexpectedly succeeded")
	}

	list, err := c.Call(ctx, controlplane.CmdConfigCenterList, nil)
	if err != nil {
		t.Fatalf("configcenter list: %v", err)
	}
	rows, _ := list["entries"].([]any)
	if len(rows) != 1 {
		t.Fatalf("entries len = %d, want 1", len(rows))
	}
	row, _ := rows[0].(map[string]any)
	if got := anyStrings(row["allowed_agents"]); strings.Join(got, ",") != "ops,planner" {
		t.Fatalf("listed allowed_agents = %v, want ops,planner", got)
	}
	if got := anyStrings(row["excluded_agents"]); strings.Join(got, ",") != "blocked" {
		t.Fatalf("listed excluded_agents = %v, want blocked", got)
	}

	access, err := c.Call(ctx, controlplane.CmdConfigCenterSetAccess, map[string]any{
		"key":             "agent/ops/runtime",
		"allowed_agents":  []any{"ops"},
		"excluded_agents": []any{"blocked", "other"},
	})
	if err != nil {
		t.Fatalf("configcenter access: %v", err)
	}
	updated, _ := access["entry"].(map[string]any)
	if updated["value"] != "mode=careful" || updated["rating"] != "internal" || updated["description"] != "ops-only runtime config" {
		t.Fatalf("access update should preserve value/rating/description: %+v", updated)
	}
	if got := anyStrings(updated["allowed_agents"]); strings.Join(got, ",") != "ops" {
		t.Fatalf("updated allowed_agents = %v, want ops", got)
	}
	if got := anyStrings(updated["excluded_agents"]); strings.Join(got, ",") != "blocked,other" {
		t.Fatalf("updated excluded_agents = %v, want blocked,other", got)
	}
}

func anyStrings(raw any) []string {
	switch xs := raw.(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), xs...)
	default:
		return nil
	}
}
