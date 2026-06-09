// SPDX-License-Identifier: MIT

package controlplane

// Provider keyring (M700): store many API keys per provider env var in the vault
// and pick which is active — "store many, pick active". The active key is what
// the provider's CredLookup reads (the bare env-var name); switching reloads the
// provider in place. Values NEVER leave the daemon — list returns label + active
// + last-4 fingerprint only, mirroring the Config Center's secret privacy rule.

import (
	"net"
	"regexp"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/creds"
)

// providerEnvPattern constrains a keyring target to a provider-style env var
// (UPPER_SNAKE, may start with a digit — models.dev has e.g. 302AI_API_KEY). The
// AGEZT_ namespace is the Config Center's; provider creds live outside it
// (OPENAI_API_KEY, ANTHROPIC_API_KEY, …), so reject AGEZT_ here.
var providerEnvPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_]*$`)

// keyEnv validates and returns the env-var name from req.Args["env"].
func keyEnv(req Request) (string, bool) {
	env, _ := req.Args["env"].(string)
	env = strings.TrimSpace(env)
	if !providerEnvPattern.MatchString(env) || strings.HasPrefix(env, brand.EnvPrefix) {
		return "", false
	}
	return env, true
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
	env, ok := keyEnv(req)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*)"})
		return
	}
	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"env": env, "keys": vault.KeyringList(env),
	}})
}

// handleProviderKeyAdd stores a new key under a label. If it becomes active (the
// first key, or active=true), the provider is reloaded in place.
func (s *Server) handleProviderKeyAdd(conn net.Conn, req Request) {
	env, ok := keyEnv(req)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*)"})
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
	activeChanged, err := vault.KeyringAdd(env, label, value, makeActive)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	result := map[string]any{"env": env, "label": label, "added": true, "active_changed": activeChanged}
	if activeChanged {
		if _, _, err := s.k.Reload(); err != nil {
			result["reload_error"] = err.Error()
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleProviderKeyActivate switches the active key and reloads the provider.
func (s *Server) handleProviderKeyActivate(conn net.Conn, req Request) {
	env, ok := keyEnv(req)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*)"})
		return
	}
	label, _ := req.Args["label"].(string)
	label = strings.TrimSpace(label)

	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	if err := vault.KeyringActivate(env, label); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	result := map[string]any{"env": env, "label": label, "active": true}
	if _, _, err := s.k.Reload(); err != nil {
		result["reload_error"] = err.Error()
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleProviderKeyRemove deletes a key. Removing the active key clears the bare
// name (provider uncredentialed until another is activated) and reloads.
func (s *Server) handleProviderKeyRemove(conn net.Conn, req Request) {
	env, ok := keyEnv(req)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.env must be a provider env var (UPPER_SNAKE, not AGEZT_*)"})
		return
	}
	label, _ := req.Args["label"].(string)
	label = strings.TrimSpace(label)

	vault, ok := s.loadVault(conn, req)
	if !ok {
		return
	}
	removed, wasActive := vault.KeyringRemove(env, label)
	if err := vault.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
		return
	}
	result := map[string]any{"env": env, "label": label, "removed": removed, "was_active": wasActive}
	if wasActive {
		if _, _, err := s.k.Reload(); err != nil {
			result["reload_error"] = err.Error()
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
