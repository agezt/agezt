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
	url, _, err := argString(req.Args, "url")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if url == "" {
		url = envOrDefault(brand.EnvPrefix+"CATALOG_URL", catalog.DefaultSyncURL)
	}
	syncer := catalog.NewSyncer()
	syncer.URL = url
	if t, _, terr := argFloat64(req.Args, "timeout_s"); terr != nil {
		s.fail(conn, req, terr)
		return
	} else if t > 0 {
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
		s.fail(conn, req, err)
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
	endpoint, _, err := argString(req.Args, "endpoint")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
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
		s.fail(conn, req, err)
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

// handleProviderConnect is the catalog-aware "register a provider + key" path
// for the Web UI's Provider Keys tab and the CLI's `provider connect`.
//
// Behavior, by id presence in the merged catalog (api + local + custom):
//
//	id already in catalog  → DO NOT touch custom.json. The existing entry
//	                         (full model list from models.dev, prices,
//	                         capabilities) is preserved as-is. Returns
//	                         {added:false, exists:true, ...} so the caller
//	                         knows no upsert happened. This is the fix for
//	                         the orphan-with-one-model bug: previously
//	                         custom.json WINS the merge, so any upsert
//	                         would wholesale-replace a models.dev-synced
//	                         entry with the user's stripped-down shape
//	                         and lose the model list.
//
//	id is new              → write a MINIMAL partial Provider to custom.json:
//	                         {id, name, npm, api, env} only. Models is
//	                         intentionally left nil (unknown coverage) so
//	                         the Governor accepts whatever model id the
//	                         operator names at chat time — the catalog no
//	                         longer pretends to know what this endpoint
//	                         serves. The `model` arg is purely a UI hint
//	                         for the caller to push to AGEZT_MODEL; the
//	                         backend does not seed it into Models.
//
// The API key itself always travels separately on the keys/add path so
// the secret value never sits on this handler. custom.json still wins
// the merge when the id is new, which is the intended behavior for a
// brand-new provider.
func (s *Server) handleProviderConnect(conn net.Conn, req Request) {
	sa, err := argStrings(req.Args, "id", "api", "model", "env", "name", "npm")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	id := strings.TrimSpace(sa["id"])
	api := strings.TrimSpace(sa["api"])
	// `model` is now informational — see the catalog-aware note above. The
	// UI/CLI uses it to push AGEZT_MODEL when the user asks for a default
	// brain; the backend no longer seeds it into the catalog Models map.
	_ = strings.TrimSpace(sa["model"])
	// env is OPTIONAL: a keyless local runtime (Ollama, LM Studio, …) connects
	// with no API key. When present it must be a valid provider env var.
	var envs []string
	if strings.TrimSpace(sa["env"]) != "" {
		env, ok := keyEnv(req)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.env must be a provider key env var (UPPER_SNAKE, not AGEZT_*)"})
			return
		}
		envs = []string{env}
	}
	if id == "" || api == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id and args.api are required"})
		return
	}
	name := sa["name"]
	if strings.TrimSpace(name) == "" {
		name = id
	}
	npm := sa["npm"]
	if strings.TrimSpace(npm) == "" {
		npm = "@ai-sdk/openai-compatible"
	}

	// Catalog-aware gate: if the merged catalog already knows this id
	// (from models.dev sync, local.json discovery, or a prior custom.json
	// write), preserve the existing entry. No upsert, no clobber. The
	// caller attaches the key on the separate keys/add path.
	cat := s.k.Catalog()
	if _, exists := cat.Providers[id]; exists {
		_, providersReloaded, rerr := s.k.Reload()
		result := map[string]any{
			"provider_id":         id,
			"added":               false,
			"exists":              true,
			"providers_reloaded":  providersReloaded,
			"note":                "id already in catalog; custom.json was NOT written — existing entry preserved. Attach the key via /api/provider/keys/add.",
		}
		if rerr != nil {
			result["reload_error"] = rerr.Error()
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
		return
	}

	// New id: minimal partial entry. No synthetic model list — see the
	// type-level doc above for why.
	p := &catalog.Provider{
		ID:     id,
		Name:   strings.TrimSpace(name),
		NPM:    strings.TrimSpace(npm),
		API:    api,
		Env:    envs,
		Models: nil, // unknown coverage — see kernel/governor/modelchain_skip_test.go
	}
	added, err := s.k.CatalogStore().UpsertCustomProvider(p)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save custom provider: " + err.Error()})
		return
	}
	_, providersReloaded, rerr := s.k.Reload()
	result := map[string]any{
		"provider_id":        id,
		"added":              added,
		"exists":             false,
		"providers_reloaded": providersReloaded,
	}
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
		s.fail(conn, req, err)
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

// registerCatalogCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerCatalogCommands() {
	register(
		commandSpec{Cmd: CmdCatalogSync, Handler: func(dc *DispatchCtx) { dc.S.handleCatalogSync(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCatalogList, Handler: func(dc *DispatchCtx) { dc.S.handleCatalogList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCatalogDiscover, Handler: func(dc *DispatchCtx) { dc.S.handleCatalogDiscover(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderReload, Handler: func(dc *DispatchCtx) { dc.S.handleProviderReload(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderConnect, Handler: func(dc *DispatchCtx) { dc.S.handleProviderConnect(dc.Conn, dc.Req) }},
	)
}
