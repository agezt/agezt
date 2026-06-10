// SPDX-License-Identifier: MIT

package controlplane

// Agent roster CRUD handlers (M783) — the management path behind `agt agent`.
// Lifecycle changes go through the kernel so every create/edit/pause/resume/
// remove is journaled (roster.*) and auditable via `agt why`. Profiles are
// addressed by ref = id OR slug everywhere, so operators can say
// `agt agent show researcher` without copying ULIDs.

import (
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

// profileView is the stable wire shape for one profile.
func profileView(p roster.Profile) map[string]any {
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func (s *Server) handleAgentList(conn net.Conn, req Request) {
	profiles := s.k.Roster().List()
	out := make([]any, 0, len(profiles))
	enabled := 0
	for _, p := range profiles {
		out = append(out, profileView(p))
		if p.Enabled {
			enabled++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"profiles": out, "count": len(out), "enabled_count": enabled},
	})
}

func (s *Server) handleAgentAdd(conn net.Conn, req Request) {
	raw, ok := req.Args["profile"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	var p roster.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	saved, err := s.k.AddProfile(p)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(saved)}})
}

// handleAgentEdit applies args.profile's MUTABLE fields wholesale to the
// profile named by args.ref (identity/lifecycle fields are protected by the
// store, so a stale client can't rename a slug or resurrect a paused agent).
func (s *Server) handleAgentEdit(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	raw, ok := req.Args["profile"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	var in roster.Profile
	if err := json.Unmarshal(b, &in); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		dst.Name = in.Name
		dst.Soul = in.Soul
		dst.Model = in.Model
		dst.Fallbacks = in.Fallbacks
		dst.TaskType = in.TaskType
		dst.MaxCostMc = in.MaxCostMc
		dst.MemoryScope = in.MemoryScope
		dst.Workdir = in.Workdir
		dst.Description = in.Description
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(p)}})
}

func (s *Server) handleAgentSetEnabled(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	// Accept enabled as a bool (CLI/JSON) or a "true"/"false"/"1"/"0" string
	// (the webui query-arg transport carries every value as a string).
	enabled := false
	switch v := req.Args["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	}
	p, err := s.k.SetProfileEnabled(ref, enabled)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(p)}})
}

func (s *Server) handleAgentRemove(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	ok, err := s.k.RemoveProfile(ref)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": ok}})
}
