// SPDX-License-Identifier: MIT

package runctx

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/edict"
)

// The basic setters are the run's dispatch knobs: which model, which system
// prompt, which attachments, which budget, which ceiling. They moved to the
// runs module in W3.76c.

func TestWithModelAndSystem_CarryTheirValues(t *testing.T) {
	ctx := WithModel(context.Background(), "m-1")
	ctx = WithSystem(ctx, "sys-1")

	if got := ModelFromCtx(ctx); got != "m-1" {
		t.Fatalf("model = %q, want m-1", got)
	}
	if got := SystemFromCtx(ctx); got != "sys-1" {
		t.Fatalf("system = %q, want sys-1", got)
	}
}

// TestWithModelAndSystem_AnExplicitOverrideHasNoProfile pins the SOURCE half
// of a run setting: an explicit per-run override has no profile behind it, and
// the boot resumer must be able to tell the two apart (a profile-sourced
// value is reconstructible; an override is not).
func TestWithModelAndSystem_AnExplicitOverrideHasNoProfile(t *testing.T) {
	ctx := WithModel(context.Background(), "m-1")
	if got := ModelSetting(ctx).ProfileSlug; got != "" {
		t.Fatalf("an explicit override must carry no profile slug, got %q", got)
	}
	if got := SystemSetting(context.Background()); got.Value != "" || got.ProfileSlug != "" {
		t.Fatalf("an unset system setting = %+v, want the empty setting", got)
	}
}

// TestWithModel_EmptyIsANoOp pins the no-op rule: an empty model leaves the
// run's model alone (the kernel's configured model applies).
func TestWithModel_EmptyIsANoOp(t *testing.T) {
	ctx := WithModel(context.Background(), "")
	if got := ModelFromCtx(ctx); got != "" {
		t.Fatalf("model = %q, want the empty override", got)
	}
	// An empty override must not ERASE a model the profile already bound: the
	// profile's model is the run's model until something replaces it.
	bound := WithModel(WithAgentProfile(context.Background(), agentapi.Profile{
		Slug:  "ops",
		Model: "ops-model",
	}), "")
	if got := ModelFromCtx(bound); got != "ops-model" {
		t.Fatalf("an empty override erased the profile's model: %q", got)
	}
}

// TestWithImages_CarriesTheAttachments pins the vision-gated attachment
// channel: an empty list is a no-op, a non-empty one rides on the context.
func TestWithImages_CarriesTheAttachments(t *testing.T) {
	if got := ImagesFromCtx(context.Background()); got != nil {
		t.Fatalf("images = %+v, want none", got)
	}
	ctx := WithImages(context.Background(), []string{"img-1", "img-2"})
	got := ImagesFromCtx(ctx)
	if len(got) != 2 || got[0] != "img-1" || got[1] != "img-2" {
		t.Fatalf("images = %+v, want [img-1 img-2]", got)
	}
	if got := ImagesFromCtx(WithImages(context.Background(), nil)); got != nil {
		t.Fatalf("images = %+v, want none", got)
	}
}

// TestWithJSONMode_CarriesTheStructuredOutputRequest pins the structured
// output flag: only an explicit TRUE asks for it.
func TestWithJSONMode_CarriesTheStructuredOutputRequest(t *testing.T) {
	if JSONModeFromCtx(context.Background()) {
		t.Fatal("json mode must be off by default")
	}
	if JSONModeFromCtx(WithJSONMode(context.Background(), false)) {
		t.Fatal("an explicit false must stay false")
	}
	if !JSONModeFromCtx(WithJSONMode(context.Background(), true)) {
		t.Fatal("an explicit true must be carried")
	}
}

