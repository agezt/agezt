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
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
)

// agentModelChain builds a named agent's run chain: the resolved primary model
// first, then the profile's ordered fallbacks, skipping duplicates of the
// primary (so an explicit --model equal to a fallback doesn't try it twice).
func agentModelChain(primary string, fallbacks []string) []string {
	chain := []string{primary}
	for _, m := range fallbacks {
		if m = strings.TrimSpace(m); m != "" && m != primary {
			chain = append(chain, m)
		}
	}
	return chain
}

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
		dst.MaxDailyMc = in.MaxDailyMc
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

// handleAgentImpact reports what depends on an agent (standing orders that fire
// it) — shown before retiring so the operator sees the effects (M846).
func (s *Server) handleAgentImpact(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	orders := s.k.AgentImpact(p.Slug)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"slug": p.Slug, "standing_orders": orders, "standing_count": len(orders),
	}})
}

// handleAgentActivity builds a per-agent activity timeline (M854): what an agent
// did — the runs it executed, the council consults and sub-agent delegations
// during those runs, the memory it wrote (M851 actor), its board messages, and
// changes to its own profile. Derived entirely from the journal (no new store),
// newest first. Answers the owner's "ne oldu ne bitti, hangi agent fikir danıştı".
func (s *Server) handleAgentActivity(conn net.Conn, req Request) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	slug := p.Slug
	limit := 50
	if raw, ok := req.Args["limit"].(float64); ok && raw > 0 {
		limit = int(raw)
	}
	if limit > 500 {
		limit = 500
	}

	// Pass 1: the correlation ids of runs this agent executed (task.received
	// carries the agent slug since M854). These also scope the council consults
	// and delegations that happened *during* the agent's runs.
	// Also collect activity events in the same pass to avoid O(2n) journal walks.
	runCorr := map[string]bool{}
	var items []map[string]any
	_ = s.k.Journal().Range(func(e *event.Event) error {
		// Build runCorr map
		if e.Kind == event.KindTaskReceived {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil && plString(pl, "agent") == slug && e.CorrelationID != "" {
				runCorr[e.CorrelationID] = true
			}
		}
		// Check if this event is attributable to the agent
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		summary, ok := agentActivitySummary(e, pl, slug, runCorr)
		if !ok {
			return nil
		}
		items = append(items, map[string]any{
			"seq":            e.Seq,
			"kind":           string(e.Kind),
			"ts_unix_ms":     e.TSUnixMS,
			"correlation_id": e.CorrelationID,
			"summary":        summary,
		})
		return nil
	})

	// Newest first, capped.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i]["seq"].(int64) > items[j]["seq"].(int64)
	})
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"slug": slug, "activity": items, "count": len(items), "total": total,
	}})
}

// agentActivitySummary decides whether one event belongs in an agent's timeline
// and renders a one-line summary. Attribution is by the slug fields the events
// already carry, plus the agent's own run correlations for run-scoped events.
func agentActivitySummary(e *event.Event, pl map[string]any, slug string, runCorr map[string]bool) (string, bool) {
	switch e.Kind {
	case event.KindTaskReceived:
		if plString(pl, "agent") == slug {
			return "started a run: " + truncate(plString(pl, "intent"), 100), true
		}
	case event.KindTaskCompleted:
		if runCorr[e.CorrelationID] {
			return "completed a run", true
		}
	case event.KindTaskFailed:
		if runCorr[e.CorrelationID] {
			r := plString(pl, "reason")
			if r == "" {
				r = "failed"
			}
			return "run failed: " + truncate(r, 80), true
		}
	case event.KindCouncilConvened:
		if runCorr[e.CorrelationID] {
			return "consulted the council: " + truncate(plString(pl, "question"), 100), true
		}
	case event.KindSubAgentSpawned:
		// The agent delegated (its run spawned a sub-agent), or it WAS the named
		// sub-agent that ran.
		if runCorr[e.CorrelationID] {
			return "delegated to a sub-agent: " + truncate(plString(pl, "agent"), 60), true
		}
		if plString(pl, "agent") == slug {
			return "ran as a delegated sub-agent", true
		}
	case event.KindMemoryWritten:
		if plString(pl, "actor") == slug {
			return "memory " + plString(pl, "action") + ": " + truncate(plString(pl, "subject"), 80), true
		}
	case event.KindBoardPosted:
		if plString(pl, "from") == slug {
			if to := plString(pl, "to"); to != "" {
				return "messaged " + to, true
			}
			return "posted to the board: " + truncate(plString(pl, "topic"), 60), true
		}
	case event.KindRosterUpdated:
		if plString(pl, "slug") == slug {
			a := plString(pl, "action")
			if a == "" {
				a = "updated"
			}
			return "profile " + a, true
		}
	}
	return "", false
}

func plString(pl map[string]any, key string) string {
	s, _ := pl[key].(string)
	return s
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Server) handleAgentRetire(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, true)
}

func (s *Server) handleAgentRevive(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, false)
}

func (s *Server) handleAgentSetRetired(conn net.Conn, req Request, retired bool) {
	ref, _ := req.Args["ref"].(string)
	if strings.TrimSpace(ref) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.ref required"})
		return
	}
	// Compute impact BEFORE the state change so a retire reports what it affected.
	var impact []string
	if retired {
		if p, ok := s.k.Roster().Get(ref); ok {
			impact = s.k.AgentImpact(p.Slug)
		}
	}
	p, err := s.k.SetProfileRetired(ref, retired)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if retired {
		res["impact"] = impact
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
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
