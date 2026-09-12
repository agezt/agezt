// SPDX-License-Identifier: MIT

// Control-plane roster CRUD: Add/Edit/SetEnabled + profile-patch + hierarchy validators.
// Code extracted from roster_crud.go during the Day-116 god-file split.
// Public API unchanged.
package controlplane


import (
	"errors"
	"fmt"
	"net"
	"strings"

	"encoding/json"
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
func applyAgentMutableProfilePatch(dst *roster.Profile, in roster.Profile, provided map[string]bool) {
	if provided["name"] {
		dst.Name = in.Name
	}
	if provided["soul"] {
		dst.Soul = in.Soul
	}
	if provided["instructions"] {
		dst.Instructions = in.Instructions
	}
	if provided["model"] {
		dst.Model = in.Model
	}
	if provided["fallbacks"] {
		dst.Fallbacks = in.Fallbacks
	}
	if provided["task_type"] {
		dst.TaskType = in.TaskType
	}
	if provided["max_cost_mc"] {
		dst.MaxCostMc = in.MaxCostMc
	}
	if provided["max_daily_mc"] {
		dst.MaxDailyMc = in.MaxDailyMc
	}
	if provided["memory_scope"] {
		dst.MemoryScope = in.MemoryScope
	}
	if provided["workdir"] {
		dst.Workdir = in.Workdir
	}
	if provided["owner_agent"] {
		dst.OwnerAgent = in.OwnerAgent
	}
	if provided["parent_agent"] {
		dst.ParentAgent = in.ParentAgent
	}
	if provided["direct_callable"] {
		dst.DirectCallable = in.DirectCallable
	}
	if provided["retry_policy"] {
		dst.RetryPolicy = in.RetryPolicy
	}
	if provided["health_policy"] {
		dst.HealthPolicy = in.HealthPolicy
	}
	if provided["self_repair"] {
		dst.SelfRepairPolicy = in.SelfRepairPolicy
	}
	if provided["noise_policy"] {
		dst.NoisePolicy = in.NoisePolicy
	}
	if provided["tool_allow"] {
		dst.ToolAllow = in.ToolAllow
	}
	if provided["tool_deny"] {
		dst.ToolDeny = in.ToolDeny
	}
	if provided["trust_ceiling"] {
		dst.TrustCeiling = in.TrustCeiling
	}
	if provided["execution_profile"] {
		dst.ExecutionProfile = strings.TrimSpace(in.ExecutionProfile)
	}
	if provided["config_overrides"] {
		dst.ConfigOverrides = in.ConfigOverrides
	}
	if provided["lifecycle"] {
		dst.Lifecycle = in.Lifecycle
	}
	if provided["tasklist"] {
		dst.TaskList = in.TaskList
	}
	if provided["description"] {
		dst.Description = in.Description
	}
}

func normalizeAgentProfileKind(raw []byte, p *roster.Profile) {
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(meta.Kind), "subagent") {
		no := false
		p.DirectCallable = &no
	}
}

func (s *Server) validateAgentHierarchyRefs(p roster.Profile) error {
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.EqualFold(ref, strings.TrimSpace(p.Slug)) {
			return fmt.Errorf("roster: %s cannot point to the same agent", label)
		}
		target, ok := s.k.Roster().Get(ref)
		if !ok {
			return fmt.Errorf("roster: %s %q does not exist", label, ref)
		}
		if target.Retired {
			return fmt.Errorf("roster: %s %q is retired", label, ref)
		}
	}
	return nil
}

func managedSubagentDirectCallError(p roster.Profile, action string) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot be " + action + " directly; " + hint
}

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

