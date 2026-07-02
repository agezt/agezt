// SPDX-License-Identifier: MIT

package controlplane

// Forge / skill inspection + lifecycle handlers — the read/govern path behind
// `agt skill`. Transitions go through the kernel's skill.Forge so every
// promote/quarantine/revert is journaled (skill.*) and auditable via `agt
// why`, exactly like a transition the agent itself proposed.

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/skill"
)

func (s *Server) handleSkillList(conn net.Conn, req Request) {
	sks, err := s.k.Forge().List()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	out := make([]any, 0, len(sks))
	active := 0
	for _, sk := range sks {
		out = append(out, skillView(sk))
		if sk.Active() {
			active++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"skills": out, "count": len(out), "active_count": active},
	})
}

func (s *Server) handleSkillGet(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	sk, found, err := s.k.Forge().Get(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := map[string]any{"found": found}
	if found {
		result["skill"] = skillView(sk)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleSkillHistory folds the journal for every lifecycle event that names
// this skill id, newest-last (chronological), so `agt skill history` reads as
// the skill's life story.
func (s *Server) handleSkillHistory(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	var events []any
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if !isSkillKind(e.Kind) {
			return nil
		}
		var p map[string]any
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		// Match the event's "id" (or a revert's "restored") to this skill.
		if p["id"] != id && p["restored"] != id {
			return nil
		}
		events = append(events, map[string]any{
			"seq":            e.Seq,
			"id":             e.ID,
			"kind":           string(e.Kind),
			"correlation_id": e.CorrelationID,
			"ts_unix_ms":     e.TSUnixMS,
			"payload":        p,
		})
		return nil
	})
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"id": id, "events": events, "count": len(events)},
	})
}

func (s *Server) handleSkillPromote(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	status, err := s.k.Forge().Promote("", id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": id, "status": string(status)},
	})
}

func (s *Server) handleSkillQuarantine(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	reason, _ := req.Args["reason"].(string)
	if err := s.k.Forge().Quarantine("", id, reason); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": id, "status": string(skill.StatusQuarantined)},
	})
}

func (s *Server) handleSkillArchive(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	reason, _ := req.Args["reason"].(string)
	if err := s.k.Forge().Archive("", id, reason); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"id": id, "status": string(skill.StatusArchived), "reason": reason,
		},
	})
}

func (s *Server) handleSkillRevert(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	restored, err := s.k.Forge().Revert("", id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": id, "restored": restored},
	})
}

func (s *Server) handleSkillRestore(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	statusText, _ := req.Args["status"].(string)
	if statusText == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.status required"})
		return
	}
	reason, _ := req.Args["reason"].(string)
	from, to, err := s.k.Forge().RestoreStatus("", id, skill.Status(statusText), reason)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": id, "from": string(from), "status": string(to), "reason": reason},
	})
}

// handleSkillShare promotes a private per-agent skill (M932) into the shared
// pool — the ownership analogue of memory_promote (M915). Clears Skill.Agent.
func (s *Server) handleSkillShare(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	sk, found, err := s.k.Forge().Reassign("", id, "")
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := map[string]any{"shared": found, "id": id}
	if found {
		result["name"] = sk.Name
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleSkillReassign changes a skill's owning agent (M942). An empty agent
// shares the skill; a non-empty slug must exist in the roster.
func (s *Server) handleSkillReassign(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	agent, _ := req.Args["agent"].(string)
	if agent != "" {
		if _, ok := s.k.Roster().Get(agent); !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no such agent: " + agent})
			return
		}
	}
	sk, found, err := s.k.Forge().Reassign("", id, agent)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	result := map[string]any{"reassigned": found, "id": id, "to_agent": agent}
	if found {
		result["name"] = sk.Name
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleSkillImport installs a skill from a portable bundle (M269). It routes
// through the Forge's Create, so the imported skill is content-addressed,
// deduped against any identical existing skill, and journaled (skill.created) —
// it arrives as a fresh DRAFT regardless of the source's lifecycle, never an
// active skill, so an operator must still promote it before it injects into
// runs.
func (s *Server) handleSkillImport(conn net.Conn, req Request) {
	name := stringArg(req.Args, "name")
	body := stringArg(req.Args, "body")
	if name == "" || body == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name and args.body required"})
		return
	}
	triggers, _, terr := argStringList(req.Args, "triggers")
	if terr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: terr.Error()})
		return
	}
	tools, _, toerr := argStringList(req.Args, "tools_required")
	if toerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: toerr.Error()})
		return
	}
	resources, rerr := argResources(req.Args, "resources")
	if rerr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: rerr.Error()})
		return
	}
	sk, created, err := s.k.Forge().Create("", skill.CreateSpec{
		Name:          name,
		Description:   stringArg(req.Args, "description"),
		Triggers:      triggers,
		Body:          body,
		ToolsRequired: tools,
		Resources:     resources,
		Agent:         stringArg(req.Args, "agent"),
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{
			"id": sk.ID, "name": sk.Name, "status": string(sk.Status),
			"created": created, "resources": sk.Resources,
		},
	})
}

