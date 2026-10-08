// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// Pausing or resuming must be visible to the very next agent_list, inside the
// list cache TTL: the write invalidates the cached rows.
func TestAgentSetEnabled_InvalidatesListCache(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{"profile": map[string]any{"slug": "cached", "soul": "You work."}}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	enabledCount := func() any {
		res, err := c.Call(ctx, controlplane.CmdAgentList, nil)
		if err != nil {
			t.Fatalf("agent list: %v", err)
		}
		return res["enabled_count"]
	}
	before := enabledCount()
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "cached", "enabled": false}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if after := enabledCount(); after == before {
		t.Fatalf("enabled_count stayed %v after pause: list cache not invalidated", after)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "cached", "enabled": true}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if again := enabledCount(); again != before {
		t.Fatalf("enabled_count %v after resume, want %v", again, before)
	}
}
