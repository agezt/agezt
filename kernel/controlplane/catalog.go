// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "reload: " + provErr.Error()})
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
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleCatalogList projects the loaded catalog into a wire shape:
// each provider with id, name, family, base url, credentialed flag,
// and its models with id, family, prices (microcents), capabilities.
// Used by `agt catalog list` and by future `agt provider list`.
func (s *Server) handleCatalogList(conn net.Conn, req Request) {
	cat := s.k.Catalog()
	// A provider is "credentialed" if a key exists for it in the process env OR the
	// vault — provider keys (incl. the M700 keyring) live in the vault, so checking
	// os.Getenv alone would miss them and mark keyed providers as un-keyed.
	vault := creds.NewStore(s.baseDir)
	_ = vault.Load()
	duplicateEnv := cat.DuplicateCredentialEnvs()
	credLookup := func(name string) string {
		name = strings.TrimSpace(name)
		if catalog.IsProviderCredentialName(name) {
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
	// Full reload (M928) — same rationale as handleCatalogSync: a freshly
	// discovered local provider must be able to displace the offline mock
	// primary without a daemon restart.
	freshCat, providersReloaded, provErr := s.k.Reload()
	if freshCat == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "reload: " + provErr.Error()})
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
	result := map[string]any{
		"endpoint":           endpoint,
		"model_count":        modelCount,
		"providers_reloaded": providersReloaded,
	}
	if provErr != nil {
		result["provider_reload_error"] = provErr.Error()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func envOrDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// handleProviderConnect registers (or replaces) a provider in the custom.json
// catalog layer and reloads in place — the backend half of the Web UI's "Quick
// Connect" gallery. It writes only the provider definition (id/name/npm/api/env
// + one model); the API key itself travels separately on the secret
// keys/add path. custom.json wins the merge, so this pins the exact base URL a
// coding-plan endpoint needs even when models.dev ships a different default.
func (s *Server) handleProviderConnect(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	id = strings.TrimSpace(id)
	api, _ := req.Args["api"].(string)
	api = strings.TrimSpace(api)
	model, _ := req.Args["model"].(string)
	model = strings.TrimSpace(model)
	// env is OPTIONAL: a keyless local runtime (Ollama, LM Studio, …) connects
	// with no API key. When present it must be a valid provider env var.
	var envs []string
	if raw, _ := req.Args["env"].(string); strings.TrimSpace(raw) != "" {
		env, ok := keyEnv(req)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.env must be a provider key env var (UPPER_SNAKE, not AGEZT_*)"})
			return
		}
		envs = []string{env}
	}
	if id == "" || api == "" || model == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id, args.api and args.model are required"})
		return
	}
	name, _ := req.Args["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = id
	}
	npm, _ := req.Args["npm"].(string)
	if strings.TrimSpace(npm) == "" {
		npm = "@ai-sdk/openai-compatible"
	}

	p := &catalog.Provider{
		ID:   id,
		Name: strings.TrimSpace(name),
		NPM:  strings.TrimSpace(npm),
		API:  api,
		Env:  envs,
		Models: map[string]*catalog.Model{
			model: {ID: model, Name: model, ToolCall: true},
		},
	}
	added, err := s.k.CatalogStore().UpsertCustomProvider(p)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save custom provider: " + err.Error()})
		return
	}
	_, providersReloaded, rerr := s.k.Reload()
	result := map[string]any{"provider_id": id, "added": added, "providers_reloaded": providersReloaded}
	if rerr != nil {
		result["reload_error"] = rerr.Error()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
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
