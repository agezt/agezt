// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestAgent_CRUDRoundTrip drives the agent-roster management surface over the
// control plane: add → list → edit → pause → remove (by slug), asserting each
// step and that every mutation is journaled (roster.created/updated/removed).
func TestAgent_CRUDRoundTrip(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":  "researcher",
			"soul":  "You research deeply and cite sources.",
			"model": "mock-model",
		},
	})
	if err != nil {
		t.Fatalf("agent add: %v", err)
	}
	prof, _ := add["profile"].(map[string]any)
	if prof == nil || prof["slug"] != "researcher" || prof["enabled"] != true {
		t.Fatalf("add returned %v", add)
	}
	if prof["name"] != "researcher" {
		t.Errorf("name should default to slug, got %v", prof["name"])
	}

	// A duplicate slug is refused.
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "researcher"},
	}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate slug: err = %v, want already-exists", err)
	}

	list, err := c.Call(ctx, controlplane.CmdAgentList, nil)
	if err != nil {
		t.Fatalf("agent list: %v", err)
	}
	if n, _ := list["count"].(float64); n != 1 {
		t.Fatalf("count = %v, want 1", list["count"])
	}

	// Edit by slug: the soul changes, the slug cannot.
	edit, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{
		"ref": "researcher",
		"profile": map[string]any{
			"slug": "hijacked", "name": "The Researcher", "soul": "v2 soul", "model": "mock-model",
		},
	})
	if err != nil {
		t.Fatalf("agent edit: %v", err)
	}
	ep, _ := edit["profile"].(map[string]any)
	if ep["slug"] != "researcher" || ep["soul"] != "v2 soul" || ep["name"] != "The Researcher" {
		t.Fatalf("edit result wrong: %v", ep)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentEdit, map[string]any{
		"ref": "ghost", "profile": map[string]any{},
	}); err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("edit unknown: err = %v", err)
	}

	// Pause by slug; the webui string transport ("false") must also work.
	pause, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "researcher", "enabled": "false"})
	if err != nil {
		t.Fatalf("agent pause: %v", err)
	}
	if pp, _ := pause["profile"].(map[string]any); pp["enabled"] != false {
		t.Fatalf("pause result wrong: %v", pause)
	}

	rm, err := c.Call(ctx, controlplane.CmdAgentRemove, map[string]any{"ref": "researcher"})
	if err != nil {
		t.Fatalf("agent remove: %v", err)
	}
	if removed, _ := rm["removed"].(bool); !removed {
		t.Error("remove should report removed=true")
	}

	// Every mutation is journaled.
	for _, want := range []event.Kind{event.KindRosterCreated, event.KindRosterUpdated, event.KindRosterRemoved} {
		n := 0
		_ = k.Journal().Range(func(e *event.Event) error {
			if e.Kind == want {
				n++
			}
			return nil
		})
		if n == 0 {
			t.Errorf("no %s event journaled", want)
		}
	}
}

// TestRun_AsAgent proves the run seam: --agent resolves the profile and applies
// its model + soul as the run's defaults (visible in the dry-run plan, which is
// built from the SAME locals the real run uses); explicit overrides still win;
// unknown and paused agents are usage errors.
func TestRun_AsAgent(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{
			"slug":  "researcher",
			"soul":  "You research deeply.",
			"model": "agent-model",
		},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}

	// The profile fills the gaps: model + system come from the agent.
	plan, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher", "dry_run": true})
	if err != nil {
		t.Fatalf("dry-run as agent: %v", err)
	}
	if plan["model"] != "agent-model" {
		t.Errorf("model = %v, want agent-model", plan["model"])
	}
	if src, _ := plan["system_source"].(string); !strings.Contains(src, "per-run") {
		t.Errorf("system_source = %v, want per-run (the soul was applied)", src)
	}

	// Explicit per-run flags still win over the profile.
	plan2, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher", "model": "explicit-model", "dry_run": true})
	if err != nil {
		t.Fatalf("dry-run with explicit model: %v", err)
	}
	if plan2["model"] != "explicit-model" {
		t.Errorf("model = %v, want explicit-model (explicit flag must win)", plan2["model"])
	}

	// Unknown agent → usage error, nothing runs.
	if _, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("unknown agent: err = %v", err)
	}

	// Paused agent → refused with a hint, nothing runs.
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled,
		map[string]any{"ref": "researcher", "enabled": false}); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdRun,
		map[string]any{"intent": "x", "agent": "researcher"}); err == nil ||
		!strings.Contains(err.Error(), "paused") {
		t.Fatalf("paused agent: err = %v", err)
	}
}
