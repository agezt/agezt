// SPDX-License-Identifier: MIT

package catalog_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
)

// catWithModels builds a single-provider catalog for the down-routing tests.
func catWithModels(provID string, models map[string]*catalog.Model) *catalog.Catalog {
	c := catalog.NewEmpty()
	c.Providers[provID] = &catalog.Provider{ID: provID, Models: models}
	return c
}

// allEligible accepts every provider (for tests that don't constrain).
func allEligible(string) bool { return true }

// TestToolCapableAlternative_PrefersLargestContextSibling (M37) — among the
// provider's tool-capable models, the one with the largest context wins.
func TestToolCapableAlternative_PrefersLargestContextSibling(t *testing.T) {
	c := catWithModels("acme", map[string]*catalog.Model{
		"mini":  {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}},
		"small": {ID: "small", ToolCall: true, Limit: catalog.Limit{Context: 16000}},
		"large": {ID: "large", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
	})
	alt, ok := c.ToolCapableAlternative("mini")
	if !ok {
		t.Fatal("expected an alternative for an incapable model")
	}
	if alt != "large" {
		t.Errorf("alt = %q want large (largest-context capable sibling)", alt)
	}
}

// TestToolCapableAlternative_TieBreaksByID — equal context → lowest ID wins,
// so the choice is deterministic despite random map iteration.
func TestToolCapableAlternative_TieBreaksByID(t *testing.T) {
	c := catWithModels("acme", map[string]*catalog.Model{
		"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}},
		"bbb":  {ID: "bbb", ToolCall: true, Limit: catalog.Limit{Context: 32000}},
		"aaa":  {ID: "aaa", ToolCall: true, Limit: catalog.Limit{Context: 32000}},
	})
	for i := 0; i < 20; i++ { // repeat: map order is randomised
		if alt, _ := c.ToolCapableAlternative("mini"); alt != "aaa" {
			t.Fatalf("alt = %q want aaa (tie-break by id)", alt)
		}
	}
}

// TestToolCapableAlternative_NoneWhenNoCapableSibling — a provider whose
// only other models are also tool-incapable has no alternative.
func TestToolCapableAlternative_NoneWhenNoCapableSibling(t *testing.T) {
	c := catWithModels("acme", map[string]*catalog.Model{
		"mini":  {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}},
		"micro": {ID: "micro", ToolCall: false, Limit: catalog.Limit{Context: 4000}},
	})
	if alt, ok := c.ToolCapableAlternative("mini"); ok {
		t.Errorf("expected no alternative, got %q", alt)
	}
}

// TestToolCapableAlternative_UnknownModel — an unknown model id yields no
// alternative (not a panic).
func TestToolCapableAlternative_UnknownModel(t *testing.T) {
	c := catWithModels("acme", map[string]*catalog.Model{
		"big": {ID: "big", ToolCall: true, Limit: catalog.Limit{Context: 100000}},
	})
	if _, ok := c.ToolCapableAlternative("ghost"); ok {
		t.Error("unknown model should have no alternative")
	}
}

// TestToolCapableAlternative_ExcludesSelf — a model never reroutes to
// itself even if (hypothetically) it were marked capable.
func TestToolCapableAlternative_ExcludesSelf(t *testing.T) {
	c := catWithModels("acme", map[string]*catalog.Model{
		"only": {ID: "only", ToolCall: true, Limit: catalog.Limit{Context: 100000}},
	})
	if alt, ok := c.ToolCapableAlternative("only"); ok {
		t.Errorf("a sole model must not reroute to itself, got %q", alt)
	}
}

// --- M40: cross-provider down-routing ---

// catMulti builds a catalog with several providers.
func catMulti(providers map[string]map[string]*catalog.Model) *catalog.Catalog {
	c := catalog.NewEmpty()
	for pid, models := range providers {
		c.Providers[pid] = &catalog.Provider{ID: pid, Models: models}
	}
	return c
}

// TestToolCapableAlternativeAmong_PrefersSameProvider — even with an
// eligible cross-provider option, a same-provider capable sibling wins.
func TestToolCapableAlternativeAmong_PrefersSameProvider(t *testing.T) {
	c := catMulti(map[string]map[string]*catalog.Model{
		"acme": {
			"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}},
			"sib":  {ID: "sib", ToolCall: true, Limit: catalog.Limit{Context: 16000}},
		},
		"other": {
			"huge": {ID: "huge", ToolCall: true, Limit: catalog.Limit{Context: 500000}},
		},
	})
	alt, ok := c.ToolCapableAlternativeAmong("mini", allEligible)
	if !ok || alt != "sib" {
		t.Errorf("alt = %q (ok=%v), want same-provider sib even though other/huge is bigger", alt, ok)
	}
}

