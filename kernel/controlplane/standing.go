// SPDX-License-Identifier: MIT

package controlplane

// Standing-order CRUD handlers — the management path behind `agt standing`.
// Standing orders are durable event/cron wake rules, not agent identities.
// Lifecycle changes go through the kernel so every create/pause/resume/remove
// is journaled (standing.*) and auditable via `agt why`.

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/standing"
)

// standingView is the stable wire shape for one order.
func standingView(o standing.Order) map[string]any {
	b, _ := json.Marshal(o)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if warning := standingFrequencyWarning(o); warning != "" {
		m["frequency_warning"] = warning
	}
	return m
}

func standingFrequencyWarning(o standing.Order) string {
	hasEvent := false
	for _, t := range o.Triggers {
		if t.Type == standing.TriggerEvent {
			hasEvent = true
		}
		if t.Type == standing.TriggerCron {
			first := ""
			if fields := strings.Fields(strings.TrimSpace(t.Schedule)); len(fields) > 0 {
				first = fields[0]
			}
			if first == "*" || first == "*/1" || first == "0/1" {
				return "cron trigger may wake this standing order every minute"
			}
		}
	}
	if hasEvent && o.CooldownSec > 0 && o.CooldownSec < 15*60 {
		return "event cooldown is below the default 15m guard"
	}
	return ""
}

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

// handleStandingEdit edits an order's mutable fields in place (M729): any subset
// of name/plan/initiative-mode/max-trust/briefing-disposition/assure/cooldown. Triggers,
// observers and scope are not touched here (they keep their current values), and
// enabled has its own pause/resume path. Unknown id → {updated:false}, mirroring
// the schedule-edit path. Every edit is journaled (standing.updated, "edited").
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

func (s *Server) validateStandingAgent(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		return fmt.Errorf("unknown standing agent: %s", ref)
	}
	if p.Retired {
		return fmt.Errorf("standing agent %s is retired", p.Slug)
	}
	if !p.Enabled {
		return fmt.Errorf("standing agent %s is paused", p.Slug)
	}
	if !p.AllowsDirectCall() {
		return fmt.Errorf("standing %s", managedSubagentDirectCallError(p, "called"))
	}
	return nil
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

// handleStandingWhy folds the journal for every standing.* event naming this
// order id — its life story: created, paused/resumed, every time it fired, and
// removed (SPEC-16 §4). Mirrors `agt skill history`.
func (s *Server) handleStandingWhy(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var events []any
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if !strings.HasPrefix(string(e.Kind), "standing.") {
			return nil
		}
		var p map[string]any
		if json.Unmarshal(e.Payload, &p) != nil {
			return nil
		}
		if p["id"] != id {
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

// SetStandingFire wires the on-demand fire path (M765). The daemon injects a closure
// that looks up the order and launches it through the same governed run path a cron/
// event trigger uses, so the control plane stays decoupled from the run launcher.
func (s *Server) SetStandingFire(fn func(id string) bool) { s.standingFire = fn }

// handleStandingFire triggers one standing order now (M765) — the sibling of
// schedule "run now" and pulse "beat now". It launches the order's run regardless of
// its cron/event triggers (useful to test an order or run it on demand). Returns as
// soon as the run is dispatched; the result lands in the journal / Runs view.
func (s *Server) handleStandingFire(conn net.Conn, req Request) {
	id, err := requiredArgString(req.Args, "id")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if s.standingFire == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "standing-order firing is not available on this daemon"})
		return
	}
	o, ok := s.k.Standing().Get(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"fired": false, "id": id}})
		return
	}
	if err := s.validateStandingAgent(o.Agent); err != nil {
		s.fail(conn, req, err)
		return
	}
	fired := s.standingFire(id)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"fired": fired, "id": id}})
}

// registerStandingCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerStandingCommands() {
	register(
		commandSpec{Cmd: CmdStandingList, Handler: func(dc *DispatchCtx) { dc.S.handleStandingList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingAdd, Handler: func(dc *DispatchCtx) { dc.S.handleStandingAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingEdit, Handler: func(dc *DispatchCtx) { dc.S.handleStandingEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleStandingSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingRemove, Handler: func(dc *DispatchCtx) { dc.S.handleStandingRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingFire, Handler: func(dc *DispatchCtx) { dc.S.handleStandingFire(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdStandingWhy, Handler: func(dc *DispatchCtx) { dc.S.handleStandingWhy(dc.Conn, dc.Req) }},
	)
}
