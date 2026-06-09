// SPDX-License-Identifier: MIT

package controlplane

// Config Center backend (M693): read the editable configuration schema + current
// values, and write changes. Non-secret values go to the config store
// (<baseDir>/config.json); secret values go to the encrypted vault
// (creds.json). Secret VALUES are never returned — only presence — mirroring the
// existing /api/config privacy rule. Provider/model changes apply live (the
// provider is rebuilt via the same path as `provider_reload`); everything else is
// read at startup, so the response says "restart to apply".

import (
	"net"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

// handleConfigSchema returns the editable configuration surface (sections +
// fields) the Config Center renders forms from.
func (s *Server) handleConfigSchema(conn net.Conn, req Request) {
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"sections": settings.Schema(),
	}})
}

// handleConfigValues returns the current state of every schema field: its value
// (NON-SECRET only — secrets report presence, never the value), whether it's set,
// and whether it's pinned by the real process environment (which wins over the
// config store, so the UI shows it read-only).
func (s *Server) handleConfigValues(conn net.Conn, req Request) {
	store := settings.NewStore(s.baseDir)
	_ = store.Load() // missing file = empty; not fatal
	vault := creds.NewStore(s.baseDir)
	_ = vault.Load()

	out := make([]map[string]any, 0, 32)
	for _, sec := range settings.Schema() {
		for _, f := range sec.Fields {
			pinned := s.configEnvPinned[f.Env]
			entry := map[string]any{
				"env":        f.Env,
				"secret":     f.Secret,
				"env_pinned": pinned,
			}
			if f.Secret {
				// Presence only — the value never leaves the daemon.
				entry["set"] = vault.Has(f.Env)
			} else {
				// Prefer the live env (covers env-pinned + our own injection),
				// fall back to the stored value.
				val := os.Getenv(f.Env)
				if val == "" {
					val, _ = store.Get(f.Env)
				}
				entry["value"] = val
				entry["set"] = val != ""
			}
			out = append(out, entry)
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"fields": out}})
}

// handleConfigSet validates and persists one field. Secret → vault; non-secret →
// config store. A live-apply field (provider/model) is also pushed into the
// process env and the provider is rebuilt immediately; everything else takes
// effect on the next restart. The response says which happened.
func (s *Server) handleConfigSet(conn net.Conn, req Request) {
	name, _ := req.Args["name"].(string)
	value, _ := req.Args["value"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	field, ok := settings.FieldByEnv(name)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown setting " + name})
		return
	}
	if err := settings.Validate(field, value); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	value = strings.TrimSpace(value)

	if field.Secret {
		vault := creds.NewStore(s.baseDir)
		if err := vault.Load(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load vault: " + err.Error()})
			return
		}
		if value == "" {
			vault.Remove(name)
		} else {
			vault.Set(name, value)
		}
		if err := vault.Save(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save vault: " + err.Error()})
			return
		}
	} else {
		store := settings.NewStore(s.baseDir)
		if err := store.Load(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load config: " + err.Error()})
			return
		}
		if value == "" {
			store.Remove(name)
		} else {
			store.Set(name, value)
		}
		if err := store.Save(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
			return
		}
	}

	result := map[string]any{"env": name, "saved": true}

	// env-pinned: the real environment overrides the store, so the edit is saved
	// but won't take effect until the operator unsets the env var.
	if s.configEnvPinned[name] {
		result["applied"] = "restart"
		result["env_pinned"] = true
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
		return
	}

	if field.Apply == settings.ApplyLive && !field.Secret {
		// Push into the live env and rebuild the provider in place — same path as
		// provider_reload. (Live secrets would also reload, but none are live today.)
		_ = os.Setenv(name, value)
		if _, _, err := s.k.Reload(); err != nil {
			result["applied"] = "restart"
			result["reload_error"] = err.Error()
		} else {
			result["applied"] = "live"
		}
	} else {
		result["applied"] = "restart"
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