// TestToolCapableAlternativeAmong_CrossesWhenNoSibling — with no capable
// sibling in-provider, the search widens to an eligible other provider.
func TestToolCapableAlternativeAmong_CrossesWhenNoSibling(t *testing.T) {
	c := catMulti(map[string]map[string]*catalog.Model{
		"acme": {
			"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}},
		},
		"big": {
			"flagship": {ID: "flagship", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	})
	alt, ok := c.ToolCapableAlternativeAmong("mini", allEligible)
	if !ok || alt != "flagship" {
		t.Errorf("alt = %q (ok=%v), want cross-provider flagship", alt, ok)
	}
}

// TestToolCapableAlternativeAmong_RespectsEligibility — a cross-provider
// capable model on an INELIGIBLE provider is never chosen.
func TestToolCapableAlternativeAmong_RespectsEligibility(t *testing.T) {
	c := catMulti(map[string]map[string]*catalog.Model{
		"acme": {
			"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}},
		},
		"unreg": {
			"strong": {ID: "strong", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	})
	// Only "acme" is eligible; "unreg" (the only capable provider) is not.
	onlyAcme := func(id string) bool { return id == "acme" }
	if alt, ok := c.ToolCapableAlternativeAmong("mini", onlyAcme); ok {
		t.Errorf("must not route to an ineligible provider, got %q", alt)
	}
}

// TestToolCapableAlternativeAmong_PicksLargestEligibleCross — among multiple
// eligible cross providers, the largest-context capable model wins.
func TestToolCapableAlternativeAmong_PicksLargestEligibleCross(t *testing.T) {
	c := catMulti(map[string]map[string]*catalog.Model{
		"acme": {"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}}},
		"p1":   {"m1": {ID: "m1", ToolCall: true, Limit: catalog.Limit{Context: 32000}}},
		"p2":   {"m2": {ID: "m2", ToolCall: true, Limit: catalog.Limit{Context: 128000}}},
	})
	alt, ok := c.ToolCapableAlternativeAmong("mini", allEligible)
	if !ok || alt != "m2" {
		t.Errorf("alt = %q (ok=%v), want m2 (largest cross-provider)", alt, ok)
	}
}

// TestToolCapableAlternativeAmong_TieBreaksByIDAcrossProviders — when two eligible
// cross-providers each offer a tool-capable model of EQUAL context, the lowest model
// id wins, deterministically (independent of provider order). The existing cross tests
// only cover largest-context, leaving the cross-provider tie-break (types.go Pass 2)
// unpinned; mutation testing (M508) showed the `id < bestID` tie-break and the
// `ctx > bestCtx` comparison could flip undetected. Two arrangements (lowest id in the
// earlier vs the later provider) pin both directions.
func TestToolCapableAlternativeAmong_TieBreaksByIDAcrossProviders(t *testing.T) {
	for _, tc := range []struct {
		name   string
		models map[string]map[string]*catalog.Model
	}{
		{"lowest-id-in-earlier-provider", map[string]map[string]*catalog.Model{
			"acme": {"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}}},
			"p1":   {"alpha": {ID: "alpha", ToolCall: true, Limit: catalog.Limit{Context: 64000}}},
			"p2":   {"zeta": {ID: "zeta", ToolCall: true, Limit: catalog.Limit{Context: 64000}}},
		}},
		{"lowest-id-in-later-provider", map[string]map[string]*catalog.Model{
			"acme": {"mini": {ID: "mini", ToolCall: false, Limit: catalog.Limit{Context: 8000}}},
			"p1":   {"zeta": {ID: "zeta", ToolCall: true, Limit: catalog.Limit{Context: 64000}}},
			"p2":   {"alpha": {ID: "alpha", ToolCall: true, Limit: catalog.Limit{Context: 64000}}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := catMulti(tc.models)
			alt, ok := c.ToolCapableAlternativeAmong("mini", allEligible)
			if !ok || alt != "alpha" {
				t.Errorf("alt = %q (ok=%v), want alpha (equal context across providers -> lowest id)", alt, ok)
			}
		})
	}
}

// vis builds a vision-capable model.
func vis(id string, ctx int) *catalog.Model {
	return &catalog.Model{ID: id, Modalities: catalog.Modalities{Input: []string{"text", "image"}}, Limit: catalog.Limit{Context: ctx}}
}

// txt builds a text-only model.
func txt(id string, ctx int) *catalog.Model {
	return &catalog.Model{ID: id, Modalities: catalog.Modalities{Input: []string{"text"}}, Limit: catalog.Limit{Context: ctx}}
}

// TestVisionCapableAmong_PicksLargestVisionModel (M821) — among eligible
// providers, the first (sorted) with a vision model returns its largest-context
// vision model.
func TestVisionCapableAmong_PicksLargestVisionModel(t *testing.T) {
	c := catalog.NewEmpty()
	c.Providers["acme"] = &catalog.Provider{ID: "acme", Models: map[string]*catalog.Model{
		"a-text": txt("a-text", 99999),
		"a-vis":  vis("a-vis", 8000),
		"a-vbig": vis("a-vbig", 200000),
	}}
	for i := 0; i < 20; i++ { // map order randomised
		id, ok := c.VisionCapableAmong(allEligible)
		if !ok || id != "a-vbig" {
			t.Fatalf("VisionCapableAmong = %q,%v want a-vbig,true", id, ok)
		}
	}
}

// TestVisionCapableAmong_SkipsIneligibleAndTextOnly — only eligible providers
// with an actual vision model qualify; a provider filtered out by the predicate
// (e.g. uncredentialed) is skipped even if it has a vision model.
func TestVisionCapableAmong_SkipsIneligibleAndTextOnly(t *testing.T) {
	c := catalog.NewEmpty()
	c.Providers["aaa"] = &catalog.Provider{ID: "aaa", Models: map[string]*catalog.Model{"t": txt("t", 1000)}} // text only
	c.Providers["bbb"] = &catalog.Provider{ID: "bbb", Models: map[string]*catalog.Model{"v": vis("v", 1000)}} // vision but ineligible
	c.Providers["ccc"] = &catalog.Provider{ID: "ccc", Models: map[string]*catalog.Model{"cv": vis("cv", 1000)}}

	id, ok := c.VisionCapableAmong(func(p string) bool { return p != "bbb" })
	if !ok || id != "cv" {
		t.Fatalf("VisionCapableAmong = %q,%v want cv,true (aaa text-only, bbb ineligible)", id, ok)
	}

	// No eligible provider has a vision model → (\"\", false).
	if id, ok := c.VisionCapableAmong(func(p string) bool { return p == "aaa" }); ok {
		t.Errorf("VisionCapableAmong over text-only provider = %q,true want \"\",false", id)
	}
}
