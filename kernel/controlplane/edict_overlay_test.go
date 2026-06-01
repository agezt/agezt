// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestEdictOverlay_FoldsPolicyChanges — `agt edict overlay` folds policy.changed
// into the net overlay (last-wins level, surviving deny rules, mode) (M94).
func TestEdictOverlay_FoldsPolicyChanges(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	pc := func(payload map[string]any) {
		k.Bus().Publish(event.Spec{
			Subject: "edict", Kind: event.KindPolicyChanged, Actor: "operator",
			Payload: payload,
		})
	}
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L1"})
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L3"}) // last-wins
	pc(map[string]any{"action": "mode.set", "to": "deny"})
	pc(map[string]any{"action": "deny.add", "name": "r1", "substring": "rm -rf", "applies_to": []string{"shell"}})
	pc(map[string]any{"action": "deny.add", "name": "r2", "substring": "secret"})
	pc(map[string]any{"action": "deny.rm", "name": "r2"}) // removed → should not survive

	res, err := c.Call(context.Background(), controlplane.CmdEdictOverlay, nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty, _ := res["empty"].(bool); empty {
		t.Fatal("overlay reported empty despite changes")
	}
	levels, _ := res["levels"].(map[string]any)
	if levels["shell"] != "L3" {
		t.Errorf("shell level = %v want L3 (last-wins)", levels["shell"])
	}
	if mode, _ := res["mode"].(string); mode == "" {
		t.Errorf("mode override missing")
	}
	denies, _ := res["deny_rules"].([]any)
	if len(denies) != 1 {
		t.Fatalf("deny_rules = %d want 1 (r2 removed)", len(denies))
	}
	d0, _ := denies[0].(map[string]any)
	if d0["name"] != "r1" {
		t.Errorf("surviving deny = %v want r1", d0["name"])
	}
}
