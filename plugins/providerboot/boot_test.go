// SPDX-License-Identifier: MIT

package providerboot

// Boot/Reload tests. The first four moved here from cmd/agezt with the Phase
// 2.4 extraction (adapted to the Deps-based API; t.Setenv still works through
// the default Get = os.Getenv). The rest are NEW and pin the extraction's
// contract: Boot and Reload share ONE registration path, so a reloaded daemon
// matches a freshly-booted one for the same on-disk state (the parity test
// M928 + M816 never had), middleware survives Reload, the cross-provider
// down-route eligibility set is live, the unconfigured sentinel is promoted
// away, and alternates land in the registry BEFORE gov.Replace rebuilds the
// routing chains.

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// bootFixtureAPI is a 4-provider catalog: alpha/beta/gamma keyed via
// keyedLookup, "unkeyed" never credentialed.
const bootFixtureAPI = `{
  "alpha": {
    "id": "alpha", "name": "Alpha", "env": ["ALPHA_API_KEY"],
    "npm": "@ai-sdk/openai-compatible", "api": "https://api.alpha.invalid/v1",
    "models": {
      "alpha-large": {
        "id": "alpha-large", "name": "alpha-large", "family": "alpha",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  },
  "beta": {
    "id": "beta", "name": "Beta", "env": ["BETA_API_KEY"],
    "npm": "@ai-sdk/openai-compatible", "api": "https://api.beta.invalid/v1",
    "models": {
      "beta-mini": {
        "id": "beta-mini", "name": "beta-mini", "family": "beta",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  },
  "gamma": {
    "id": "gamma", "name": "Gamma", "env": ["GAMMA_API_KEY"],
    "npm": "@ai-sdk/openai-compatible", "api": "https://api.gamma.invalid/v1",
    "models": {
      "gamma-pro": {
        "id": "gamma-pro", "name": "gamma-pro", "family": "gamma",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  },
  "unkeyed": {
    "id": "unkeyed", "name": "Unkeyed", "env": ["UNKEYED_API_KEY"],
    "npm": "@ai-sdk/openai-compatible", "api": "https://api.unkeyed.invalid/v1",
    "models": {
      "unkeyed-1": {
        "id": "unkeyed-1", "name": "unkeyed-1", "family": "unkeyed",
        "tool_call": true,
        "modalities": {"input":["text"], "output":["text"]},
        "limit": {"context": 32768, "output": 4096},
        "cost": {"input": 1, "output": 2}
      }
    }
  }
}`

func bootFixtureCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.ParseAPIFile([]byte(bootFixtureAPI))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return cat
}

func keyedLookup(name string) string {
	switch name {
	case "ALPHA_API_KEY", "BETA_API_KEY", "GAMMA_API_KEY":
		return "test-key"
	}
	return ""
}

// mapGet returns a Deps.Get backed by a plain map — a fully isolated config
// environment (unset names read "", exactly like os.Getenv on an unset var).
func mapGet(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// provSnap is one registry entry's identity for boot-vs-reload comparison.
// ProviderType captures middleware wrapping: an unwrapped provider is the
// concrete adapter type, a wrapped one is the agent middleware wrapper — so
// snapshot equality fails if one path wraps and the other doesn't.
type provSnap struct {
	Name         string
	Auth         governor.AuthMode
	Fallback     bool
	Models       []string
	ProviderType string
}

// snapshotGovernor captures the registry as a name-sorted []provSnap.
func snapshotGovernor(g *governor.Governor) []provSnap {
	infos := g.Registry().All()
	snaps := make([]provSnap, 0, len(infos))
	for _, info := range infos {
		snaps = append(snaps, provSnap{
			Name:         info.Name,
			Auth:         info.AuthMode,
			Fallback:     info.IsFallback,
			Models:       append([]string(nil), info.Models...),
			ProviderType: fmt.Sprintf("%T", info.Provider),
		})
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Name < snaps[j].Name })
	return snaps
}

// ----- tests moved from cmd/agezt (Phase 2.4) -----

