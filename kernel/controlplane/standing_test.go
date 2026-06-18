// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/standing"
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
		"id": id, "name": "renamed", "plan": "new plan", "mode": "ask", "assure": float64(2), "cooldown_sec": float64(1800),
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
	if cd, _ := eo["cooldown_sec"].(float64); int(cd) != 1800 {
		t.Errorf("cooldown_sec = %v, want 1800", eo["cooldown_sec"])
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

func TestStandingList_IncludesFrequencyWarnings(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	for _, order := range []map[string]any{
		{
			"name":     "chatty cron",
			"triggers": []any{map[string]any{"type": "cron", "schedule": "* * * * *"}},
		},
		{
			"name":         "chatty event",
			"triggers":     []any{map[string]any{"type": "event", "subject": "run.failed"}},
			"cooldown_sec": float64(60),
		},
		{
			"name":         "quiet event",
			"triggers":     []any{map[string]any{"type": "event", "subject": "board.dm.ops"}},
			"cooldown_sec": float64(3600),
		},
	} {
		if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": order}); err != nil {
			t.Fatalf("standing add %s: %v", order["name"], err)
		}
	}

	list, err := c.Call(ctx, controlplane.CmdStandingList, nil)
	if err != nil {
		t.Fatalf("standing list: %v", err)
	}
	orders, _ := list["orders"].([]any)
	warnings := map[string]string{}
	for _, raw := range orders {
		row, _ := raw.(map[string]any)
		name, _ := row["name"].(string)
		warnings[name], _ = row["frequency_warning"].(string)
	}
	if !strings.Contains(warnings["chatty cron"], "every minute") {
		t.Fatalf("chatty cron warning = %q, want every minute", warnings["chatty cron"])
	}
	if !strings.Contains(warnings["chatty event"], "default 15m guard") {
		t.Fatalf("chatty event warning = %q, want default guard", warnings["chatty event"])
	}
	if warnings["quiet event"] != "" {
		t.Fatalf("quiet event warning = %q, want none", warnings["quiet event"])
	}
}

func TestStandingAgentMustBeDirectCallable(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "Lead.", "model": "m"},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "parent_agent": "lead", "direct_callable": false},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "old", "soul": "Old.", "model": "m"},
	}); err != nil {
		t.Fatalf("old add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "old"}); err != nil {
		t.Fatalf("old retire: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "paused", "soul": "Paused.", "model": "m"},
	}); err != nil {
		t.Fatalf("paused add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "paused", "enabled": false}); err != nil {
		t.Fatalf("paused disable: %v", err)
	}

	validOrder := func(agent string) map[string]any {
		return map[string]any{
			"name":     "watch " + agent,
			"agent":    agent,
			"triggers": []any{map[string]any{"type": "cron", "schedule": "0 8 * * *"}},
		}
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": validOrder("ghost")}); err == nil || !strings.Contains(err.Error(), "unknown standing agent") {
		t.Fatalf("unknown standing agent err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": validOrder("old")}); err == nil || !strings.Contains(err.Error(), "is retired") {
		t.Fatalf("retired standing agent err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": validOrder("paused")}); err == nil || !strings.Contains(err.Error(), "is paused") {
		t.Fatalf("paused standing agent err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": validOrder("worker")}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("managed standing agent err = %v", err)
	}

	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{"order": validOrder("lead")})
	if err != nil {
		t.Fatalf("standing add with direct agent: %v", err)
	}
	id, _ := add["order"].(map[string]any)["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{"id": id, "agent": "worker"}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("edit to managed standing agent err = %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{"id": id, "agent": "paused"}); err == nil || !strings.Contains(err.Error(), "is paused") {
		t.Fatalf("edit to paused standing agent err = %v", err)
	}
	edit, err := c.Call(ctx, controlplane.CmdStandingEdit, map[string]any{"id": id, "agent": ""})
	if err != nil {
		t.Fatalf("edit clear standing agent: %v", err)
	}
	if got, _ := edit["order"].(map[string]any)["agent"].(string); got != "" {
		t.Fatalf("agent after clear = %q want empty", got)
	}
}

