// SPDX-License-Identifier: MIT

// Package catalog owns transport-independent catalog sync, listing and discovery.
// Typed operation specs retain the existing wire, persistence and reload behavior.
package catalog

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	providercatalog "github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

type SyncInput struct {
	URL            string  `json:"url,omitempty"`
	TimeoutSeconds float64 `json:"timeout_s,omitempty"`
}
type ListInput struct{}
type DiscoverInput struct {
	Endpoint string `json:"endpoint,omitempty"`
}

type Service struct {
	k       *runtime.Kernel
	baseDir string
}

func New(k *runtime.Kernel, baseDir string) *Service { return &Service{k: k, baseDir: baseDir} }

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func (s *Service) Sync(ctx context.Context, in SyncInput) (SyncOutput, error) {
	url := in.URL
	if url == "" {
		url = envOrDefault(brand.EnvPrefix+"CATALOG_URL", providercatalog.DefaultSyncURL)
	}
	syncer := providercatalog.NewSyncer()
	syncer.URL = url
	if t := in.TimeoutSeconds; t > 0 {
		syncer.Timeout = time.Duration(t) * time.Second
	}

	raw, cat, res, err := syncer.Sync(ctx)
	if err != nil {
		s.k.Bus().Publish(event.Spec{
			Subject:       "catalog.sync",
			Kind:          event.KindCatalogSyncFailed,
			Actor:         "catalog",
			CorrelationID: opapi.CorrelationFromContext(ctx),
			Payload:       map[string]any{"url": url, "error": err.Error()},
		})
		return SyncOutput{}, err
	}

	if err := s.k.CatalogStore().SaveAPI(raw, url); err != nil {
		return SyncOutput{}, fmt.Errorf("save: %w", err)
	}
	// FULL reload — catalog snapshot AND provider re-selection (M928). A daemon
	// that booted catalog-less degrades to the offline mock primary; a bare
	// ReloadCatalog here used to leave that mock serving every run even though
	// the fresh catalog + existing vault keys now make real providers eligible
	// (the first-run "sync from the UI, chat still answers [offline-mock]" trap).
	// On a provider-rebuild failure the catalog snapshot has already installed
	// (Reload loads it first), so surface the error in the result instead of
	// failing the sync the operator asked for.
	freshCat, providersReloaded, provErr := s.k.Reload()
	if freshCat == nil {
		return SyncOutput{}, fmt.Errorf("reload: %w", provErr)
	}
	_, _ = s.k.Bus().Publish(event.Spec{
		Subject:       "catalog.sync",
		Kind:          event.KindCatalogSynced,
		Actor:         "catalog",
		CorrelationID: opapi.CorrelationFromContext(ctx),
		Payload: map[string]any{
			"url":            url,
			"bytes":          res.Bytes,
			"provider_count": res.ProviderCount,
			"model_count":    res.ModelCount,
			"duration_ms":    res.Duration.Milliseconds(),
		},
	})
	_ = cat // already installed via Reload
	result := SyncOutput{URL: url, Bytes: res.Bytes, ProviderCount: res.ProviderCount, ModelCount: res.ModelCount, DurationMS: res.Duration.Milliseconds(), ProvidersReloaded: providersReloaded}
	if provErr != nil {
		result.ProviderReloadError = provErr.Error()
	}
	return result, nil
}

