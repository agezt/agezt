// SPDX-License-Identifier: MIT

// Package providerboot: unconfiguredProvider stub + UnconfiguredName const.
// This is the Provider the runtime falls back to when the catalog has zero
// eligible entries — every completion attempt fails fast with an actionable
// message telling the operator to add a provider + key + model. Extracted
// from providerboot.go during the Day-211 god-file split. Public API unchanged.
package providerboot


import (
	"context"
	"fmt"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/agent"
)
// UnconfiguredName is the Name() of the sentinel primary registered when no
// LLM provider is configured. The reload path keys off it to swap in a real
// provider once the operator configures one, and the daemon's first-run nudge
// compares Result.Primary against it.
const UnconfiguredName = "unconfigured"

// unconfiguredProvider is the daemon's primary when NO LLM provider is
// configured (AGEZT_PROVIDER unset). The daemon ships with no default provider
// or model (owner rule: "hiçbir default provider/model"), so a fresh install
// boots with this sentinel: the daemon, Web UI, and Setup all run, but any LLM
// call fails fast with an actionable message telling the operator to add a
// provider + key and a model (via AGEZT_MODEL or a routing/fallback chain). It
// is swapped for a real provider by the reload path once one is configured.
type unconfiguredProvider struct{}

func (unconfiguredProvider) Name() string { return UnconfiguredName }
func (unconfiguredProvider) Complete(ctx context.Context, _ agent.CompletionRequest) (*agent.CompletionResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("no LLM provider configured — add a provider and API key (Setup → Providers, or set %sPROVIDER) and a model (%sMODEL, a per-task route, or a fallback chain)", brand.EnvPrefix, brand.EnvPrefix)
}
