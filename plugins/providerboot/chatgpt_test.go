// SPDX-License-Identifier: MIT

package providerboot

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/chatgptauth"
	"github.com/agezt/agezt/kernel/governor"
)

// cliCachePayload is a trimmed Codex CLI models_cache.json: two listed models
// out of priority order plus one hidden entry.
const cliCachePayload = `{"models":[
 {"slug":"gpt-5.4","display_name":"GPT-5.4","visibility":"list","priority":16,"context_window":300000,"base_instructions":"instr-54"},
 {"slug":"codex-auto-review","display_name":"Codex Auto Review","visibility":"hide","priority":43,"base_instructions":"instr-review"},
 {"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","visibility":"list","priority":1,"context_window":400000,"max_context_window":500000,
  "input_modalities":["text","image"],"base_instructions":"instr-sol"}
]}`

// resetChatGPTModelCache clears the process-wide discovery memo so tests don't
// leak resolved sets into each other.
func resetChatGPTModelCache(t *testing.T) {
	t.Helper()
	clear := func() {
		chatgptModelCache.mu.Lock()
		chatgptModelCache.set = chatgptModelSet{}
		chatgptModelCache.at = time.Time{}
		chatgptModelCache.mu.Unlock()
	}
	clear()
	t.Cleanup(clear)
}

// codexHome points CODEX_HOME at a temp dir, optionally seeding a models cache.
// Returns the dir. With cache == "" the CLI-cache source is absent, so
// resolution falls through to the builtin snapshot.
func codexHome(t *testing.T, cache string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	if cache != "" {
		if err := os.WriteFile(filepath.Join(dir, "models_cache.json"), []byte(cache), 0o600); err != nil {
			t.Fatalf("write models cache: %v", err)
		}
	}
	return dir
}