// TestBuildFromCatalog_CrossProviderModelDoesNotFailBoot: AGEZT_MODEL may name a
// model the chosen provider's catalog doesn't serve because it is resolved per-run
// through a fallback chain on a DIFFERENT provider (e.g. provider=minimax-coding-plan
// + model=gpt-5.4 via @new-chain). BuildFromCatalog must auto-repair this — construct
// the wire with an inert catalog-valid placeholder while preserving the override as
// the run model — instead of hard-failing the daemon boot (the user-reported
// "compat: ... has no model" startup crash).
func TestBuildFromCatalog_CrossProviderModelDoesNotFailBoot(t *testing.T) {
	entry := &catalog.Provider{
		ID: "minimax-coding-plan", NPM: "@ai-sdk/openai-compatible",
		API: "http://localhost:9/v1", // local-family (no Env) → no creds needed to construct
		Models: map[string]*catalog.Model{
			"minimax-m2": {ID: "minimax-m2", ToolCall: true, Limit: catalog.Limit{Context: 200000}},
		},
	}
	d := Deps{Lookup: func(string) string { return "" }}

	// Model NOT in this provider's catalog → must NOT error; run model preserved
	// for routing, provider still constructed.
	prov, _, runModel, _, err := BuildFromCatalog(d, entry, "gpt-5.4")
	if err != nil {
		t.Fatalf("cross-provider model must not fail boot, got error: %v", err)
	}
	if prov == nil {
		t.Fatal("provider should still be constructed with the placeholder model")
	}
	if runModel != "gpt-5.4" {
		t.Errorf("run model should stay the override %q for routing, got %q", "gpt-5.4", runModel)
	}

	// A catalog-valid override still works unchanged.
	if _, _, rm, _, err := BuildFromCatalog(d, entry, "minimax-m2"); err != nil || rm != "minimax-m2" {
		t.Errorf("valid model: got (%q, %v), want (minimax-m2, nil)", rm, err)
	}

	// A provider with no models at all is still a hard error (nothing to construct).
	empty := &catalog.Provider{ID: "empty", NPM: "@ai-sdk/openai-compatible", API: "http://localhost:9/v1"}
	if _, _, _, _, err := BuildFromCatalog(d, empty, "gpt-5.4"); err == nil {
		t.Error("a provider with zero catalog models should still error")
	}
}

// TestBoot_UnconfiguredWhenNoProvider: with no AGEZT_PROVIDER and no
// credentialed catalog, the daemon must boot the "unconfigured" sentinel — NOT a
// mock and NOT an auto-picked provider — and must surface no default run model.
// A run then fails fast with the actionable "no LLM provider configured" error
// rather than returning a silent mock answer. This pins the owner's
// "hiçbir default provider/model" rule at the boot layer.
func TestBoot_UnconfiguredWhenNoProvider(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"PROVIDER", "")
	t.Setenv(brand.EnvPrefix+"MODEL", "")
	t.Setenv(brand.EnvPrefix+"DEMO_ECHO", "")

	res, err := Boot(Deps{Catalog: catalog.NewEmpty(), Lookup: func(string) string { return "" }, BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if res.Model != "" {
		t.Errorf("run model = %q, want empty (no built-in default model)", res.Model)
	}
	if !strings.Contains(res.Desc, "unconfigured") {
		t.Errorf("banner desc = %q, want it to mention unconfigured", res.Desc)
	}
	if res.Primary != UnconfiguredName {
		t.Errorf("Primary = %q, want the %q sentinel", res.Primary, UnconfiguredName)
	}

	// A run with a model still fails — there is no provider behind it, and no mock
	// fallback to silently answer.
	_, rerr := res.Governor.Complete(context.Background(), agent.CompletionRequest{
		Model:    "anything",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hi"}},
	})
	if rerr == nil {
		t.Fatal("unconfigured daemon answered a run; want a hard error")
	}
	if !strings.Contains(rerr.Error(), "no LLM provider configured") {
		t.Errorf("err = %v, want it to mention 'no LLM provider configured'", rerr)
	}
}

func TestBoot_DemoEchoRequiresExplicitEnv(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"PROVIDER", "")
	t.Setenv(brand.EnvPrefix+"MODEL", "")
	t.Setenv(brand.EnvPrefix+"DEMO_ECHO", "1")

	res, err := Boot(Deps{Catalog: catalog.NewEmpty(), Lookup: func(string) string { return "" }, BaseDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if res.Model != "mock" {
		t.Errorf("run model = %q, want mock", res.Model)
	}
	if !strings.Contains(res.Desc, "demo echo") {
		t.Errorf("banner desc = %q, want it to mention demo echo", res.Desc)
	}

	resp, err := res.Governor.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mock",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "hello e2e"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := resp.Message.Content; got != "[echo] hello e2e" {
		t.Errorf("response = %q, want %q", got, "[echo] hello e2e")
	}
}

// TestSelectPrimary_UnknownProviderIsHardError: an explicit but unknown
// AGEZT_PROVIDER is a loud error, never a silent degrade to mock.
func TestSelectPrimary_UnknownProviderIsHardError(t *testing.T) {
	t.Setenv(brand.EnvPrefix+"PROVIDER", "does-not-exist")
	t.Setenv(brand.EnvPrefix+"MODEL", "")
	t.Setenv(brand.EnvPrefix+"DEMO_ECHO", "1")
	if _, _, _, _, err := SelectPrimary(Deps{Catalog: catalog.NewEmpty(), Lookup: func(string) string { return "" }, BaseDir: t.TempDir()}); err == nil {
		t.Fatal("unknown provider id should be a hard error")
	}
}

// ----- new tests (Phase 2.4 contract) -----

// TestBootReloadParity is THE test M928 + M816 never had: for the same on-disk
// state (3 keyed + 1 unkeyed catalog providers, middleware opted in), a fresh
// Boot with AGEZT_PROVIDER=alpha and an unconfigured Boot followed by a Reload
// that selects alpha must produce IDENTICAL registries — same names, models,
// auth modes, and middleware wrapping — and the same resolved run model.
func TestBootReloadParity(t *testing.T) {
	cat := bootFixtureCatalog(t)
	base := t.TempDir()
	configured := map[string]string{
		brand.EnvPrefix + "PROVIDER":        "alpha",
		brand.EnvPrefix + "MODEL":           "alpha-large",
		brand.EnvPrefix + "GEN_TEMPERATURE": "0.4", // non-empty middleware stack
	}
	dConfigured := Deps{Catalog: cat, Lookup: keyedLookup, BaseDir: base, Get: mapGet(configured)}

	// Path 1: straight boot into the configured state.
	direct, err := Boot(dConfigured)
	if err != nil {
		t.Fatalf("Boot (configured): %v", err)
	}

	// Path 2: boot unconfigured (same catalog + keys, no provider selected),
	// then hot-reload into the same configured state.
	unconfigured := map[string]string{
		brand.EnvPrefix + "GEN_TEMPERATURE": "0.4",
	}
	viaReload, err := Boot(Deps{Catalog: cat, Lookup: keyedLookup, BaseDir: base, Get: mapGet(unconfigured)})
	if err != nil {
		t.Fatalf("Boot (unconfigured): %v", err)
	}
	model, err := Reload(viaReload.Governor, dConfigured)
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if model != direct.Model {
		t.Errorf("reload model = %q, boot model = %q — want equal", model, direct.Model)
	}
	got := snapshotGovernor(viaReload.Governor)
	want := snapshotGovernor(direct.Governor)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("boot vs reload registry drift:\n boot:   %+v\n reload: %+v", want, got)
	}
}

