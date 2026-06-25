// SPDX-License-Identifier: MIT

package controlplane

// Provider keyring (M700): store many API keys per provider and pick which is
// active — "store many, pick active". Values NEVER leave the daemon — list
// returns label + active + last-4 fingerprint only, mirroring the Config
// Center's secret privacy rule.

import (
	"net"
	"regexp"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/catalog"
	"github.com/agezt/agezt/kernel/creds"
)

// providerEnvPattern constrains a keyring target to a provider-style env var
// (UPPER_SNAKE, may start with a digit — models.dev has e.g. 302AI_API_KEY). The
// AGEZT_ namespace is the Config Center's; provider creds live outside it
// (OPENAI_API_KEY, ANTHROPIC_API_KEY, …), so reject AGEZT_ here.
var providerEnvPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_]*$`)
var providerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// keyEnv validates and returns the env-var name from req.Args["env"].
func keyEnv(req Request) (string, bool) {
	env, _ := req.Args["env"].(string)
	env = strings.TrimSpace(env)
	if !providerEnvPattern.MatchString(env) || strings.HasPrefix(env, brand.EnvPrefix) {
		return "", false
	}
	return env, true
}

// keyTarget returns the display provider/env plus the actual vault keyring
// target. Without args.provider this preserves the legacy env-global keyring;
// with args.provider it stores under provider:<id>:<ENV>, so providers that
// share the same models.dev env name do not share API keys accidentally.
func keyTarget(req Request) (provider, env, target string, ok bool) {
	env, ok = keyEnv(req)
	if !ok {
		return "", "", "", false
	}
	provider, _ = req.Args["provider"].(string)
	provider = strings.TrimSpace(provider)
	if provider != "" {
		if !providerIDPattern.MatchString(provider) {
			return "", "", "", false
		}
		target = catalog.ProviderCredentialName(provider, env)
	} else {
		target = env
	}
	return provider, env, target, true
}

func keyTargetError() Response {
	return Response{Type: RespError, Error: "args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*); args.provider, when set, must be a catalog provider id"}
}

func (s *Server) loadVault(conn net.Conn, req Request) (*creds.Store, bool) {
	vault := creds.NewStore(s.baseDir)
	if err := vault.Load(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load vault: " + err.Error()})
		return nil, false
	}
	return vault, true
}

// handleProviderKeyList returns the keys stored for a provider env var — labels,
// which is active, and a last-4 fingerprint. Never the values.
func (s *Server) handleProviderKeyList(conn net.Conn, req Request) {
	provider, env, target, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"provider": provider, "env": env, "keys": vault.KeyringList(target),
	}})
}

// handleProviderKeyAdd stores a new key under a label. If it becomes active (the
// first key, or active=true), the provider is reloaded in place.
func (s *Server) handleProviderKeyAdd(conn net.Conn, req Request) {
	provider, env, target, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	label, _ := req.Args["label"].(string)
	label = strings.TrimSpace(label)
	value, _ := req.Args["value"].(string)
	makeActive, _ := req.Args["active"].(bool)

	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	activeChanged, err := vault.KeyringAdd(target, label, value, makeActive)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	result := map[string]any{"provider": provider, "env": env, "label": label, "added": true, "active_changed": activeChanged}
	if activeChanged {
		if _, _, err := s.k.Reload(); err != nil {
			result["reload_error"] = err.Error()
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleProviderKeyActivate switches the active key and reloads the provider.
func (s *Server) handleProviderKeyActivate(conn net.Conn, req Request) {
	provider, env, target, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	label, _ := req.Args["label"].(string)
	label = strings.TrimSpace(label)

	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	if err := vault.KeyringActivate(target, label); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	result := map[string]any{"provider": provider, "env": env, "label": label, "active": true}
	if _, _, err := s.k.Reload(); err != nil {
		result["reload_error"] = err.Error()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleProviderKeyRemove deletes a key. Removing the active key clears the bare
// name (provider uncredentialed until another is activated) and reloads.
func (s *Server) handleProviderKeyRemove(conn net.Conn, req Request) {
	provider, env, target, ok := keyTarget(req)
	if !ok {
		resp := keyTargetError()
		resp.ID = req.ID
		s.writeResp(conn, resp)
		return
	}
	label, _ := req.Args["label"].(string)
	label = strings.TrimSpace(label)

	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	removed, wasActive := vault.KeyringRemove(target, label)
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	result := map[string]any{"provider": provider, "env": env, "label": label, "removed": removed, "was_active": wasActive}
	if wasActive {
		if _, _, err := s.k.Reload(); err != nil {
			result["reload_error"] = err.Error()
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