// argResources decodes the optional resources bundle from a control-plane call:
// a JSON object mapping each relative path to that file's text content. Absent
// or empty → nil (a body-only skill). A non-object value is rejected.
func argResources(args map[string]any, key string) (map[string][]byte, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("args.%s must be an object of {path: content}", key)
	}
	if len(obj) == 0 {
		return nil, nil
	}
	out := make(map[string][]byte, len(obj))
	for path, v := range obj {
		content, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("args.%s[%q] must be a string", key, path)
		}
		out[path] = []byte(content)
	}
	return out, nil
}

// handleSkillFiles lists a skill's bundle resources (relative paths) plus the
// absolute bundle directory the agent runs scripts from. Read-only.
func (s *Server) handleSkillFiles(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	sk, found, err := s.k.Forge().Get(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no skill with id " + id})
		return
	}
	bundles := s.k.Forge().Bundles()
	files := sk.Resources
	dir := ""
	if bundles != nil {
		if live, lerr := bundles.List(sk.Name); lerr == nil && live != nil {
			files = live // the on-disk truth, in case the manifest drifted
		}
		dir = bundles.Dir(sk.Name)
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": sk.ID, "name": sk.Name, "files": files, "dir": dir, "count": len(files)},
	})
}

// handleSkillReadFile returns the text content of one bundle resource. Read-only;
// the bundle store rejects any path that escapes the skill's directory.
func (s *Server) handleSkillReadFile(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	path, _ := req.Args["path"].(string)
	if id == "" || path == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id and args.path required"})
		return
	}
	sk, found, err := s.k.Forge().Get(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "no skill with id " + id})
		return
	}
	bundles := s.k.Forge().Bundles()
	if bundles == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "skill bundles are not available on this daemon"})
		return
	}
	data, err := bundles.Read(sk.Name, path)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{"id": sk.ID, "name": sk.Name, "path": path, "content": string(data), "bytes": len(data)},
	})
}

// handleSkillHygiene reports which active skills look idle (never used, or not
// used in idle_days) so an operator can prune dead weight from the retrieval pool
// (M858). Read-only; the cleanup action is the existing CmdSkillQuarantine.
func (s *Server) handleSkillHygiene(conn net.Conn, req Request) {
	days := dlInt(req.Args, "idle_days")
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	rep, err := s.k.Forge().Hygiene(cutoff)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	idle := make([]any, 0, len(rep.Idle))
	for _, sk := range rep.Idle {
		v := skillView(sk)
		v["uses"] = sk.Metrics.Uses
		v["last_used_ms"] = sk.Metrics.LastUsedMS
		idle = append(idle, v)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"idle_days": days, "total": rep.Total, "active": rep.Active,
		"idle": idle, "idle_count": len(idle),
	}})
}

func isSkillKind(k event.Kind) bool {
	switch k {
	case event.KindSkillCreated, event.KindSkillPromoted, event.KindSkillQuarantined,
		event.KindSkillReverted, event.KindSkillRestored, event.KindSkillActivated:
		return true
	}
	return false
}

// skillView renders a skill.Skill as a stable JSON object for the wire.
func skillView(sk skill.Skill) map[string]any {
	v := map[string]any{
		"id":           sk.ID,
		"name":         sk.Name,
		"description":  sk.Description,
		"status":       string(sk.Status),
		"version":      sk.Version,
		"agent":        sk.Agent,
		"created_ms":   sk.CreatedMS,
		"last_seen_ms": sk.LastSeenMS,
		"metrics": map[string]any{
			"uses": sk.Metrics.Uses, "successes": sk.Metrics.Successes,
			"failures": sk.Metrics.Failures, "last_used_ms": sk.Metrics.LastUsedMS,
			"shadow_evals": sk.Metrics.ShadowEvals, "shadow_wins": sk.Metrics.ShadowWins,
		},
	}
	if len(sk.Triggers) > 0 {
		v["triggers"] = sk.Triggers
	}
	if len(sk.ToolsRequired) > 0 {
		v["tools_required"] = sk.ToolsRequired
	}
	if len(sk.Resources) > 0 {
		v["resources"] = sk.Resources
	}
	if len(sk.Lineage) > 0 {
		v["lineage"] = sk.Lineage
	}
	if sk.Body != "" {
		v["body"] = sk.Body
	}
	if sk.SourceEvent != "" {
		v["source_event"] = sk.SourceEvent
	}
	return v
}
