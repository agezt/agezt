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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	saved, err := s.k.AddStanding(o)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	if v, ok := req.Args["agent"].(string); ok {
		if err := s.validateStandingAgent(v); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}
	o, ok, err := s.k.UpdateStanding(id, func(o *standing.Order) {
		if v, ok := req.Args["name"].(string); ok {
			o.Name = v
		}
		if v, ok := req.Args["plan"].(string); ok {
			o.Plan = v
		}
		if v, ok := req.Args["agent"].(string); ok {
			o.Agent = strings.TrimSpace(v) // M790: run firings AS this roster agent ("" clears)
		}
		if v, ok := req.Args["mode"].(string); ok {
			o.Initiative.Mode = standing.InitiativeMode(v)
		}
		if v, ok := req.Args["max_trust"].(string); ok {
			o.Initiative.MaxTrust = v
		}
		if v, ok := req.Args["briefing_min"].(string); ok {
			o.BriefingMin = v
		}
		// assure is numeric; the JSON body carries it as a float64.
		if v, ok := req.Args["assure"].(float64); ok {
			o.Assure = int(v)
		}
		if v, ok := req.Args["cooldown_sec"].(float64); ok {
			o.CooldownSec = int64(v)
		}
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
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
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
	}
	o, err := s.k.SetStandingEnabled(id, enabled)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"order": standingView(o)}})
}

// handleStandingWhy folds the journal for every standing.* event naming this
// order id — its life story: created, paused/resumed, every time it fired, and
// removed (SPEC-16 §4). Mirrors `agt skill history`.
func (s *Server) handleStandingWhy(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
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
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	removed, err := s.k.RemoveStanding(id)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	id, _ := req.Args["id"].(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	fired := s.standingFire(id)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"fired": fired, "id": id}})
}