// TestWithRunTimeoutAndMaxCost_CarryTheirBudgets pins the two budget knobs:
// a non-positive value is a no-op, a positive one is carried.
func TestWithRunTimeoutAndMaxCost_CarryTheirBudgets(t *testing.T) {
	if got := RunTimeoutFromCtx(context.Background()); got != 0 {
		t.Fatalf("run timeout = %v, want none", got)
	}
	if got := MaxCostFromCtx(context.Background()); got != 0 {
		t.Fatalf("max cost = %d, want none", got)
	}
	ctx := WithRunTimeout(context.Background(), 30*time.Second)
	ctx = WithMaxCost(ctx, 500)
	if got := RunTimeoutFromCtx(ctx); got != 30*time.Second {
		t.Fatalf("run timeout = %v, want 30s", got)
	}
	if got := MaxCostFromCtx(ctx); got != 500 {
		t.Fatalf("max cost = %d, want 500", got)
	}
	if got := RunTimeoutFromCtx(WithRunTimeout(context.Background(), 0)); got != 0 {
		t.Fatalf("run timeout = %v, want none", got)
	}
	if got := MaxCostFromCtx(WithMaxCost(context.Background(), 0)); got != 0 {
		t.Fatalf("max cost = %d, want none", got)
	}
}

// TestWithTrustCeiling_NeverLoosensTheInheritedBound pins the security rule
// the ceiling carries: it tightens monotonically down a delegation tree, so a
// child profile can never loosen the parent's cap (CWE-269, VULN-001). The
// levels run from the TIGHTEST (LevelDeny) to the LOOSEST (LevelAllow), so
// "tighter" means the smaller number.
func TestWithTrustCeiling_NeverLoosensTheInheritedBound(t *testing.T) {
	tight := WithTrustCeiling(context.Background(), edict.LevelAskFirst)
	lvl, ok := TrustCeilingFromCtx(tight)
	if !ok || lvl != edict.LevelAskFirst {
		t.Fatalf("trust ceiling = %v/%v, want the tight cap", lvl, ok)
	}
	// A LOOSER ceiling must not widen the inherited one.
	looser := WithTrustCeiling(tight, edict.LevelAllow)
	lvl, _ = TrustCeilingFromCtx(looser)
	if lvl != edict.LevelAskFirst {
		t.Fatalf("trust ceiling = %v, want the inherited tight cap", lvl)
	}
	// A TIGHTER ceiling does apply.
	tighter := WithTrustCeiling(tight, edict.LevelDeny)
	lvl, _ = TrustCeilingFromCtx(tighter)
	if lvl != edict.LevelDeny {
		t.Fatalf("trust ceiling = %v, want the tighter cap", lvl)
	}
}

// TestWithTrustCeiling_NoClampKeepsTheInheritedCap pins the "no clamp" rule:
// LevelAllow means "no ceiling", and it must not ERASE an inherited cap.
func TestWithTrustCeiling_NoClampKeepsTheInheritedCap(t *testing.T) {
	tight := WithTrustCeiling(context.Background(), edict.LevelDeny)
	lvl, ok := TrustCeilingFromCtx(WithTrustCeiling(tight, edict.LevelAllow))
	if !ok || lvl != edict.LevelDeny {
		t.Fatalf("trust ceiling = %v/%v, want the inherited tight cap", lvl, ok)
	}
	// "No clamp" means NO ceiling at all: the run must not carry one, or the
	// Governor would read a cap that was never asked for.
	lvl, ok = TrustCeilingFromCtx(WithTrustCeiling(context.Background(), edict.LevelAllow))
	if ok {
		t.Fatalf("a no-clamp ceiling must not be recorded, got %v", lvl)
	}
}

// TestWithRoot_CarriesTheDelegationTreeRoot pins the tree attribution: the
// root correlation rides on the context so every descendant counts against the
// same tree.
func TestWithRoot_CarriesTheDelegationTreeRoot(t *testing.T) {
	if got := RootFromCtx(context.Background()); got != "" {
		t.Fatalf("root = %q, want none", got)
	}
	ctx := WithRoot(context.Background(), "corr-root")
	if got := RootFromCtx(ctx); got != "corr-root" {
		t.Fatalf("root = %q, want corr-root", got)
	}
}

// TestCloneStringMap_CopiesAndNils pins the copy helper the config overrides
// ride on: the caller gets its own map, and an empty map reads as nil.
func TestCloneStringMap_CopiesAndNils(t *testing.T) {
	if got := CloneStringMap(nil); got != nil {
		t.Fatalf("clone = %+v, want nil", got)
	}
	src := map[string]string{"A": "1"}
	got := CloneStringMap(src)
	got["A"] = "2"
	if src["A"] != "1" {
		t.Fatalf("the source map was mutated: %+v", src)
	}
}
