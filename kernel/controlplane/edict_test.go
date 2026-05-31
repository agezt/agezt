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

// TestEdictDeny_AddListRemove walks the runtime-management lifecycle:
// add a rule, see it in the list tagged removable, confirm it actually
// hard-denies via edict_test, then remove it and confirm it's gone.
func TestEdictDeny_AddListRemove(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// Add a scoped rule.
	add, err := c.Call(ctx, controlplane.CmdEdictDenyAdd, map[string]any{"rule": "shell:kubectl delete"})
	if err != nil {
		t.Fatalf("deny add: %v", err)
	}
	name, _ := add["name"].(string)
	if name == "" {
		t.Fatalf("deny add returned no name: %v", add)
	}

	// It must now appear in the list, flagged removable.
	list, err := c.Call(ctx, controlplane.CmdEdictDenyList, nil)
	if err != nil {
		t.Fatalf("deny list: %v", err)
	}
	rules, _ := list["rules"].([]any)
	var found map[string]any
	for _, raw := range rules {
		r, _ := raw.(map[string]any)
		if n, _ := r["name"].(string); n == name {
			found = r
		}
	}
	if found == nil {
		t.Fatalf("added rule %q not in list: %v", name, rules)
	}
	if removable, _ := found["removable"].(bool); !removable {
		t.Errorf("runtime rule %q should be removable", name)
	}

	// And it must actually fire through the engine.
	probe, err := c.Call(ctx, controlplane.CmdEdictTest, map[string]any{
		"capability": "shell", "input": "kubectl delete ns prod",
	})
	if err != nil {
		t.Fatalf("edict test: %v", err)
	}
	if d, _ := probe["decision"].(string); d != "deny" {
		t.Errorf("added rule did not deny: decision=%q", d)
	}
	if hd, _ := probe["hard_denied"].(bool); !hd {
		t.Error("added rule should hard-deny")
	}

	// Remove it.
	rm, err := c.Call(ctx, controlplane.CmdEdictDenyRemove, map[string]any{"name": name})
	if err != nil {
		t.Fatalf("deny rm: %v", err)
	}
	if removed, _ := rm["removed"].(bool); !removed {
		t.Errorf("deny rm did not remove %q: %v", name, rm)
	}

	// After removal the probe is no longer hard-denied.
	probe2, _ := c.Call(ctx, controlplane.CmdEdictTest, map[string]any{
		"capability": "shell", "input": "kubectl delete ns prod",
	})
	if hd, _ := probe2["hard_denied"].(bool); hd {
		t.Error("rule should no longer fire after removal")
	}
}

// TestEdictDeny_CannotRemoveFloorRule verifies the security invariant:
// a built-in floor rule cannot be removed at runtime — the handler
// surfaces an error rather than silently dropping the kernel's deny.
func TestEdictDeny_CannotRemoveFloorRule(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdEdictDenyRemove,
		map[string]any{"name": "rm-rf-root"})
	if err == nil {
		t.Fatal("removing a built-in floor rule must error, not succeed")
	}
}

// TestEdictDeny_AddRejectsMultiAndEmpty guards the input contract:
// exactly one rule per add, and no empty-substring footgun.
func TestEdictDeny_AddRejectsMultiAndEmpty(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdEdictDenyAdd,
		map[string]any{"rule": "a ; b"}); err == nil {
		t.Error("multi-rule spec should be rejected")
	}
	if _, err := c.Call(ctx, controlplane.CmdEdictDenyAdd,
		map[string]any{"rule": "shell:"}); err == nil {
		t.Error("empty-substring rule should be rejected")
	}
}

// TestEdictSetLevel_RuntimeChange walks a level change end to end: flip
// shell to deny (L0), confirm edict_test now denies it and edict_show
// reflects the new level, then flip it back to allow (L4).
func TestEdictSetLevel_RuntimeChange(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	set, err := c.Call(ctx, controlplane.CmdEdictSetLevel, map[string]any{
		"capability": "shell", "level": "L0",
	})
	if err != nil {
		t.Fatalf("set level: %v", err)
	}
	if to, _ := set["to"].(string); to != "L0" {
		t.Errorf("to = %q want L0", to)
	}
	// from must be the F3 default for shell (L2), proving we captured it.
	if from, _ := set["from"].(string); from != "L2" {
		t.Errorf("from = %q want L2 (shell default)", from)
	}

	probe, _ := c.Call(ctx, controlplane.CmdEdictTest, map[string]any{
		"capability": "shell", "input": "echo hi",
	})
	if d, _ := probe["decision"].(string); d != "deny" {
		t.Errorf("after L0, shell should deny; got %q", d)
	}

	show, _ := c.Call(ctx, controlplane.CmdEdictShow, nil)
	levels, _ := show["levels"].(map[string]any)
	if lvl, _ := levels["shell"].(string); lvl != "L0" {
		t.Errorf("edict show shell = %q want L0", lvl)
	}

	// Flip back to allow; the alias form must work too.
	back, err := c.Call(ctx, controlplane.CmdEdictSetLevel, map[string]any{
		"capability": "shell", "level": "allow",
	})
	if err != nil {
		t.Fatalf("set level back: %v", err)
	}
	if to, _ := back["to"].(string); to != "L4" {
		t.Errorf("to = %q want L4", to)
	}
}

// TestEdictSetLevel_RejectsBadInput pins the input contract: unknown
// capability and unparseable level are both errors, not silent no-ops.
func TestEdictSetLevel_RejectsBadInput(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdEdictSetLevel,
		map[string]any{"capability": "shel", "level": "L4"}); err == nil {
		t.Error("unknown capability should error")
	}
	if _, err := c.Call(ctx, controlplane.CmdEdictSetLevel,
		map[string]any{"capability": "shell", "level": "L9"}); err == nil {
		t.Error("unparseable level should error")
	}
}

// TestEdictSetLevel_FloorStillFires is the safety guarantee: loosening a
// capability to L4 must NOT let a hard-deny pattern through.
func TestEdictSetLevel_FloorStillFires(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdEdictSetLevel,
		map[string]any{"capability": "shell", "level": "L4"}); err != nil {
		t.Fatalf("set level: %v", err)
	}
	probe, _ := c.Call(ctx, controlplane.CmdEdictTest, map[string]any{
		"capability": "shell", "input": "rm -rf /",
	})
	if hd, _ := probe["hard_denied"].(bool); !hd {
		t.Error("hard-deny floor must fire even at L4")
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
