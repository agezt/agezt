// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"os"
	"sort"
	"time"

	"github.com/ersinkoc/agezt/internal/brand"
	"github.com/ersinkoc/agezt/kernel/catalog"
	"github.com/ersinkoc/agezt/kernel/event"
)

// handleCatalogSync runs a remote sync (models.dev/api.json or whatever
// AGEZT_CATALOG_URL says), writes api.json + meta atomically, reloads
// the in-process catalog, and publishes catalog.synced (or
// catalog.sync_failed). Args:
//
//	url        (optional) override the sync URL for this call
//	timeout_s  (optional) override the per-call timeout
func (s *Server) handleCatalogSync(ctx context.Context, conn net.Conn, req Request) {
	url, _ := req.Args["url"].(string)
	if url == "" {
		url = envOrDefault(brand.EnvPrefix+"CATALOG_URL", catalog.DefaultSyncURL)
	}
	syncer := catalog.NewSyncer()
	syncer.URL = url
	if t, ok := req.Args["timeout_s"].(float64); ok && t > 0 {
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	if err := s.k.CatalogStore().SaveAPI(raw, url); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save: " + err.Error()})
		return
	}
	if _, err := s.k.ReloadCatalog(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "reload: " + err.Error()})
		return
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
	_ = cat // already installed via ReloadCatalog
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{
			"url":            url,
			"bytes":          res.Bytes,
			"provider_count": res.ProviderCount,
			"model_count":    res.ModelCount,
			"duration_ms":    res.Duration.Milliseconds(),
		},
	})
}

// handleCatalogList projects the loaded catalog into a wire shape:
// each provider with id, name, family, base url, credentialed flag,
// and its models with id, family, prices (microcents), capabilities.
// Used by `agt catalog list` and by future `agt provider list`.
func (s *Server) handleCatalogList(conn net.Conn, req Request) {
	cat := s.k.Catalog()
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
				"id":        m.ID,
				"name":      m.Name,
				"family":    m.Family,
				"tool_call": m.ToolCall,
				"reasoning": m.Reasoning,
				"context":   m.Limit.Context,
				"output":    m.Limit.Output,
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
			"id":            p.ID,
			"name":          p.Name,
			"family":        string(p.Family()),
			"api":           p.API,
			"doc":           p.Doc,
			"env":           p.Env,
			"credentialed":  p.HasCredentials(os.Getenv),
			"model_count":   len(p.Models),
			"models":        models,
		})
	}
	meta, _ := s.k.CatalogStore().LoadMeta()
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{
			"providers":       providers,
			"sources":         cat.Sources,
			"api_synced_at":   meta.APISyncedAt,
			"api_source_url":  meta.APISourceURL,
			"local_synced_at": meta.LocalSyncedAt,
			"local_source":    meta.LocalSource,
			"provider_count":  len(providers),
		},
	})
}

// handleCatalogDiscover runs Ollama-style auto-discovery against the
// supplied (or env-default) endpoint, writes the synthesised provider
// to local.json, and reloads. Failure is per-call non-fatal; the
// catalog.discovery_failed event surfaces why.
func (s *Server) handleCatalogDiscover(ctx context.Context, conn net.Conn, req Request) {
	endpoint, _ := req.Args["endpoint"].(string)
	if endpoint == "" {
		endpoint = envOrDefault(brand.EnvPrefix+"OLLAMA_ENDPOINT", catalog.DefaultOllamaEndpoint)
	}
	frag, err := catalog.DiscoverOllama(ctx, endpoint)
	if err != nil {
		_, _ = s.k.Bus().Publish(event.Spec{
			Subject: "catalog.discovery",
			Kind:    event.KindCatalogDiscoveryFailed,
			Actor:   "catalog",
			Payload: map[string]any{"endpoint": endpoint, "error": err.Error()},
		})
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if err := s.k.CatalogStore().SaveLocal(frag, "ollama@"+endpoint); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save: " + err.Error()})
		return
	}
	if _, err := s.k.ReloadCatalog(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "reload: " + err.Error()})
		return
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
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{
			"endpoint":    endpoint,
			"model_count": modelCount,
		},
	})
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// handleProviderReload re-reads catalog files + vault from disk and
// rebuilds the primary provider in place. The catalog refresh always
// happens; the provider rebuild runs only when the daemon configured
// runtime.Config.OnReload (cmd/agezt does so by default).
//
// This is the operator-facing replacement for the "restart the daemon
// to pick up this change" hint that `agt provider creds set` printed
// since M1.o. Result carries `providers_reloaded: bool` so the CLI
// can tell the operator which path actually ran.
func (s *Server) handleProviderReload(conn net.Conn, req Request) {
	cat, providersReloaded, err := s.k.Reload()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := map[string]any{
		"providers_reloaded": providersReloaded,
		"provider_count":     len(cat.Providers),
	}
	if !providersReloaded {
		// Surface the no-op clearly so operators don't wonder why a
		// creds change didn't take effect: when the daemon was built
		// without OnReload, only the catalog refresh ran.
		result["note"] = "OnReload not configured; only the catalog snapshot was refreshed. Restart the daemon for the new credentials to take effect."
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
