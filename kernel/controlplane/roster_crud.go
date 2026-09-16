// SPDX-License-Identifier: MIT

// Control-plane roster CRUD: the agent Add / Edit /
// SetEnabled HTTP handlers. The profile-patch helper
// (applyAgentMutableProfilePatch) and the hierarchy
// validators (normalizeAgentProfileKind +
// validateAgentHierarchyRefs +
// managedSubagentDirectCallError) live in
// roster_crud_internal.go. Carved out of roster_crud.go
// during the Day-116 god-file split. Public API unchanged.
package controlplane

import (
	"encoding/json"
	"errors"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/roster"
)

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
	normalizeAgentProfileKind(b, &p)
	p.System = false // System is kernel-owned (set only by guardian seeding); never accept it from a client (M961)
	if err := s.validateAgentHierarchyRefs(p); err != nil {
		s.fail(conn, req, err)
		return
	}
	saved, err := s.k.AddProfile(p)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(saved)}})
}

// handleAgentEdit applies args.profile's MUTABLE fields to the profile named by
// args.ref, using a PATCH semantic: only fields explicitly present in the input
// JSON payload are applied; all other fields remain unchanged. This prevents a
// partial profile (e.g. only {"model":"gpt-5"}) from clearing soul, budget,
// policy fields, etc. Identity/lifecycle fields are protected by the store, so
// a stale client can't rename a slug or resurrect a paused agent.
func (s *Server) handleAgentEdit(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, ok := req.Args["profile"]
	if !ok {
		s.failMsg(conn, req, "args.profile required")
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.failMsg(conn, req, "args.profile: "+err.Error())
		return
	}
	// Parse the raw payload into a flat map to detect which top-level keys the
	// caller explicitly provided (as opposed to zero-value fields from omission).
	provided := map[string]bool{}
	if rawMap, _ := raw.(map[string]any); rawMap != nil {
		for k := range rawMap {
			provided[k] = true
		}
	}
	var in roster.Profile
	if err := json.Unmarshal(b, &in); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	normalizeAgentProfileKind(b, &in)
	// If the caller provided "kind" (e.g. "subagent"), normalizeAgentProfileKind
	// may have set DirectCallable = false on `in`. Propagate that into the
	// provided set so applyAgentMutableProfilePatch applies it.
	if provided["kind"] && in.DirectCallable != nil && !*in.DirectCallable {
		provided["direct_callable"] = true
	}
	current, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	candidate := current
	applyAgentMutableProfilePatch(&candidate, in, provided)
	if err := s.validateAgentHierarchyRefs(candidate); err != nil {
		s.fail(conn, req, err)
		return
	}
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		applyAgentMutableProfilePatch(dst, in, provided)
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(p)}})
}

// applyAgentMutableProfilePatch applies only those fields of `in` whose JSON key
// is present in `provided` to `dst`. This implements a PATCH merge where
// omitted fields keep their current value — fixing the classic "partial edit
// clears omitted fields" bug.

func (s *Server) handleAgentSetEnabled(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
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
		if errors.Is(err, roster.ErrRetired) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + ref + " is retired — revive it first"})
			return
		}
		s.fail(conn, req, err)
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if enabled {
		res["standing_paused"] = s.countAgentPausedStanding(p.Slug)
		res["schedules_paused"] = s.countAgentPausedSchedules(p.Slug)
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}