// TestMiddlewareSurvivesReload pins drift fix #1: the reload path must wrap
// the primary (and alternates) in the M997 middleware stack exactly like Boot
// does. Before the fix, Reload registered RAW providers, so GEN_TEMPERATURE /
// EXTRACT_REASONING / SIMULATE_STREAMING silently stopped applying after any
// provider reload until daemon restart.
func TestMiddlewareSurvivesReload(t *testing.T) {
	env := map[string]string{
		brand.EnvPrefix + "DEMO_ECHO":       "1",
		brand.EnvPrefix + "GEN_TEMPERATURE": "0.7",
	}
	d := Deps{Catalog: catalog.NewEmpty(), Lookup: func(string) string { return "" }, BaseDir: t.TempDir(), Get: mapGet(env)}
	res, err := Boot(d)
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	info, ok := res.Governor.Registry().Get("mock")
	if !ok {
		t.Fatal("demo-echo primary not registered")
	}
	if _, raw := info.Provider.(*mock.Provider); raw {
		t.Fatal("boot: primary registered unwrapped despite a non-empty middleware stack")
	}

	if _, err := Reload(res.Governor, d); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	info, ok = res.Governor.Registry().Get("mock")
	if !ok {
		t.Fatal("primary missing after reload")
	}
	if _, raw := info.Provider.(*mock.Provider); raw {
		t.Error("reload registered the RAW provider — middleware dropped on the reload path")
	}
}

// TestEligibleUpdatesAcrossReload pins drift fix #2: the cross-provider
// down-route eligibility set the governor's altFinder reads is LIVE — Reload
// refreshes it when providers gain/lose credentials. Before the fix, the
// closure held the map built once at boot, so a reload mutated the registry
// but down-routing kept the boot-time view.
func TestEligibleUpdatesAcrossReload(t *testing.T) {
	cat := bootFixtureCatalog(t)
	base := t.TempDir()
	env := map[string]string{
		brand.EnvPrefix + "PROVIDER":              "alpha",
		brand.EnvPrefix + "MODEL_DOWNROUTE_CROSS": "on", // the altFinder closes over the live set
	}
	alphaBeta := func(name string) string {
		if name == "ALPHA_API_KEY" || name == "BETA_API_KEY" {
			return "test-key"
		}
		return ""
	}
	alphaGamma := func(name string) string {
		if name == "ALPHA_API_KEY" || name == "GAMMA_API_KEY" {
			return "test-key"
		}
		return ""
	}

	res, err := Boot(Deps{Catalog: cat, Lookup: alphaBeta, BaseDir: base, Get: mapGet(env)})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if !res.Eligible("beta") || res.Eligible("gamma") {
		t.Fatalf("boot eligibility wrong: beta=%v gamma=%v, want true/false",
			res.Eligible("beta"), res.Eligible("gamma"))
	}

	// Rotate: beta's key revoked, gamma's added → Reload must refresh the set.
	if _, err := Reload(res.Governor, Deps{Catalog: cat, Lookup: alphaGamma, BaseDir: base, Get: mapGet(env)}); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if res.Eligible("beta") {
		t.Error("beta still eligible after its key was revoked — the eligibility set is frozen at boot")
	}
	if !res.Eligible("gamma") {
		t.Error("gamma not eligible after gaining a key — the eligibility set is frozen at boot")
	}
	// The registry itself must agree (stale-drop sweep + re-registration).
	if _, ok := res.Governor.Registry().Get("beta"); ok {
		t.Error("beta still registered after losing eligibility")
	}
	if _, ok := res.Governor.Registry().Get("gamma"); !ok {
		t.Error("gamma not registered after gaining eligibility")
	}
}

