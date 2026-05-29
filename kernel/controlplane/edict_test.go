// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestEdictShow_ReturnsDefaultPolicy verifies the wire shape: an
// edict.New(edict.Options{}) engine (which is what startPair gets
// through runtime.Open's default) loads DefaultLevels and
// DefaultHardDeny. The handler must surface both, plus the
// ask_policy label ("allow" by default).
func TestEdictShow_ReturnsDefaultPolicy(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	res, err := c.Call(context.Background(), controlplane.CmdEdictShow, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	if got, _ := res["ask_policy"].(string); got != "allow" {
		t.Errorf("ask_policy = %q want %q", got, "allow")
	}

	levels, ok := res["levels"].(map[string]any)
	if !ok {
		t.Fatalf("levels wrong type: %T", res["levels"])
	}
	// DefaultLevels seeds 8 capabilities (shell, file_read, file_list,
	// file_write, file_delete, http_get, http_post, provider_call) per
	// edict.DefaultLevels(). If new caps land, this test reminds us to
	// confirm the wire shape still works for them.
	if len(levels) < 8 {
		t.Errorf("levels len = %d, want at least 8 (DefaultLevels count)", len(levels))
	}
	// Spot-check a high-value entry — shell is operator-critical;
	// regressions on its default would be a security event.
	// DECISIONS F3 vocabulary: TrustLevel.String() emits "L0".."L4"
	// (where L2 = ask-first; see kernel/edict/edict.go).
	if lvl, _ := levels["shell"].(string); lvl != "L2" {
		t.Errorf("shell level = %q want %q (DECISIONS F3, L2 = ask-first)", lvl, "L2")
	}

	rules, ok := res["hard_deny"].([]any)
	if !ok {
		t.Fatalf("hard_deny wrong type: %T", res["hard_deny"])
	}
	if len(rules) == 0 {
		t.Fatal("hard_deny is empty; DefaultHardDeny seeds several entries (fork-bomb, rm-rf-root, ...)")
	}
	// Each rule must carry the documented shape: name, substring,
	// applies_to (which can be null when global).
	first, _ := rules[0].(map[string]any)
	for _, k := range []string{"name", "substring", "applies_to"} {
		if _, present := first[k]; !present {
			t.Errorf("hard_deny[0] missing key %q; got %v", k, first)
		}
	}
}

// TestEdictShow_HardDenyRulesAreSortedByName covers the
// deterministic-output promise. Operators diffing `agt edict
// show` output across calls/deployments shouldn't see the row
// order flicker.
func TestEdictShow_HardDenyRulesAreSortedByName(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	res, err := c.Call(context.Background(), controlplane.CmdEdictShow, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	rules, _ := res["hard_deny"].([]any)
	var prevName string
	for i, raw := range rules {
		r, _ := raw.(map[string]any)
		name, _ := r["name"].(string)
		if i > 0 && name < prevName {
			t.Errorf("hard_deny not sorted at index %d: %q < %q", i, name, prevName)
		}
		prevName = name
	}
}
