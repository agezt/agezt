// SPDX-License-Identifier: MIT

package controlplane

// Provider catalog management: handleProviderConnect + handleProviderReload
// + envOrDefault. Carved out of catalog.go during the Day 154 god-file
// split so the main file can focus on catalog sync / list / discover.
// Public API unchanged.

import (
	"net"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/catalog"
)
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