// TestReload_SentinelPromotedToPrimary pins the M816 fix end-to-end: a daemon
// booted UNCONFIGURED must, after the operator configures a provider and
// reloads, route runs to the real provider — the sentinel is removed, not left
// at primary[0] refusing every run.
func TestReload_SentinelPromotedToPrimary(t *testing.T) {
	base := t.TempDir()
	res, err := Boot(Deps{Catalog: catalog.NewEmpty(), Lookup: func(string) string { return "" }, BaseDir: base, Get: mapGet(map[string]string{})})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if res.Primary != UnconfiguredName {
		t.Fatalf("Primary = %q, want %q", res.Primary, UnconfiguredName)
	}

	// First-run wizard equivalent: the operator configures a provider, then
	// reloads (demo echo keeps the test offline).
	env := map[string]string{brand.EnvPrefix + "DEMO_ECHO": "1"}
	model, err := Reload(res.Governor, Deps{Catalog: catalog.NewEmpty(), Lookup: func(string) string { return "" }, BaseDir: base, Get: mapGet(env)})
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if model != "mock" {
		t.Errorf("reload model = %q, want mock", model)
	}
	if _, ok := res.Governor.Registry().Get(UnconfiguredName); ok {
		t.Error("unconfigured sentinel still registered after reload configured a provider")
	}
	provs := res.Governor.Providers()
	if len(provs) == 0 || provs[0].Name != "mock" {
		t.Fatalf("routing chain = %v, want the new primary at position 0", provs)
	}
	// The M816 symptom was runs STILL failing with "no provider configured".
	resp, err := res.Governor.Complete(context.Background(), agent.CompletionRequest{
		Model:    "mock",
		Messages: []agent.Message{{Role: agent.RoleUser, Content: "post-reload"}},
	})
	if err != nil {
		t.Fatalf("Complete after reload: %v", err)
	}
	if got := resp.Message.Content; got != "[echo] post-reload" {
		t.Errorf("response = %q, want %q", got, "[echo] post-reload")
	}
}

// TestReload_AlternatesRegisteredBeforeReplace pins the load-bearing ordering:
// registry mutations (alternate reconciliation) happen BEFORE gov.Replace,
// because Replace rebuilds the governor's CACHED routing chains from the
// registry. Had Reload installed the primary first, freshly-keyed alternates
// would sit in the registry but be invisible to routing until the next reload.
func TestReload_AlternatesRegisteredBeforeReplace(t *testing.T) {
	cat := bootFixtureCatalog(t)
	base := t.TempDir()

	// Boot with the catalog present but nothing keyed → no alternates.
	res, err := Boot(Deps{Catalog: cat, Lookup: func(string) string { return "" }, BaseDir: base, Get: mapGet(map[string]string{})})
	if err != nil {
		t.Fatalf("Boot: %v", err)
	}

	// Keys arrive + a primary is selected; one reload must make BOTH the new
	// primary and the other keyed providers routable.
	env := map[string]string{brand.EnvPrefix + "PROVIDER": "alpha"}
	if _, err := Reload(res.Governor, Deps{Catalog: cat, Lookup: keyedLookup, BaseDir: base, Get: mapGet(env)}); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	// Providers() reads the governor's cached chain (rebuilt by Replace), NOT
	// the registry — so beta/gamma showing up here proves they were registered
	// before Replace rebuilt the chain.
	names := map[string]bool{}
	for _, p := range res.Governor.Providers() {
		names[p.Name] = true
	}
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !names[want] {
			t.Errorf("routing chain missing %q after reload — alternates must be registered before gov.Replace (chain: %v)", want, names)
		}
	}
	if names[UnconfiguredName] {
		t.Errorf("routing chain still contains the %q sentinel", UnconfiguredName)
	}
}
