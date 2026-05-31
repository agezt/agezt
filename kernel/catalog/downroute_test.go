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