func TestStandingEnable_ValidatesBoundAgentState(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "ops", "soul": "Operate.", "model": "m"},
	}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	add, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":     "ops mailbox",
			"agent":    "ops",
			"triggers": []any{map[string]any{"type": "event", "subject": "board.dm.ops"}},
		},
	})
	if err != nil {
		t.Fatalf("standing add: %v", err)
	}
	id, _ := add["order"].(map[string]any)["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": false}); err != nil {
		t.Fatalf("standing pause: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "ops", "enabled": false}); err != nil {
		t.Fatalf("agent pause: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": true}); err == nil || !strings.Contains(err.Error(), "is paused") {
		t.Fatalf("enable standing bound to paused agent err = %v, want paused", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentSetEnabled, map[string]any{"ref": "ops", "enabled": true}); err != nil {
		t.Fatalf("agent resume: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentRetire, map[string]any{"ref": "ops"}); err != nil {
		t.Fatalf("agent retire: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": id, "enabled": true}); err == nil || !strings.Contains(err.Error(), "is retired") {
		t.Fatalf("enable standing bound to retired agent err = %v, want retired", err)
	}

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "soul": "Lead.", "model": "m"},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "parent_agent": "lead", "direct_callable": false},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}
	managed, err := c.Call(ctx, controlplane.CmdStandingAdd, map[string]any{
		"order": map[string]any{
			"name":     "lead mailbox",
			"agent":    "lead",
			"triggers": []any{map[string]any{"type": "event", "subject": "board.help.lead"}},
		},
	})
	if err != nil {
		t.Fatalf("standing add lead: %v", err)
	}
	managedID, _ := managed["order"].(map[string]any)["id"].(string)
	if _, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": managedID, "enabled": false}); err != nil {
		t.Fatalf("standing pause managed test order: %v", err)
	}
	if _, err := k.Standing().Update(managedID, func(o *standing.Order) {
		o.Agent = "worker"
	}); err != nil {
		t.Fatalf("force corrupt standing agent: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdStandingSetEnabled, map[string]any{"id": managedID, "enabled": true}); err == nil || !strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("enable standing bound to managed sub-agent err = %v, want managed sub-agent", err)
	}
	list, err := c.Call(ctx, controlplane.CmdStandingList, nil)
	if err != nil {
		t.Fatalf("standing list: %v", err)
	}
	rows, _ := list["orders"].([]any)
	var found map[string]any
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["id"] == managedID {
			found = row
			break
		}
	}
	if found == nil {
		t.Fatalf("managed standing order %s missing from list", managedID)
	}
	if found["target_status"] != "blocked" {
		t.Fatalf("target_status = %v, want blocked in standing list row: %+v", found["target_status"], found)
	}
	if errText, _ := found["target_error"].(string); !strings.Contains(errText, "managed sub-agent") {
		t.Fatalf("target_error = %q, want managed sub-agent", errText)
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
	k, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
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

	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "lead", "model": "m"},
	}); err != nil {
		t.Fatalf("lead add: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{
		"profile": map[string]any{"slug": "worker", "parent_agent": "lead", "direct_callable": false},
	}); err != nil {
		t.Fatalf("worker add: %v", err)
	}
	if _, _, err := k.UpdateStanding(id, func(o *standing.Order) {
		o.Agent = "worker"
	}); err != nil {
		t.Fatalf("force corrupt standing agent: %v", err)
	}
	firedID = ""
	if _, err := c.Call(ctx, controlplane.CmdStandingFire, map[string]any{"id": id}); err == nil ||
		!strings.Contains(err.Error(), "managed sub-agent") {
		t.Fatalf("standing fire bound to managed agent err = %v, want managed sub-agent", err)
	}
	if firedID != "" {
		t.Fatalf("standing fire callback should not run for invalid agent, got %q", firedID)
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
