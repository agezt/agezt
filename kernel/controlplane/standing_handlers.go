// SPDX-License-Identifier: MIT
//
// kernel/controlplane standing-order CRUD handlers (List, Add, Edit, SetEnabled, Remove).
// Extracted from standing_handlers.go during Day 211 god-file refactor (#83).
// Public API unchanged.
package controlplane

import (
	"encoding/json"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/standing"
)

func (s *Server) handleStandingList(conn net.Conn, req Request) {
	orders := s.k.Standing().List()
	out := make([]any, 0, len(orders))
	enabled := 0
	for _, o := range orders {
		row := standingView(o)
		if err := s.validateStandingAgent(o.Agent); err != nil {
			row["target_status"] = "blocked"
			row["target_error"] = err.Error()
		} else if strings.TrimSpace(o.Agent) != "" {
			row["target_status"] = "ready"
		}
		out = append(out, row)
		if o.Enabled {
			enabled++
		}
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"orders": out, "count": len(out), "enabled_count": enabled},
	})
}
func (s *Server) handleStandingAdd(conn net.Conn, req Request) {
	raw, ok := req.Args["order"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.order required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.order: " + err.Error()})
		return
	}
	var o standing.Order
	if err := json.Unmarshal(b, &o); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.order: " + err.Error()})
		return
	}
	if err := s.validateStandingAgent(o.Agent); err != nil {
		s.fail(conn, req, err)
		return
	}
	saved, err := s.k.AddStanding(o)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"order": standingView(saved)}})
}
func (s *Server) handleStandingEdit(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Decode every patch field up front (first wrong-typed arg fails the whole
	// edit) so the UpdateStanding closure applies a pre-validated patch.
	var argErr error
	str := func(key string) (string, bool) {
		v, present, err := argString(req.Args, key)
		if err != nil && argErr == nil {
			argErr = err
		}
		return v, present
	}
	num := func(key string) (float64, bool) {
		v, present, err := argFloat64(req.Args, key)
		if err != nil && argErr == nil {
			argErr = err
		}
		return v, present
	}
	name, nameSet := str("name")
	plan, planSet := str("plan")
	agent, agentSet := str("agent")
	mode, modeSet := str("mode")
	maxTrust, maxTrustSet := str("max_trust")
	briefingMin, briefingSet := str("briefing_min")
	assure, assureSet := num("assure")
	cooldownSec, cooldownSet := num("cooldown_sec")
	if argErr != nil {
		s.fail(conn, req, argErr)
		return
	}
	if agentSet {
		if err := s.validateStandingAgent(agent); err != nil {
			s.fail(conn, req, err)
			return
		}
	}
	o, ok, err := s.k.UpdateStanding(id, func(o *standing.Order) {
		if nameSet {
			o.Name = name
		}
		if planSet {
			o.Plan = plan
		}
		if agentSet {
			o.Agent = strings.TrimSpace(agent) // M790: run firings AS this roster agent ("" clears)
		}
		if modeSet {
			o.Initiative.Mode = standing.InitiativeMode(mode)
		}
		if maxTrustSet {
			o.Initiative.MaxTrust = maxTrust
		}
		if briefingSet {
			o.BriefingMin = briefingMin
		}
		if assureSet {
			o.Assure = int(assure)
		}
		if cooldownSet {
			o.CooldownSec = int64(cooldownSec)
		}
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": false}})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"updated": true, "order": standingView(o)}})
}
func (s *Server) handleStandingSetEnabled(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
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
	if enabled {
		o, ok := s.k.Standing().Get(id)
		if !ok {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown standing order: " + id})
			return
		}
		if err := s.validateStandingAgent(o.Agent); err != nil {
			s.fail(conn, req, err)
			return
		}
	}
	o, err := s.k.SetStandingEnabled(id, enabled)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"order": standingView(o)}})
}
func (s *Server) handleStandingRemove(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removed, err := s.k.RemoveStanding(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": removed, "id": id}})
}
