// SPDX-License-Identifier: MIT

package acpcatalog

import (
	"context"
	"testing"
)

func TestFind(t *testing.T) {
	if a, ok := Find("gemini"); !ok || a.Bin != "gemini" {
		t.Fatalf("Find(gemini) = %+v, %v", a, ok)
	}
	if a, ok := Find("GEMINI"); !ok || a.Slug != "gemini" {
		t.Fatalf("Find is case-insensitive: %+v %v", a, ok)
	}
	if _, ok := Find("nope"); ok {
		t.Fatal("Find(nope) should not resolve")
	}
}

func TestResolveCommand(t *testing.T) {
	// Empty ref → fallback (the configured default).
	if cmd, ok := ResolveCommand("", "custom --acp"); !ok || cmd != "custom --acp" {
		t.Fatalf("empty ref should use fallback: %q %v", cmd, ok)
	}
	// Empty ref and no fallback → nothing usable.
	if _, ok := ResolveCommand("", ""); ok {
		t.Fatal("empty ref + empty fallback should be unresolved")
	}
	// SECURITY (CWE-78): a non-slug ref must NOT be executed as a raw command —
	// agent input could otherwise inject an arbitrary shell line via the bridge.
	if cmd, ok := ResolveCommand("my-acp-binary --flag", ""); ok {
		t.Fatalf("non-slug ref must be rejected, not run as a raw command: got %q", cmd)
	}
	// Even with an operator fallback present, an explicit unknown selector is
	// rejected rather than silently passed through or downgraded to the default.
	if cmd, ok := ResolveCommand("; rm -rf ~", "custom --acp"); ok {
		t.Fatalf("injection-shaped ref must be rejected: got %q", cmd)
	}
}

func TestDetect_CoversCatalogAndFlagsActive(t *testing.T) {
	inv := Detect(context.Background(), "gemini --experimental-acp")
	if len(inv.Agents) != len(Catalog) {
		t.Fatalf("Detect returned %d agents, want %d", len(inv.Agents), len(Catalog))
	}
	if inv.InstalledCount+inv.MissingCount != len(Catalog) {
		t.Fatalf("counts don't sum: installed=%d missing=%d total=%d", inv.InstalledCount, inv.MissingCount, len(Catalog))
	}
	var geminiActive, otherActive bool
	for _, st := range inv.Agents {
		if st.Slug == "gemini" {
			geminiActive = st.Active
		} else if st.Active {
			otherActive = true
		}
	}
	if !geminiActive {
		t.Fatal("gemini should be flagged active for 'gemini --experimental-acp'")
	}
	if otherActive {
		t.Fatal("only the matching agent should be active")
	}
}

func TestDetect_NoActiveWhenUnset(t *testing.T) {
	inv := Detect(context.Background(), "")
	if inv.ActiveCommand != "" {
		t.Fatalf("active command should be empty, got %q", inv.ActiveCommand)
	}
	for _, st := range inv.Agents {
		if st.Active {
			t.Fatalf("no agent should be active when default is unset: %s", st.Slug)
		}
	}
}