func (s *Service) List(_ context.Context, _ ListInput) (ListOutput, error) {
	cat := s.k.Catalog()
	// A provider is "credentialed" if a key exists for it in the process env OR the
	// vault — provider keys (incl. the M700 keyring) live in the vault, so checking
	// os.Getenv alone would miss them and mark keyed providers as un-keyed.
	vault := creds.NewStore(s.baseDir)
	_ = vault.Load()
	duplicateEnv := cat.DuplicateCredentialEnvs()
	credLookup := func(name string) string {
		name = strings.TrimSpace(name)
		if providercatalog.IsProviderCredentialName(name) {
			return vault.Get(name)
		}
		if v := os.Getenv(name); v != "" {
			return v
		}
		if duplicateEnv[name] {
			return ""
		}
		return vault.Get(name)
	}
	providers := make([]ProviderOutput, 0, len(cat.Providers))
	for _, p := range cat.ProviderList() {
		models := make([]ModelOutput, 0, len(p.Models))
		// Deterministic order for stable CLI output.
		ids := make([]string, 0, len(p.Models))
		for id := range p.Models {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			m := p.Models[id]
			entry := ModelOutput{
				ID: m.ID, Name: m.Name, Family: m.Family,
				ToolCall: m.ToolCall, StrictToolArgs: m.SupportsStrictToolArgs(),
				SchemaConstrainedDecoding:  m.SchemaConstrainedDecoding,
				GrammarConstrainedDecoding: m.GrammarConstrainedDecoding,
				Reasoning:                  m.Reasoning, Context: m.Limit.Context, Output: m.Limit.Output,
			}
			if m.Cost != nil {
				inputUSD, outputUSD := m.Cost.Input, m.Cost.Output
				inputMC, outputMC := m.Cost.InputMicrocentsPerMTok(), m.Cost.OutputMicrocentsPerMTok()
				entry.CostInputUSDPerMTok, entry.CostOutputUSDPerMTok = &inputUSD, &outputUSD
				entry.CostInputMCPerMTok, entry.CostOutputMCPerMTok = &inputMC, &outputMC
			}
			models = append(models, entry)
		}
		providers = append(providers, ProviderOutput{
			ID: p.ID, Name: p.Name, Family: string(p.Family()), API: p.API, Doc: p.Doc,
			Env: slices.Clone(p.Env), Credentialed: p.HasCredentials(credLookup),
			ModelCount: len(p.Models), Models: models,
		})
	}
	meta, _ := s.k.CatalogStore().LoadMeta()
	return ListOutput{
		Providers: providers, Sources: slices.Clone(cat.Sources), ProviderCount: len(providers),
		APISyncedAt: meta.APISyncedAt.Format(time.RFC3339Nano), APISourceURL: meta.APISourceURL,
		LocalSyncedAt: meta.LocalSyncedAt.Format(time.RFC3339Nano), LocalSource: meta.LocalSource,
	}, nil
}

func (s *Service) Discover(ctx context.Context, in DiscoverInput) (DiscoverOutput, error) {
	endpoint := in.Endpoint
	if endpoint == "" {
		endpoint = envOrDefault(brand.EnvPrefix+"OLLAMA_ENDPOINT", providercatalog.DefaultOllamaEndpoint)
	}
	frag, err := providercatalog.DiscoverOllama(ctx, endpoint)
	if err != nil {
		_, _ = s.k.Bus().Publish(event.Spec{
			Subject:       "catalog.discovery",
			Kind:          event.KindCatalogDiscoveryFailed,
			Actor:         "catalog",
			CorrelationID: opapi.CorrelationFromContext(ctx),
			Payload:       map[string]any{"endpoint": endpoint, "error": err.Error()},
		})
		return DiscoverOutput{}, err
	}
	if err := s.k.CatalogStore().SaveLocal(frag, "ollama@"+endpoint); err != nil {
		return DiscoverOutput{}, fmt.Errorf("save: %w", err)
	}
	// Full reload (M928) — same rationale as handleCatalogSync: a freshly
	// discovered local provider must be able to displace the offline mock
	// primary without a daemon restart.
	freshCat, providersReloaded, provErr := s.k.Reload()
	if freshCat == nil {
		return DiscoverOutput{}, fmt.Errorf("reload: %w", provErr)
	}
	modelCount := 0
	for _, p := range frag.Providers {
		modelCount += len(p.Models)
	}
	_, _ = s.k.Bus().Publish(event.Spec{
		Subject:       "catalog.discovery",
		Kind:          event.KindCatalogDiscoveryCompleted,
		Actor:         "catalog",
		CorrelationID: opapi.CorrelationFromContext(ctx),
		Payload: map[string]any{
			"source":      "ollama@" + endpoint,
			"model_count": modelCount,
		},
	})
	result := DiscoverOutput{Endpoint: endpoint, ModelCount: modelCount, ProvidersReloaded: providersReloaded}
	if provErr != nil {
		result.ProviderReloadError = provErr.Error()
	}
	return result, nil
}
