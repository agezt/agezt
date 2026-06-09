// SPDX-License-Identifier: MIT

package controlplane

// Agent persona (M710): view and edit the default system prompt that frames every
// run — the daemon's "personality" and standing instructions. Unlike `agt config`
// (which reports only whether a system prompt is SET, never its content, since a
// generic config dump could leak proprietary instructions), this is the owner's
// dedicated editor, so it returns and accepts the full text. Edits apply LIVE
// (kernel.SetSystem — the next run uses it, no restart) and persist to the config
// store as AGEZT_SYSTEM_PROMPT so they survive a restart (startup injection).

import (
	"net"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/settings"
)

// handlePersonaGet returns the live system prompt (the agent persona) and whether
// one is set. Content is returned because this is the owner editing their own daemon.
func (s *Server) handlePersonaGet(conn net.Conn, req Request) {
	system := s.k.System()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"system": system,
		"set":    system != "",
	}})
}

// handlePersonaSet replaces the agent persona. args.system is the new prompt (an
// empty/blank value clears it, returning to no custom persona). Applies live and
// persists to the config store as AGEZT_SYSTEM_PROMPT.
func (s *Server) handlePersonaSet(conn net.Conn, req Request) {
	raw, ok := req.Args["system"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.system required (string; empty to clear)"})
		return
	}
	system, ok := raw.(string)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.system must be a string"})
		return
	}

	// Persist to the config store (survives restart via injection at startup).
	store := settings.NewStore(s.baseDir)
	if err := store.Load(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "load config: " + err.Error()})
		return
	}
	envName := brand.EnvPrefix + "SYSTEM_PROMPT"
	if system != "" {
		store.Set(envName, system)
	} else {
		store.Remove(envName)
	}
	if err := store.Save(); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "save config: " + err.Error()})
		return
	}

	// Apply live — the next run frames itself with the new persona.
	s.k.SetSystem(system)

	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"saved":   true,
		"applied": "live",
		"set":     system != "",
		"length":  len(system),
	}})
}
