// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestStanding_CRUDRoundTrip drives the full management surface over the control
// plane: add → list → pause → remove, asserting each step and that every
// mutation is journaled (standing.created/updated/removed) (M403, SPEC-16 §4).
func TestStanding_CRUDRoundTrip(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add a cron-triggered order.
	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":       "portfolio watch",
			"triggers":   []any{map[string]any{"type": "cron", "schedule": "0 8 * * *"}},
			"plan":       "brief me each morning",
			"initiative": map[string]any{"mode": "act_or_ask", "max_trust": "L2"},
		},
	})
	if err != nil {
		t.Fatalf("standing_add: %v", err)
	}
	o, _ := add["order"].(map[string]any)
	id, _ := o["id"].(string)
	if id == "" {
		t.Fatalf("add returned no id: %v", add)
	}
	if en, _ := o["enabled"].(bool); !en {
		t.Error("new order should be enabled")
	}

	// An invalid order is rejected.
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{"name": ""},
	}); err == nil {
		t.Error("adding an invalid order should error")
	}

	// List shows it.
	list, err := c.Call(ctx, controlplane.CmdStandingList, nil)
	if err != nil {
		t.Fatalf("standing_list: %v", err)
	}
	if cnt, _ := list["count"].(float64); int(cnt) != 1 {
		t.Errorf("list count = %v, want 1", list["count"])
	}

	// Pause it.
	pause, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": false})
	if err != nil {
		t.Fatalf("standing pause: %v", err)
	}
	po, _ := pause["order"].(map[string]any)
	if en, _ := po["enabled"].(bool); en {
		t.Error("order should be paused")
	}

	// Remove it.
	rm, err := c.Call(ctx, controlplane.CmdStandingRemove, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("standing remove: %v", err)
	}
	if removed, _ := rm["removed"].(bool); !removed {
		t.Error("remove should report removed=true")
	}

	// Every mutation is journaled.
	for _, want := range []event.Kind{event.KindStandingCreated, event.KindStandingUpdated, event.KindStandingRemoved} {
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

// TestStanding_Edit edits an order's mutable fields over the control plane (M729):
// add → edit name/plan/mode/assure → list shows the edit, the journal records a
// standing.updated, and editing an unknown id reports updated:false.
func TestStanding_Edit(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":       "watch",
			"triggers":   []any{map[string]any{"type": "cron", "schedule": "0 8 * * *"}},
			"plan":       "old plan",
			"initiative": map[string]any{"mode": "act_or_ask", "max_trust": "L2"},
		},
	})
	if err != nil {
		t.Fatalf("standing_add: %v", err)
	}
	id, _ := add["order"].(map[string]any)["id"].(string)

	edit, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{
		"id": id, "name": "renamed", "plan": "new plan", "mode": "ask", "assure": float64(2),
	})
	if err != nil {
		t.Fatalf("standing_edit: %v", err)
	}
	if up, _ := edit["updated"].(bool); !up {
		t.Fatalf("edit should report updated=true: %v", edit)
	}
	eo, _ := edit["order"].(map[string]any)
	if eo["name"] != "renamed" || eo["plan"] != "new plan" {
		t.Errorf("edited fields not reflected: %v", eo)
	}
	if init, _ := eo["initiative"].(map[string]any); init["mode"] != "ask" {
		t.Errorf("mode not edited: %v", eo["initiative"])
	}
	if as, _ := eo["assure"].(float64); int(as) != 2 {
		t.Errorf("assure = %v, want 2", eo["assure"])
	}
	// Identity preserved.
	if eo["id"] != id {
		t.Errorf("id changed on edit: %v != %v", eo["id"], id)
	}

	// List reflects the persisted edit.
	list, _ := c.Call(ctx, controlplane.CmdStandingList, nil)
	orders, _ := list["orders"].([]any)
	if len(orders) != 1 || orders[0].(map[string]any)["name"] != "renamed" {
		t.Errorf("list does not show the edit: %v", list)
	}

	// A standing.updated event is journaled.
	n := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindStandingUpdated {
			n++
		}
		return nil
	})
	if n == 0 {
		t.Error("edit was not journaled as standing.updated")
	}

	// Editing an unknown id reports updated:false (not an error).
	miss, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{"id": "nope", "name": "x"})
	if err != nil {
		t.Fatalf("edit unknown id errored: %v", err)
	}
	if up, _ := miss["updated"].(bool); up {
		t.Error("editing an unknown id should report updated=false")
	}
}

// TestStanding_Why folds an order's life story: create + pause → at least the
// created and updated events, scoped to that order id.
func TestStanding_Why(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":     "watch",
			"triggers": []any{map[string]any{"type": "event", "subject": "github.>"}},
		},
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	o, _ := add["order"].(map[string]any)
	id, _ := o["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": false}); err != nil {
		t.Fatalf("pause: %v", err)
	}

	why, err := c.Call(ctx, controlplane.CmdStandingWhy, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("why: %v", err)
	}
	evs, _ := why["events"].([]any)
	if len(evs) < 2 {
		t.Fatalf("why returned %d events, want >= 2 (created + updated)", len(evs))
	}
	// Every returned event must be scoped to this order id.
	for _, raw := range evs {
		e, _ := raw.(map[string]any)
		p, _ := e["payload"].(map[string]any)
		if p["id"] != id {
			t.Errorf("why returned an event for a different order: %v", p["id"])
		}
	}
	// An unknown id yields no events.
	none, _ := c.Call(ctx, controlplane.CmdStandingWhy, map[string]any{"id": "nope"})
	if cnt, _ := none["count"].(float64); cnt != 0 {
		t.Errorf("why for unknown id = %v events, want 0", cnt)
	}
}

// TestStandingFire (M765) drives the on-demand fire path: with a fire callback
// wired, CmdStandingFire looks up the order and invokes it; an unknown id is a
// no-op; and without a callback wired the daemon reports it unavailable.
func TestStandingFire(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Seed an order to fire.
	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":     "nightly digest",
			"triggers": []any{map[string]any{"type": "cron", "schedule": "0 8 * * *"}},
			"plan":     "summarize the day",
		},
	})
	if err != nil {
		t.Fatalf("standing_add: %v", err)
	}
	o, _ := add["order"].(map[string]any)
	id, _ := o["id"].(string)

	// Before wiring, firing reports unavailable.
	if _, err := c.Call(ctx, controlplane.CmdStandingFire, map[string]any{"id": id}); err == nil {
		t.Error("fire with no callback wired should error")
	}

	// Wire a recording fire callback (mirrors the daemon's injection).
	var firedID string
	srv.SetStandingFire(func(fid string) bool { firedID = fid; return true })

	res, err := c.Call(ctx, controlplane.CmdStandingFire, map[string]any{"id": id})
	if err != nil {
		t.Fatalf("standing_fire: %v", err)
	}
	if fired, _ := res["fired"].(bool); !fired {
		t.Errorf("fired = %v, want true", res["fired"])
	}
	if firedID != id {
		t.Errorf("callback got id %q, want %q", firedID, id)
	}

	// An unknown id is a no-op (fired:false), not an error.
	res2, err := c.Call(ctx, controlplane.CmdStandingFire, map[string]any{"id": "nope"})
	if err != nil {
		t.Fatalf("standing_fire(unknown): %v", err)
	}
	if fired, _ := res2["fired"].(bool); fired {
		t.Errorf("unknown id should not fire, got fired=%v", res2["fired"])
	}
}
