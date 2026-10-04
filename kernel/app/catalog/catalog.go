// SPDX-License-Identifier: MIT

// Package catalog owns transport-independent catalog sync, listing and discovery.
// Existing presentation/persistence/reload behavior remains while operation binding follows.
package catalog

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	providercatalog "github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

type SyncInput struct {
	URL            string
	TimeoutSeconds float64
}
type ListInput struct{}
type DiscoverInput struct{ Endpoint string }
type SyncOutput = map[string]any
type ListOutput = map[string]any
type DiscoverOutput = map[string]any

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
			Subject: "catalog.sync",
			Kind:    event.KindCatalogSyncFailed,
			Actor:   "catalog",
			Payload: map[string]any{"url": url, "error": err.Error()},
		})
		return nil, err
	}

	if err := s.k.CatalogStore().SaveAPI(raw, url); err != nil {
		return nil, fmt.Errorf("save: %w", err)
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
		return nil, fmt.Errorf("reload: %w", provErr)
	}
	_, _ = s.k.Bus().Publish(event.Spec{
		Subject: "catalog.sync",
		Kind:    event.KindCatalogSynced,
		Actor:   "catalog",
		Payload: map[string]any{
			"url":            url,
			"bytes":          res.Bytes,
			"provider_count": res.ProviderCount,
			"model_count":    res.ModelCount,
			"duration_ms":    res.Duration.Milliseconds(),
		},
	})
	_ = cat // already installed via Reload
	result := map[string]any{
		"url":                url,
		"bytes":              res.Bytes,
		"provider_count":     res.ProviderCount,
		"model_count":        res.ModelCount,
		"duration_ms":        res.Duration.Milliseconds(),
		"providers_reloaded": providersReloaded,
	}
	if provErr != nil {
		result["provider_reload_error"] = provErr.Error()
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
	providers := make([]map[string]any, 0, len(cat.Providers))
	for _, p := range cat.ProviderList() {
		models := make([]map[string]any, 0, len(p.Models))
		// Deterministic order for stable CLI output.
		ids := make([]string, 0, len(p.Models))
		for id := range p.Models {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			m := p.Models[id]
			entry := map[string]any{
				"id":                           m.ID,
				"name":                         m.Name,
				"family":                       m.Family,
				"tool_call":                    m.ToolCall,
				"strict_tool_args":             m.SupportsStrictToolArgs(),
				"schema_constrained_decoding":  m.SchemaConstrainedDecoding,
				"grammar_constrained_decoding": m.GrammarConstrainedDecoding,
				"reasoning":                    m.Reasoning,
				"context":                      m.Limit.Context,
				"output":                       m.Limit.Output,
			}
			if m.Cost != nil {
				entry["cost_input_usd_per_mtok"] = m.Cost.Input
				entry["cost_output_usd_per_mtok"] = m.Cost.Output
				entry["cost_input_mc_per_mtok"] = m.Cost.InputMicrocentsPerMTok()
				entry["cost_output_mc_per_mtok"] = m.Cost.OutputMicrocentsPerMTok()
			}
			models = append(models, entry)
		}
		providers = append(providers, map[string]any{
			"id":           p.ID,
			"name":         p.Name,
			"family":       string(p.Family()),
			"api":          p.API,
			"doc":          p.Doc,
			"env":          p.Env,
			"credentialed": p.HasCredentials(credLookup),
			"model_count":  len(p.Models),
			"models":       models,
		})
	}
	meta, _ := s.k.CatalogStore().LoadMeta()
	return map[string]any{
		"providers":       providers,
		"sources":         cat.Sources,
		"api_synced_at":   meta.APISyncedAt,
		"api_source_url":  meta.APISourceURL,
		"local_synced_at": meta.LocalSyncedAt,
		"local_source":    meta.LocalSource,
		"provider_count":  len(providers),
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
			Subject: "catalog.discovery",
			Kind:    event.KindCatalogDiscoveryFailed,
			Actor:   "catalog",
			Payload: map[string]any{"endpoint": endpoint, "error": err.Error()},
		})
		return nil, err
	}
	if err := s.k.CatalogStore().SaveLocal(frag, "ollama@"+endpoint); err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	// Full reload (M928) — same rationale as handleCatalogSync: a freshly
	// discovered local provider must be able to displace the offline mock
	// primary without a daemon restart.
	freshCat, providersReloaded, provErr := s.k.Reload()
	if freshCat == nil {
		return nil, fmt.Errorf("reload: %w", provErr)
	}
	modelCount := 0
	for _, p := range frag.Providers {
		modelCount += len(p.Models)
	}
	_, _ = s.k.Bus().Publish(event.Spec{
		Subject: "catalog.discovery",
		Kind:    event.KindCatalogDiscoveryCompleted,
		Actor:   "catalog",
		Payload: map[string]any{
			"source":      "ollama@" + endpoint,
			"model_count": modelCount,
		},
	})
	result := map[string]any{
		"endpoint":           endpoint,
		"model_count":        modelCount,
		"providers_reloaded": providersReloaded,
	}
	if provErr != nil {
		result["provider_reload_error"] = provErr.Error()
	}
	return result, nil
}
