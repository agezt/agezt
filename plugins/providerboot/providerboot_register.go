// SPDX-License-Identifier: MIT

package providerboot

// Provider registration: registerAlternates + SelectPrimary.
// Carved out of providerboot_runtime.go during the Day 189 god-file
// split so the main file can stay focused on the Boot/Reload
// lifecycle and the catalog file can stay focused on catalog-driven
// provider construction.
// Public API unchanged.

import (
	"fmt"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/governor"
)

func registerAlternates(reg *governor.Registry, d Deps, primaryName string, mw []agent.Middleware, replace bool) map[string]bool {
	eligible := map[string]bool{primaryName: true}
	for _, entry := range d.Catalog.ProviderList() {
		if entry.ID == primaryName {
			continue // already the primary
		}
		if !Eligible(entry, d.Lookup) {
			continue
		}
		p, _, _, auth, err := BuildFromCatalog(d, entry, "")
		if err != nil {
			continue
		}
		info := &governor.ProviderInfo{
			Name:     p.Name(),
			Provider: agent.Wrap(p, mw...),
			AuthMode: auth,
			Models:   catalogModelIDs(d.Catalog, entry.ID),
		}
		var rerr error
		if replace {
			rerr = reg.Replace(info)
		} else {
			rerr = reg.Register(info)
		}
		if rerr != nil {
			continue // duplicate name or similar — skip gracefully
		}
		eligible[entry.ID] = true
	}
	// ChatGPT ("Sign in with ChatGPT") registers as a subscription alternate
	// when signed in (and not already the primary) — its models route to the
	// Responses backend adapter.
	if registerChatGPTAlternate(reg, d.BaseDir, primaryName, replace, mw) {
		eligible["chatgpt"] = true
	}
	if replace {
		// Stale-drop sweep (reload only): alternates that lost eligibility are
		// removed; fallback entries are never touched.
		for _, info := range reg.All() {
			if info.IsFallback || eligible[info.Name] {
				continue
			}
			reg.Remove(info.Name)
		}
	}
	return eligible
}

// Boot constructs the routing layer: one primary provider plus every other
// credentialed catalog provider as a model-routable alternate.
//
// The daemon has NO default provider, NO credential auto-pick, NO silent offline
// mock fallback, and NO default model (owner rule: "hiçbir default
// provider/model"). The only mock path is the explicit AGEZT_DEMO_ECHO=1 e2e /
// demo escape hatch.
//
// **Provider selection (catalog-driven):**
//
//	$AGEZT_PROVIDER=<catalog-id>    → e.g. "anthropic", "ollama-local",
//	                                  "groq", "openai" — any provider in the
//	                                  synced catalog. The ONLY way to select a
//	                                  primary. An unknown id is a hard error.
//	(unset)                          → UNCONFIGURED: a sentinel primary that
//	                                  fails every LLM call with an actionable
//	                                  "configure a provider" error. The daemon,
//	                                  Web UI, and Setup still run.
//	$AGEZT_DEMO_ECHO=1               → explicit offline demo/e2e mock provider
//	                                  when $AGEZT_PROVIDER is unset.
//	$AGEZT_MODEL=<model-id>         → the run model. If unset, runs resolve their
//	                                  model from per-task routing or a fallback
//	                                  chain; with neither, the governor returns
//	                                  ErrNoModelConfigured.
func SelectPrimary(d Deps) (agent.Provider, string, string, governor.AuthMode, error) {
	cat := d.Catalog
	// AGEZT_PROVIDER and AGEZT_MODEL are *config*, not credentials —
	// always read from the config env directly (operators may want a one-off
	// override that doesn't sit in the vault).
	want := strings.ToLower(strings.TrimSpace(d.get(brand.EnvPrefix + "PROVIDER")))
	modelOverride := strings.TrimSpace(d.get(brand.EnvPrefix + "MODEL"))

	// ChatGPT ("Sign in with ChatGPT") is not a compat catalog provider — it uses
	// the OAuth token store + Responses adapter, so build it directly.
	if want == "chatgpt" {
		prov, desc, auth, ok := buildChatGPTPrimary(d.BaseDir, modelOverride)
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=chatgpt but not signed in — use Setup → Providers → Sign in with ChatGPT first", brand.EnvPrefix)
		}
		return prov, desc, modelOverride, auth, nil
	}

	// Explicit catalog id is the ONLY way to select a primary. The daemon has no
	// default provider, never auto-picks from credentials, and has no offline mock
	// fallback (owner rule: "hiçbir default provider/model"). An unknown id is a
	// hard error so a typo is loud, not silently degraded.
	if want != "" {
		entry, ok := cat.Providers[want]
		if !ok {
			return nil, "", "", "", fmt.Errorf(
				"%sPROVIDER=%q not in catalog; run `agt catalog sync` then `agt catalog list`",
				brand.EnvPrefix, want)
		}
		return BuildFromCatalog(d, entry, modelOverride)
	}

	if strings.TrimSpace(d.get(brand.EnvPrefix+"DEMO_ECHO")) == "1" {
		model := modelOverride
		if model == "" {
			model = "mock"
		}
		return demoEchoProvider(),
			"demo echo mock (explicit " + brand.EnvPrefix + "DEMO_ECHO=1; offline e2e/demo)",
			model, governor.AuthLocal, nil
	}

	// AGEZT_PROVIDER unset → boot UNCONFIGURED. The daemon, Web UI, and Setup all
	// run so the operator can add a provider + key and configure routing/chains,
	// but any LLM call fails fast with an actionable error (unconfiguredProvider).
	// No credential auto-pick, no mock.
	return unconfiguredProvider{},
		"unconfigured (no " + brand.EnvPrefix + "PROVIDER set — add a provider + key in Setup → Providers; LLM runs fail until then)",
		"", governor.AuthLocal, nil
}

