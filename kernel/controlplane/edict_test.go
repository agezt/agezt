// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"io"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/tenant"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// newTenantReg builds a registry of real per-tenant kernels (each a fresh
// runtime.Open) for the per-tenant policy tests, and wires it into srv.
func newTenantReg(t *testing.T, srv *controlplane.Server) {
	t.Helper()
	reg, err := tenant.New(t.TempDir(), func(id, baseDir string) (io.Closer, error) {
		return runtime.Open(runtime.Config{
			BaseDir:  baseDir,
			Provider: mock.New(mock.FinalText("t")),
			Tools:    map[string]agent.Tool{},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.CloseAll() })
	srv.SetTenants(reg)
}

// hardDenied is a tiny helper: does an edict_test probe report hard_denied?
func hardDenied(t *testing.T, c *controlplane.Client, args map[string]any) bool {
	t.Helper()
	res, err := c.Call(context.Background(), controlplane.CmdEdictTest, args)
	if err != nil {
		t.Fatalf("edict_test %v: %v", args, err)
	}
	hd, _ := res["hard_denied"].(bool)
	return hd
}

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
	// Spot-check a high-value entry — under the M814 owner posture every
	// capability defaults to L4 (allow); restriction is the operator's
	// opt-OUT. TrustLevel.String() emits "L0".."L4".
	if lvl, _ := levels["shell"].(string); lvl != "L4" {
		t.Errorf("shell level = %q want %q (M814 allow-by-default)", lvl, "L4")
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
	// from must be the M814 default for shell (L4 allow), proving we captured it.
	if from, _ := set["from"].(string); from != "L4" {
		t.Errorf("from = %q want L4 (shell default, allow-by-default)", from)
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

// TestEdictSetMode_RuntimeChange flips the approval mode to deny and
// confirms edict_show reflects it and an Ask-class capability now denies,
// then flips to prompt. Rejects an unknown mode.
func TestEdictSetMode_RuntimeChange(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	// M814: every capability is allow-by-default, so nothing is ask-class
	// out of the box — opt shell INTO ask-first to exercise mode folding.
	if _, err := c.Call(ctx, controlplane.CmdEdictSetLevel, map[string]any{"capability": "shell", "level": "L2"}); err != nil {
		t.Fatalf("opt shell into ask-first: %v", err)
	}

	set, err := c.Call(ctx, controlplane.CmdEdictSetMode, map[string]any{"mode": "deny"})
	if err != nil {
		t.Fatalf("set mode: %v", err)
	}
	if from, _ := set["from"].(string); from != "allow" {
		t.Errorf("from = %q want allow (default)", from)
	}
	if to, _ := set["to"].(string); to != "deny" {
		t.Errorf("to = %q want deny", to)
	}

	show, _ := c.Call(ctx, controlplane.CmdEdictShow, nil)
	if ap, _ := show["ask_policy"].(string); ap != "deny" {
		t.Errorf("edict show ask_policy = %q want deny", ap)
	}

	// shell is L2 (ask-class) by default; under AskDeny it must now deny.
	probe, _ := c.Call(ctx, controlplane.CmdEdictTest, map[string]any{
		"capability": "shell", "input": "echo hi",
	})
	if d, _ := probe["decision"].(string); d != "deny" {
		t.Errorf("under deny mode, ask-class shell should deny; got %q", d)
	}

	if _, err := c.Call(ctx, controlplane.CmdEdictSetMode, map[string]any{"mode": "prompt"}); err != nil {
		t.Fatalf("set mode prompt: %v", err)
	}
	if _, err := c.Call(ctx, controlplane.CmdEdictSetMode, map[string]any{"mode": "loose"}); err == nil {
		t.Error("unknown mode should error")
	}
}

// TestEdictDeny_PerTenantIsolation proves M22: a deny rule added to one
// tenant's engine is invisible to other tenants and to the primary.
func TestEdictDeny_PerTenantIsolation(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	newTenantReg(t, srv)
	ctx := context.Background()

	// Add a deny rule to tenant "alpha" only.
	if _, err := c.Call(ctx, controlplane.CmdEdictDenyAdd, map[string]any{
		"tenant": "alpha", "rule": "shell:kubectl delete",
	}); err != nil {
		t.Fatalf("add to alpha: %v", err)
	}

	// alpha denies it; beta and the primary do NOT (isolated engines).
	if !hardDenied(t, c, map[string]any{"tenant": "alpha", "capability": "shell", "input": "kubectl delete x"}) {
		t.Error("alpha should hard-deny its own rule")
	}
	if hardDenied(t, c, map[string]any{"tenant": "beta", "capability": "shell", "input": "kubectl delete x"}) {
		t.Error("beta must NOT see alpha's rule")
	}
	if hardDenied(t, c, map[string]any{"capability": "shell", "input": "kubectl delete x"}) {
		t.Error("primary must NOT see a tenant's rule")
	}

	// deny list reflects the same isolation: one runtime rule on alpha, none on beta.
	countRuntime := func(tenant string) int {
		args := map[string]any{}
		if tenant != "" {
			args["tenant"] = tenant
		}
		res, err := c.Call(ctx, controlplane.CmdEdictDenyList, args)
		if err != nil {
			t.Fatalf("deny list %q: %v", tenant, err)
		}
		rules, _ := res["rules"].([]any)
		n := 0
		for _, raw := range rules {
			if r, _ := raw.(map[string]any); r["removable"] == true {
				n++
			}
		}
		return n
	}
	if got := countRuntime("alpha"); got != 1 {
		t.Errorf("alpha runtime rules = %d, want 1", got)
	}
	if got := countRuntime("beta"); got != 0 {
		t.Errorf("beta runtime rules = %d, want 0", got)
	}
}

// TestEdictLevel_PerTenantIsolation proves a trust-level change on one
// tenant doesn't leak to another or to the primary.
func TestEdictLevel_PerTenantIsolation(t *testing.T) {
	_, srv, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	newTenantReg(t, srv)
	ctx := context.Background()

	// Lock shell to L0 on alpha.
	if _, err := c.Call(ctx, controlplane.CmdEdictSetLevel, map[string]any{
		"tenant": "alpha", "capability": "shell", "level": "L0",
	}); err != nil {
		t.Fatalf("set level alpha: %v", err)
	}
	shellLevel := func(tenant string) string {
		args := map[string]any{}
		if tenant != "" {
			args["tenant"] = tenant
		}
		res, _ := c.Call(ctx, controlplane.CmdEdictShow, args)
		levels, _ := res["levels"].(map[string]any)
		s, _ := levels["shell"].(string)
		return s
	}
	if got := shellLevel("alpha"); got != "L0" {
		t.Errorf("alpha shell = %q want L0", got)
	}
	if got := shellLevel("beta"); got == "L0" {
		t.Errorf("beta shell = %q must not be L0 (isolation leak)", got)
	}
	if got := shellLevel(""); got == "L0" {
		t.Errorf("primary shell = %q must not be L0 (isolation leak)", got)
	}
}

// TestEdict_TenantWithoutRegistry errors clearly when a tenant is named
// but multi-tenancy is disabled (no registry wired).
func TestEdict_TenantWithoutRegistry(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, err := c.Call(context.Background(), controlplane.CmdEdictShow, map[string]any{"tenant": "alpha"})
	if err == nil {
		t.Fatal("naming a tenant with no registry should error")
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
