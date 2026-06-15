// SPDX-License-Identifier: MIT

package governor

import (
	"slices"
	"testing"
)

// Named reusable fallback chains (M963): a chain is a named, ordered model
// list. Any "@<name>" slot in a resolved model list expands to that chain's
// models at the single point completeChained builds the final ladder, so
// agent/task/explicit/default sources all get expansion for free. These
// white-box tests pin expandChains and the default-chain fallback.

func TestExpandChains_ReplacesReferences(t *testing.T) {
	g := newBudgetGov(Config{})
	g.SetFallbackChains(map[string][]string{
		"fast": {"a", "b"},
		"big":  {"c", "d"},
	}, "")

	got := g.expandChains([]string{"@fast", "x", "@big"})
	want := []string{"a", "b", "x", "c", "d"}
	if !slices.Equal(got, want) {
		t.Fatalf("expandChains = %v, want %v", got, want)
	}
}

func TestExpandChains_FlattenAndDedup(t *testing.T) {
	g := newBudgetGov(Config{})
	g.SetFallbackChains(map[string][]string{
		"fast": {"a", "b"},
		"alt":  {"b", "c"}, // b overlaps fast
	}, "")

	got := g.expandChains([]string{"@fast", "@alt", "a"})
	want := []string{"a", "b", "c"} // order preserved, duplicates dropped
	if !slices.Equal(got, want) {
		t.Fatalf("expandChains dedup = %v, want %v", got, want)
	}
}

func TestExpandChains_UnknownDropped(t *testing.T) {
	g := newBudgetGov(Config{})
	g.SetFallbackChains(map[string][]string{"fast": {"a"}}, "")

	// A deleted/unknown chain reference must not crash a run — it is simply
	// dropped, leaving the real ids behind.
	got := g.expandChains([]string{"@gone", "real", "@fast"})
	want := []string{"real", "a"}
	if !slices.Equal(got, want) {
		t.Fatalf("expandChains unknown = %v, want %v", got, want)
	}
}

func TestExpandChains_PassThroughPlainIDs(t *testing.T) {
	g := newBudgetGov(Config{})
	in := []string{"a", "b", "c"}
	got := g.expandChains(slices.Clone(in))
	if !slices.Equal(got, in) {
		t.Fatalf("expandChains plain = %v, want %v", got, in)
	}
}

func TestDefaultChainModels(t *testing.T) {
	g := newBudgetGov(Config{})
	if got := g.defaultChainModels(); got != nil {
		t.Fatalf("no default set: defaultChainModels = %v, want nil", got)
	}
	g.SetFallbackChains(map[string][]string{"std": {"a", "b"}}, "std")
	got := g.defaultChainModels()
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("defaultChainModels = %v, want [a b]", got)
	}
	// Default pointing at a missing chain yields nothing (no crash).
	g.SetFallbackChains(map[string][]string{"std": {"a", "b"}}, "gone")
	if got := g.defaultChainModels(); got != nil {
		t.Fatalf("default to missing chain: got %v, want nil", got)
	}
}

func TestFallbackChainsView_Roundtrip(t *testing.T) {
	g := newBudgetGov(Config{})
	in := map[string][]string{"fast": {"a", "b"}}
	g.SetFallbackChains(in, "fast")
	chains, def := g.FallbackChainsView()
	if def != "fast" {
		t.Fatalf("default = %q, want fast", def)
	}
	if !slices.Equal(chains["fast"], []string{"a", "b"}) {
		t.Fatalf("view = %v, want [a b]", chains["fast"])
	}
	// View must be a copy: mutating it must not corrupt the registry.
	chains["fast"][0] = "zzz"
	again, _ := g.FallbackChainsView()
	if again["fast"][0] != "a" {
		t.Fatal("FallbackChainsView leaked the internal slice")
	}
}
