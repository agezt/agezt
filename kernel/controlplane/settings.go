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
	"encoding/json"
	"net"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

// handleConfigSchema returns the editable configuration surface (sections +
// fields) the Config Center renders forms from — the built-in schema merged with
// any skill/plugin-registered sections from <baseDir>/schemas/*.json.
func (s *Server) handleConfigSchema(conn net.Conn, req Request) {
	reg := settings.NewRegistry(s.baseDir)
	sections := reg.Sections()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"sections":          sections,
		"reload_boundaries": settings.ReloadBoundaries(sections),
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
	reg := settings.NewRegistry(s.baseDir)

	out := make([]map[string]any, 0, 32)
	for _, sec := range reg.Sections() {
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
	field, ok := settings.NewRegistry(s.baseDir).FieldByEnv(name)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown setting " + name})
		return
	}
	if field.ReadOnly {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: name + " is read-only and cannot be changed from the Config Center"})
		return
	}
	if err := settings.Validate(field, value); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	value = strings.TrimSpace(value)
	if field.Locked && value == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: name + " is locked and cannot be cleared"})
		return
	}

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

	switch {
	case field.Apply == settings.ApplyLive && field.Secret:
		// A live SECRET (e.g. AGEZT_WEB_PASSWORD, M933) is pushed into the env —
		// consumers that read it lazily (the console password gate) pick it up
		// immediately. No provider rebuild: a secret edit doesn't change routing.
		setLiveEnv(name, value)
		result["applied"] = "live"
	case field.Apply == settings.ApplyLive && !configFieldNeedsKernelReload(name):
		// Some live non-secret settings are read lazily from the process env at
		// submission/check time (for example execution-profile policy). Push them
		// into the env without rebuilding the provider.
		setLiveEnv(name, value)
		result["applied"] = "live"
	case field.Apply == settings.ApplyLive:
		// Push into the live env and rebuild the provider in place — same path as
		// provider_reload.
		setLiveEnv(name, value)
		if _, _, err := s.k.Reload(); err != nil {
			result["applied"] = "restart"
			result["reload_error"] = err.Error()
		} else {
			result["applied"] = "live"
		}
	default:
		result["applied"] = "restart"
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func configFieldNeedsKernelReload(name string) bool {
	switch name {
	case "AGEZT_PROVIDER", "AGEZT_MODEL":
		return true
	default:
		return false
	}
}

func setLiveEnv(name, value string) {
	if value == "" {
		_ = os.Unsetenv(name)
		return
	}
	_ = os.Setenv(name, value)
}

// handleConfigSchemaRegister persists a skill/plugin-contributed schema section to
// <baseDir>/schemas/<id>.json. The section is validated (slug id, namespaced
// AGEZT_* fields, no shadowing of a built-in setting); on success it appears in
// the Config Center immediately. The arg `section` is the JSON section object.
func (s *Server) handleConfigSchemaRegister(conn net.Conn, req Request) {
	raw, ok := req.Args["section"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.section required"})
		return
	}
	// The section arrives as decoded JSON (map[string]any); round-trip it into the
	// typed Section so validation sees the real shape.
	blob, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "encode section: " + err.Error()})
		return
	}
	var sec settings.Section
	if err := json.Unmarshal(blob, &sec); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "decode section: " + err.Error()})
		return
	}
	if err := settings.NewRegistry(s.baseDir).Register(sec); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"id": sec.ID, "registered": true, "applied": "restart",
	}})
}

// handleConfigSchemaUnregister removes a registered schema section by id. The
// section's stored VALUES (config.json / vault) are left untouched; only the
// schema is removed, so the Config Center stops showing the section. A Locked
// (system-approved) section is refused unless args.force is truthy.
func (s *Server) handleConfigSchemaUnregister(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	force, _ := req.Args["force"].(bool)
	existed, err := settings.NewRegistry(s.baseDir).Unregister(id, force)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"id": id, "removed": existed,
	}})
}
