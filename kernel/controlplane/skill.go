// SPDX-License-Identifier: MIT

package controlplane

// Forge / skill inspection + lifecycle handlers — the read/govern path behind
// `agt skill`. Transitions go through the kernel's skill.Forge so every
// promote/quarantine/revert is journaled (skill.*) and auditable via `agt
// why`, exactly like a transition the agent itself proposed.

import (
	"encoding/json"
	"net"

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
	sk, created, err := s.k.Forge().Create("", skill.CreateSpec{
		Name:          name,
		Description:   stringArg(req.Args, "description"),
		Triggers:      triggers,
		Body:          body,
		ToolsRequired: tools,
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID: req.ID, Type: RespResult,
		Result: map[string]any{
			"id": sk.ID, "name": sk.Name, "status": string(sk.Status), "created": created,
		},
	})
}

func isSkillKind(k event.Kind) bool {
	switch k {
	case event.KindSkillCreated, event.KindSkillPromoted, event.KindSkillQuarantined,
		event.KindSkillReverted, event.KindSkillActivated:
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
		"created_ms":   sk.CreatedMS,
		"last_seen_ms": sk.LastSeenMS,
		"metrics": map[string]any{
			"uses": sk.Metrics.Uses, "successes": sk.Metrics.Successes,
			"failures": sk.Metrics.Failures, "last_used_ms": sk.Metrics.LastUsedMS,
		},
	}
	if len(sk.Triggers) > 0 {
		v["triggers"] = sk.Triggers
	}
	if len(sk.ToolsRequired) > 0 {
		v["tools_required"] = sk.ToolsRequired
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
