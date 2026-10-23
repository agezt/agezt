// SPDX-License-Identifier: MIT

package boot

// The port contract's own tests. These pin what the engine may assume about
// the adapters it asks for — and, just as important, what it does when an
// adapter is missing or misbehaves: the wiring errors are LOUD, because an
// engine that quietly ran without an adapter is the drift class (a provider
// registered unwrapped, a stale model set memoized) this engine exists to
// kill.

import (
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/kernel/modules/providers/api"
	"github.com/agezt/agezt/kernel/modules/providers/catalog"
	"github.com/agezt/agezt/kernel/modules/providers/chatgptauth"
)

// TestEligibleSkipsAnUnsupportedFamily pins the family half of the eligibility
// predicate: a credentialed catalog provider whose family the adapter cannot
// serve is NOT eligible, so it is never registered as an alternate. Registering
// it would put a provider in the routing chain that can only fail at call time.
func TestEligibleSkipsAnUnsupportedFamily(t *testing.T) {
	cat := bootFixtureCatalog(t)
	family := cat.Providers["alpha"].Family()

	// The adapter serves this family → eligible.
	yes := ports(family)
	if !Eligible(yes, cat.Providers["alpha"], keyedLookup) {
		t.Fatal("a credentialed provider of a supported family must be eligible")
	}

	// The adapter cannot serve it → not eligible.
	no := ports(catalog.Family("not-supported"))
	if Eligible(no, cat.Providers["alpha"], keyedLookup) {
		t.Fatal("a provider of an unsupported family must not be eligible")
	}

	// And the registration path honours the same predicate: the alternate is
	// not registered when the family is unsupported.
	reg := governor.NewRegistry()
	p := ports(catalog.Family("not-supported"))
	registerAlternates(reg, Deps{Catalog: cat, Lookup: keyedLookup, BaseDir: t.TempDir(), Ports: p}, "alpha", p.Wrap(nil), false)
	if _, ok := reg.Get("beta"); ok {
		t.Error("beta was registered even though its family is unsupported")
	}
}

// TestAlternatesAreWrappedToo pins that the middleware stack covers the
// ALTERNATES as well as the primary. A stack that reached only the primary
// would make an alternate behave differently from the provider that was chosen
// — the exact drift class the reload parity test exists for.
func TestAlternatesAreWrappedToo(t *testing.T) {
	cat := bootFixtureCatalog(t)
	env := map[string]string{
		brand.EnvPrefix + "PROVIDER":        "alpha",
		brand.EnvPrefix + "MODEL":           "alpha-large",
		brand.EnvPrefix + "GEN_TEMPERATURE": "0.4", // non-empty stack
	}
	p := ports()
	res, err := Boot(Deps{Catalog: cat, Lookup: keyedLookup, BaseDir: t.TempDir(), Get: mapGet(env), Ports: p})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	info, ok := res.Governor.Registry().Get("beta")
	if !ok {
		t.Fatal("beta not registered as an alternate")
	}
	if _, raw := info.Provider.(*fakeProvider); raw {
		t.Error("the alternate was registered unwrapped — the middleware stack reached only the primary")
	}
	if p.wrapped < 2 {
		t.Errorf("wrapped = %d, want the primary AND the alternate to go through the stack", p.wrapped)
	}
}

// TestChatGPTCatalogEntryUsesTheAdapterBaseURL pins that the catalog entry
// records the base URL the ADAPTER serves — the module does not hardcode one,
// and it must not record an empty one (the entry would point nowhere).
func TestChatGPTCatalogEntryUsesTheAdapterBaseURL(t *testing.T) {
	p := ports()
	entry := chatgptCatalogEntry(p, builtinChatGPTModelSet())
	if entry.API != p.ChatGPTBaseURL() {
		t.Errorf("API = %q, want the adapter's base URL %q", entry.API, p.ChatGPTBaseURL())
	}
	if entry.API == "" {
		t.Error("API = empty — the catalog entry would point nowhere")
	}
}

// nilWrapperPorts is a mis-wired adapter: the middleware stack comes back nil.
type nilWrapperPorts struct {
	fakePorts
}

func (nilWrapperPorts) Wrap(func(string) string) func(llm.Provider) llm.Provider { return nil }

// TestNilWrapperIsLoud pins that a mis-wired adapter is reported, not
// dereferenced: a nil wrapper would take the daemon down with a panic on the
// boot path.
func TestNilWrapperIsLoud(t *testing.T) {
	d := Deps{
		Catalog: catalog.NewEmpty(),
		Lookup:  func(string) string { return "" },
		BaseDir: t.TempDir(),
		Ports:   &nilWrapperPorts{},
	}
	if _, err := Boot(d); err == nil {
		t.Error("Boot with a nil middleware wrapper must fail loudly")
	}
}

// TestSyncChatGPTCatalogDropsTheWeakerMemo pins the sign-in path: signing in
// unlocks the BACKEND source, so a memo that came from the weaker Codex CLI
// cache must be dropped — otherwise the surface the operator is shown keeps
// describing the offline cache instead of what the backend serves.
func TestSyncChatGPTCatalogDropsTheWeakerMemo(t *testing.T) {
	codexHome(t, cliCachePayload)
	resetChatGPTModelCache(t)
	baseDir := t.TempDir()

	// The operator is signed in (the vault holds a token set) and the backend
	// answers with its own surface.
	mgr := chatgptauth.NewManager(baseDir)
	if err := mgr.StoreTokens(chatgptauth.Tokens{
		AccessToken: "access-token", RefreshToken: "refresh-token",
	}); err != nil {
		t.Fatalf("StoreTokens: %v", err)
	}

	p := ports()
	store := catalog.NewStore(t.TempDir())

	// First the backend is unreachable: the Codex CLI cache answers and its
	// answer is MEMOIZED (it is authoritative, so it is kept for the TTL).
	models, def := SyncChatGPTCatalog(store, baseDir, p)
	if len(models) != 2 || def != "gpt-5.6-sol" {
		t.Fatalf("surface = %v / %q, want the cache's surface", models, def)
	}
	chatgptModelCache.mu.Lock()
	memo := chatgptModelCache.set.Source
	chatgptModelCache.mu.Unlock()
	if memo != chatgptSourceCache {
		t.Fatalf("memoized source = %q, want %q", memo, chatgptSourceCache)
	}

	// The operator signs in: the BACKEND source unlocks and serves its own
	// surface. The weaker cache memo must be dropped, or the operator keeps
	// being shown the offline cache's models.
	p.models = []api.ModelInfo{{Slug: "gpt-backend", DisplayName: "Backend", Visibility: "list", Priority: 1}}
	models, def = SyncChatGPTCatalog(store, baseDir, p)
	if len(models) != 1 || models[0] != "gpt-backend" || def != "gpt-backend" {
		t.Fatalf("surface = %v / %q, want the backend's own model after sign-in", models, def)
	}
}
