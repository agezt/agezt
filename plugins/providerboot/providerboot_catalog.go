// SPDX-License-Identifier: MIT

package providerboot

// Catalog-driven provider construction: catalogModelIDs +
// demoEchoProvider + BuildFromCatalog. Carved out of
// providerboot_runtime.go during the Day 189 god-file split so the
// main file can stay focused on Boot/Reload and the register file
// can stay focused on registration.
// Public API unchanged.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/governor"
	"github.com/agezt/agezt/plugins/providers/compat"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func catalogModelIDs(cat *catalog.Catalog, providerID string) []string {
	if cat == nil {
		return nil
	}
	entry, ok := cat.Providers[providerID]
	if !ok || len(entry.Models) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entry.Models))
	for m := range entry.Models {
		ids = append(ids, m)
	}
	sort.Strings(ids)
	return ids
}

// SelectPrimary returns the primary provider, a banner description,
// the resolved run model id (may be ""), the auth-mode tag for the
// Governor's registry, and an error.
//
// Selection:
//
//  1. AGEZT_PROVIDER=<catalog id> → look up in cat; compat.Build it. The ONLY
//     way to select a real primary; an unknown id is a hard error.
//  2. AGEZT_PROVIDER unset and AGEZT_DEMO_ECHO=1 → explicit offline e2e/demo
//     mock provider.
//  3. AGEZT_PROVIDER unset        → the "unconfigured" sentinel primary. No
//     auto-pick, no silent mock. The daemon boots so Setup/routing can be
//     configured, but LLM runs fail fast with an actionable error.
//
// The run model comes from AGEZT_MODEL when set; otherwise it is left empty and
// resolved per-run from routing / a fallback chain (or ErrNoModelConfigured).
func demoEchoProvider() *mock.Provider {
	p := mock.New()
	p.Responder = func(req agent.CompletionRequest) agent.CompletionResponse {
		text := ""
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == agent.RoleUser {
				text = strings.TrimSpace(req.Messages[i].Content)
				if text != "" {
					break
				}
			}
		}
		if text == "" {
			text = "ok"
		}
		return mock.FinalText("[echo] " + text)
	}
	return p
}

// BuildFromCatalog finalises a catalog entry into a wire Provider.
// Shared by both the explicit-id path and the alternate-registration path.
// Credentials resolve through d.Lookup (the chained vault+env resolver).
func BuildFromCatalog(d Deps, entry *catalog.Provider, modelOverride string) (agent.Provider, string, string, governor.AuthMode, error) {
	lookup := d.Lookup
	// The daemon has NO default run model. AGEZT_MODEL, when set, is the model
	// every run uses unless per-task routing or a fallback chain overrides it.
	// When AGEZT_MODEL is empty the returned run model stays "" — so cfg.Model is
	// empty and the governor refuses any run that doesn't resolve a model via
	// routing/chain (ErrNoModelConfigured), per the owner's no-default rule.
	//
	// compat.Build still needs *a* concrete, catalog-valid model id to construct
	// the provider wire, so when AGEZT_MODEL is empty we fall back to the first
	// catalog model as an INERT construction placeholder. It is never surfaced as
	// a run default (cfg.Model stays "") and is never reached at call time (the
	// governor guard + per-provider model-required errors fire first).
	runModel := modelOverride
	constructModel := modelOverride
	if constructModel == "" {
		constructModel = compat.FirstModelID(entry)
	}
	if constructModel == "" {
		return nil, "", "", "", fmt.Errorf("provider %q in catalog has no models; set %sMODEL", entry.ID, brand.EnvPrefix)
	}
	// Auto-repair a cross-provider default model (don't hard-fail the boot):
	// AGEZT_MODEL may name a model this provider's catalog doesn't serve because
	// it is resolved per-run through routing / a fallback chain on a DIFFERENT
	// provider (e.g. AGEZT_PROVIDER=minimax-coding-plan + AGEZT_MODEL=gpt-5.4,
	// where gpt-5.4 rides @new-chain). compat.Build only needs a concrete,
	// catalog-valid id to construct the wire, so fall back to the inert
	// placeholder for CONSTRUCTION while keeping runModel as the override — the
	// governor still resolves the real model per run (or fails that one run with
	// an actionable error), instead of the whole daemon refusing to start.
	if modelOverride != "" {
		if _, ok := entry.Models[modelOverride]; !ok {
			placeholder := compat.FirstModelID(entry)
			fmt.Fprintf(d.stderr(),
				"%s: %sMODEL %q is not in provider %q's catalog — treating it as a routing/fallback-chain model and constructing %q with placeholder %q (set %sMODEL to one of this provider's models to silence)\n",
				brand.Binary, brand.EnvPrefix, modelOverride, entry.ID, entry.ID, placeholder, brand.EnvPrefix)
			constructModel = placeholder
		}
	}
	prov, _, err := compat.Build(entry, constructModel, lookup)
	if err != nil {
		return nil, "", "", "", err
	}
	auth := governor.AuthAPIKey
	if len(entry.Env) == 0 {
		auth = governor.AuthLocal
	}
	modelDesc := runModel
	if modelDesc == "" {
		modelDesc = "(unset — resolved from routing/fallback chain per run)"
	}
	desc := fmt.Sprintf("%s(catalog; family=%s, model=%s)", entry.ID, entry.Family(), modelDesc)
	return prov, desc, runModel, auth, nil
}

