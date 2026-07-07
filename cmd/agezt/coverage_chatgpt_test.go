// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/governor"
)

// TestChatGPTCatalogEntry verifies the catalog metadata carries every served
// model plus the vault token key (so HasCredentials reflects sign-in state).
func TestChatGPTCatalogEntry(t *testing.T) {
	p := chatgptCatalogEntry()
	if p == nil {
		t.Fatal("chatgptCatalogEntry returned nil")
	}
	if p.ID != "chatgpt" {
		t.Errorf("ID = %q, want chatgpt", p.ID)
	}
	if len(p.Models) != len(chatgptModels) {
		t.Errorf("Models count = %d, want %d", len(p.Models), len(chatgptModels))
	}
	for _, id := range chatgptModels {
		m, ok := p.Models[id]
		if !ok {
			t.Errorf("missing model %q", id)
			continue
		}
		if !m.ToolCall || !m.Reasoning {
			t.Errorf("model %q missing ToolCall/Reasoning flags", id)
		}
	}
	if len(p.Env) == 0 || p.Env[0] != chatgptauth.VaultKey {
		t.Errorf("Env = %v, want first entry %q", p.Env, chatgptauth.VaultKey)
	}
}

// TestSeedChatGPTCatalog covers: nil store (no-op), empty store (seed once),
// and an already-seeded store (never clobbered).
func TestSeedChatGPTCatalog(t *testing.T) {
	// nil store must not panic.
	seedChatGPTCatalog(nil)

	store := catalog.NewStore(t.TempDir())

	// First seed adds the chatgpt entry.
	seedChatGPTCatalog(store)
	cur, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cur == nil || cur.Providers["chatgpt"] == nil {
		t.Fatal("chatgpt provider not seeded on first call")
	}

	// Second seed is a no-op (already present) — exercises the early return.
	seedChatGPTCatalog(store)
	cur2, err := store.Load()
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	if cur2.Providers["chatgpt"] == nil {
		t.Fatal("chatgpt provider dropped on second seed")
	}
}

// TestBuildChatGPTPrimary_NotSignedIn confirms the primary build refuses when
// no tokens exist, honoring the "registered only when signed in" rule.
func TestBuildChatGPTPrimary_NotSignedIn(t *testing.T) {
	dir := t.TempDir()
	prov, desc, auth, ok := buildChatGPTPrimary(dir, "")
	if ok {
		t.Fatal("buildChatGPTPrimary returned ok=true with no tokens")
	}
	if prov != nil || desc != "" || auth != "" {
		t.Errorf("expected zero values, got prov=%v desc=%q auth=%q", prov, desc, auth)
	}
}

// TestChatGPTTokenFn builds a token func from a manager and exercises both the
// force and non-force branches. Neither should panic; both propagate an error
// when no tokens exist, which is all we need for coverage of the closure body.
func TestChatGPTTokenFn(t *testing.T) {
	mgr := chatgptauth.NewManager(t.TempDir())
	fn := chatgptTokenFn(mgr)
	if fn == nil {
		t.Fatal("chatgptTokenFn returned nil")
	}
	// non-force path
	_, _, _ = fn(t.Context(), false)
	// force path
	_, _, _ = fn(t.Context(), true)
}

// TestRegisterChatGPTAlternate covers the two early-return guards: chatgpt is
// already primary, and not-signed-in. Both must return false without touching
// the registry.
func TestRegisterChatGPTAlternate_Guards(t *testing.T) {
	reg := governor.NewRegistry()

	// primaryName == "chatgpt" → refuse (already the primary).
	if registerChatGPTAlternate(reg, t.TempDir(), "chatgpt", false) {
		t.Error("registerChatGPTAlternate should refuse when chatgpt is primary")
	}

	// Not signed in → refuse.
	if registerChatGPTAlternate(reg, t.TempDir(), "openai", false) {
		t.Error("registerChatGPTAlternate should refuse when not signed in")
	}
	if registerChatGPTAlternate(reg, t.TempDir(), "openai", true) {
		t.Error("registerChatGPTAlternate should refuse when not signed in (replace path)")
	}
}