// TestChatGPTCatalogEntry verifies the catalog metadata carries every served
// model plus the vault token key (so HasCredentials reflects sign-in state).
func TestChatGPTCatalogEntry(t *testing.T) {
	set := builtinChatGPTModelSet()
	p := chatgptCatalogEntry(set)
	if p == nil {
		t.Fatal("chatgptCatalogEntry returned nil")
	}
	if p.ID != "chatgpt" {
		t.Errorf("ID = %q, want chatgpt", p.ID)
	}
	if len(p.Models) != len(set.IDs) {
		t.Errorf("Models count = %d, want %d", len(p.Models), len(set.IDs))
	}
	for _, id := range set.IDs {
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

// TestChatGPTCatalogEntryFromDiscovery checks that discovered metadata (display
// name, context window, modalities) reaches the catalog rather than being
// flattened to bare ids.
func TestChatGPTCatalogEntryFromDiscovery(t *testing.T) {
	set, ok := chatgptModelsFromCLICache(writeCache(t, cliCachePayload))
	if !ok {
		t.Fatal("chatgptModelsFromCLICache returned ok=false")
	}
	p := chatgptCatalogEntry(set)
	sol := p.Models["gpt-5.6-sol"]
	if sol == nil {
		t.Fatal("gpt-5.6-sol missing from catalog entry")
	}
	if sol.Name != "GPT-5.6-Sol" {
		t.Errorf("Name = %q, want the backend display name", sol.Name)
	}
	if sol.Limit.Context != 500000 {
		t.Errorf("Limit.Context = %d, want max_context_window 500000", sol.Limit.Context)
	}
	if !sol.Attachment {
		t.Error("Attachment = false, want true (image input modality)")
	}
	if p.Models["codex-auto-review"] != nil {
		t.Error("hidden model leaked into the catalog entry")
	}
}

// writeCache drops a models cache in a temp dir and returns its path.
func writeCache(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models_cache.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write models cache: %v", err)
	}
	return path
}

// TestChatGPTModelsFromCLICache covers the offline discovery source: priority
// ordering, hidden-model exclusion, the default pick, and that hidden entries
// still contribute their instructions (an operator may pin one by id).
func TestChatGPTModelsFromCLICache(t *testing.T) {
	set, ok := chatgptModelsFromCLICache(writeCache(t, cliCachePayload))
	if !ok {
		t.Fatal("ok = false for a valid cache")
	}
	if set.Source != chatgptSourceCache || !set.Authoritative() {
		t.Errorf("Source = %q, Authoritative = %v", set.Source, set.Authoritative())
	}
	want := []string{"gpt-5.6-sol", "gpt-5.4"}
	if len(set.IDs) != len(want) {
		t.Fatalf("IDs = %v, want %v (hidden excluded)", set.IDs, want)
	}
	for i, id := range want {
		if set.IDs[i] != id {
			t.Errorf("IDs[%d] = %q, want %q (priority order)", i, set.IDs[i], id)
		}
	}
	if set.Default != "gpt-5.6-sol" {
		t.Errorf("Default = %q, want the highest-priority listed model", set.Default)
	}
	if set.Instructions["gpt-5.6-sol"] != "instr-sol" || set.Instructions["codex-auto-review"] != "instr-review" {
		t.Errorf("Instructions = %v, want per-model prompts incl. hidden models", set.Instructions)
	}
}

// TestChatGPTModelsFromCLICacheBad covers every path that must decline rather
// than return a half-built set.
func TestChatGPTModelsFromCLICacheBad(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"non-json", "not json"},
		{"no models", `{"models":[]}`},
		{"all hidden", `{"models":[{"slug":"x","visibility":"hide"}]}`},
		{"no slug", `{"models":[{"display_name":"nameless","visibility":"list"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := chatgptModelsFromCLICache(writeCache(t, tc.body)); ok {
				t.Error("ok = true, want false")
			}
		})
	}
	if _, ok := chatgptModelsFromCLICache(""); ok {
		t.Error("empty path should decline")
	}
	if _, ok := chatgptModelsFromCLICache(filepath.Join(t.TempDir(), "missing.json")); ok {
		t.Error("missing file should decline")
	}
}

// TestResolveChatGPTModels walks the source ladder: with no tokens and no CLI
// cache the builtin snapshot answers; with a cache present that wins; and an
// authoritative result is memoized.
func TestResolveChatGPTModels(t *testing.T) {
	codexHome(t, "")
	resetChatGPTModelCache(t)
	mgr := chatgptauth.NewManager(t.TempDir())

	set := resolveChatGPTModels(mgr)
	if set.Source != chatgptSourceBuiltin {
		t.Fatalf("Source = %q, want %q", set.Source, chatgptSourceBuiltin)
	}
	if set.Default != chatgptFallbackDefault || len(set.IDs) != len(chatgptFallbackModels) {
		t.Errorf("builtin set = %v / %q", set.IDs, set.Default)
	}
	if set.Authoritative() {
		t.Error("builtin set must not be authoritative")
	}

	// A builtin result is never memoized, so the cache source is picked up next.
	codexHome(t, cliCachePayload)
	set = resolveChatGPTModels(mgr)
	if set.Source != chatgptSourceCache || set.Default != "gpt-5.6-sol" {
		t.Fatalf("Source = %q, Default = %q, want cache/gpt-5.6-sol", set.Source, set.Default)
	}

	// Authoritative results ARE memoized: removing the cache changes nothing.
	codexHome(t, "")
	if again := resolveChatGPTModels(mgr); again.Source != chatgptSourceCache {
		t.Errorf("Source = %q after cache removal, want the memoized %q", again.Source, chatgptSourceCache)
	}
}

// TestSeedChatGPTCatalog covers: nil store (no-op), empty store (seed), and the
// refresh contract — an authoritative set REPLACES a stale entry (the bug: a
// seed-once entry kept serving model ids the backend had dropped), while a
// builtin set leaves an existing entry alone.
func TestSeedChatGPTCatalog(t *testing.T) {
	// Point CODEX_HOME at an empty dir BEFORE anything resolves, so the operator's
	// real Codex cache can't answer discovery and memoize an authoritative set.
	codexHome(t, "")
	resetChatGPTModelCache(t)

	// nil store must not panic.
	SeedChatGPTCatalog(nil, t.TempDir())

	baseDir := t.TempDir()
	store := catalog.NewStore(t.TempDir())

	// First seed adds the chatgpt entry (builtin — nothing better available).
	SeedChatGPTCatalog(store, baseDir)
	cur, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cur == nil || cur.Providers["chatgpt"] == nil {
		t.Fatal("chatgpt provider not seeded on first call")
	}

	// Plant a stale entry, as a pre-fix install would have on disk.
	stale := chatgptCatalogEntry(builtinChatGPTModelSet())
	stale.Models = map[string]*catalog.Model{"gpt-5-codex": {ID: "gpt-5-codex", Name: "gpt-5-codex"}}
	if _, err := store.UpsertCustomProvider(stale); err != nil {
		t.Fatalf("UpsertCustomProvider: %v", err)
	}

	// A builtin set must NOT clobber what is already on disk.
	SeedChatGPTCatalog(store, baseDir)
	cur, _ = store.Load()
	if cur.Providers["chatgpt"].Models["gpt-5-codex"] == nil {
		t.Error("builtin seed overwrote an existing entry")
	}

	// An authoritative set must refresh it, dropping the dead model id.
	resetChatGPTModelCache(t)
	codexHome(t, cliCachePayload)
	SeedChatGPTCatalog(store, baseDir)
	cur, _ = store.Load()
	got := cur.Providers["chatgpt"]
	if got.Models["gpt-5-codex"] != nil {
		t.Error("stale gpt-5-codex survived an authoritative refresh")
	}
	if got.Models["gpt-5.6-sol"] == nil {
		t.Error("discovered gpt-5.6-sol missing after refresh")
	}
}

// TestSyncChatGPTCatalog is the post-sign-in path the control plane calls: it
// reports the served surface and refreshes a stale entry. Because sign-in status
// is polled, a repeat call must not rewrite an entry that already matches.
func TestSyncChatGPTCatalog(t *testing.T) {
	codexHome(t, cliCachePayload)
	resetChatGPTModelCache(t)
	baseDir := t.TempDir()
	dir := t.TempDir()
	store := catalog.NewStore(dir)

	stale := chatgptCatalogEntry(builtinChatGPTModelSet())
	stale.Models = map[string]*catalog.Model{"gpt-5-codex": {ID: "gpt-5-codex", Name: "gpt-5-codex"}}
	if _, err := store.UpsertCustomProvider(stale); err != nil {
		t.Fatalf("UpsertCustomProvider: %v", err)
	}

	models, defaultModel := SyncChatGPTCatalog(store, baseDir)
	if defaultModel != "gpt-5.6-sol" || len(models) != 2 {
		t.Fatalf("surface = %v / %q, want 2 models defaulting to gpt-5.6-sol", models, defaultModel)
	}
	cur, _ := store.Load()
	if cur.Providers["chatgpt"].Models["gpt-5-codex"] != nil {
		t.Error("stale model id survived the sign-in refresh")
	}

	// A matching entry must not be rewritten (status polling calls this).
	path := filepath.Join(dir, "custom.json")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat custom.json: %v", err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime().Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	marker, _ := os.Stat(path)
	if _, _ = SyncChatGPTCatalog(store, baseDir); true {
		after, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat after: %v", err)
		}
		if !after.ModTime().Equal(marker.ModTime()) {
			t.Error("catalog rewritten despite an unchanged model set")
		}
	}

	// A nil store is a no-op that still reports the surface.
	if ids, def := SyncChatGPTCatalog(nil, baseDir); len(ids) != 2 || def != "gpt-5.6-sol" {
		t.Errorf("nil-store surface = %v / %q", ids, def)
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

// TestNewChatGPTProvider checks the discovered per-model prompts are attached to
// the adapter — without them a new model gets the stale vendored prompt.
func TestNewChatGPTProvider(t *testing.T) {
	set, ok := chatgptModelsFromCLICache(writeCache(t, cliCachePayload))
	if !ok {
		t.Fatal("cache parse failed")
	}
	p := newChatGPTProvider(chatgptauth.NewManager(t.TempDir()), set.Default, set)
	if p == nil {
		t.Fatal("newChatGPTProvider returned nil")
	}
	if p.Model != "gpt-5.6-sol" {
		t.Errorf("Model = %q, want gpt-5.6-sol", p.Model)
	}
	if p.Instructions["gpt-5.6-sol"] != "instr-sol" {
		t.Errorf("Instructions not attached: %v", p.Instructions)
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
	if registerChatGPTAlternate(reg, t.TempDir(), "chatgpt", false, nil) {
		t.Error("registerChatGPTAlternate should refuse when chatgpt is primary")
	}

	// Not signed in → refuse.
	if registerChatGPTAlternate(reg, t.TempDir(), "openai", false, nil) {
		t.Error("registerChatGPTAlternate should refuse when not signed in")
	}
	if registerChatGPTAlternate(reg, t.TempDir(), "openai", true, nil) {
		t.Error("registerChatGPTAlternate should refuse when not signed in (replace path)")
	}
}
