// SPDX-License-Identifier: MIT

package controlplane

// Agent CRUD handlers (M783): add / edit / set-enabled / task-update plus
// the profile-patch + hierarchy-validation helpers. Carved out of roster.go
// during the Day 24 god file split #3 so the main file can focus on
// lifecycle (pause/revive/remove/wake/repair/escalation).

import (
	"encoding/json"
	"errors"
	"fmt"
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

func (s *Server) handleAgentTaskUpdate(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	op, _, err := argString(req.Args, "op")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	op = strings.ToLower(strings.TrimSpace(op))
	if op == "" {
		op = "update"
	}
	if op != "add" && op != "update" && op != "remove" && op != "delete" {
		s.failMsg(conn, req, "args.op must be add, update, or remove")
		return
	}
	var in roster.AgentTask
	if raw, ok := req.Args["task"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			s.failMsg(conn, req, "args.task: "+err.Error())
			return
		}
		if err := json.Unmarshal(b, &in); err != nil {
			s.failMsg(conn, req, "args.task: "+err.Error())
			return
		}
	}
	// Flat-arg overrides layered over args.task (both transports are live).
	for _, f := range []struct {
		key string
		dst *string
	}{
		{"id", &in.ID}, {"title", &in.Title}, {"description", &in.Description},
		{"scope", &in.Scope}, {"status", &in.Status},
	} {
		if v, present, err := argString(req.Args, f.key); err != nil {
			s.fail(conn, req, err)
			return
		} else if present {
			*f.dst = v
		}
	}
	titleProvided := hasArg(req.Args, "title") || taskFieldPresent(req.Args["task"], "title")
	scopeProvided := hasArg(req.Args, "scope") || taskFieldPresent(req.Args["task"], "scope")
	statusProvided := hasArg(req.Args, "status") || taskFieldPresent(req.Args["task"], "status")
	if op == "add" || (op == "update" && titleProvided) {
		if strings.TrimSpace(in.Title) == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
	}
	if scopeProvided {
		switch strings.TrimSpace(in.Scope) {
		case "", "cycle", "total":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.scope must be cycle or total"})
			return
		}
	}
	if statusProvided {
		switch strings.TrimSpace(in.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.status must be todo, doing, done, blocked, or retired"})
			return
		}
	}
	var task roster.AgentTask
	found := false
	p, exists, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		switch op {
		case "add":
			task = in
			dst.TaskList = append(dst.TaskList, task)
			found = true
		case "update":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				if _, ok := req.Args["title"]; ok || in.Title != "" {
					dst.TaskList[i].Title = in.Title
				}
				if _, ok := req.Args["description"]; ok || in.Description != "" {
					dst.TaskList[i].Description = in.Description
				}
				if _, ok := req.Args["scope"]; ok || in.Scope != "" {
					dst.TaskList[i].Scope = in.Scope
				}
				if _, ok := req.Args["status"]; ok || in.Status != "" {
					dst.TaskList[i].Status = in.Status
				}
				task = dst.TaskList[i]
				found = true
				return
			}
		case "remove", "delete":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				task = dst.TaskList[i]
				dst.TaskList = append(append([]roster.AgentTask{}, dst.TaskList[:i]...), dst.TaskList[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !exists {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if !found {
		if strings.TrimSpace(in.ID) == "" && op != "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
			return
		}
		if op == "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent task: " + in.ID})
		return
	}
	if op == "add" {
		for _, t := range p.TaskList {
			if t.Title == strings.TrimSpace(in.Title) && (strings.TrimSpace(in.ID) == "" || t.ID == strings.TrimSpace(in.ID)) {
				task = t
			}
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"updated": true,
		"profile": profileView(p),
		"task":    task,
	}})
}

func hasArg(args map[string]any, key string) bool {
	_, ok := args[key]
	return ok
}

func taskFieldPresent(raw any, key string) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, ok = obj[key]
	return ok
}

// handleAgentImpact reports what depends on an agent — shown before retiring or
// removing so the operator sees the effects (M846).
