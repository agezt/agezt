// SPDX-License-Identifier: MIT
//
// kernel/controlplane Config Center handlers (handleConfigSchema, handleConfigValues,
// handleConfigSet, handleConfigSchemaRegister, handleConfigSchemaUnregister).
// Extracted from settings.go during Day 211 god-file refactor (#84).
// Public API unchanged.
package controlplane

import (
	"encoding/json"
	"net"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/settings"
)

func (s *Server) handleConfigSchema(conn net.Conn, req Request) {
	reg := settings.NewRegistry(s.baseDir)
	sections := reg.Sections()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"sections":          sections,
		"reload_boundaries": settings.ReloadBoundaries(sections),
	}})
}
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
func (s *Server) handleConfigSet(conn net.Conn, req Request) {
	name, err := requiredArgString(req.Args, "name")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	name = strings.TrimSpace(name)
	value, _, err := argString(req.Args, "value")
	if err != nil {
		s.fail(conn, req, err)
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
		s.fail(conn, req, err)
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
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"id": sec.ID, "registered": true, "applied": "restart",
	}})
}
func (s *Server) handleConfigSchemaUnregister(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	id = strings.TrimSpace(id)
	force, _, err := argBool(req.Args, "force")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	existed, err := settings.NewRegistry(s.baseDir).Unregister(id, force)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"id": id, "removed": existed,
	}})
}
