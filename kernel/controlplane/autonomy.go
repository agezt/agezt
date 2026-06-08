// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

// autonomyScanN is how many recent journal events the feed folds over. The feed
// is a curated "what did the system do on its own" view, so it scans a generous
// window and keeps only the self-directed milestones.
const (
	autonomyScanN        = 2000
	autonomyDefaultLimit = 60
	autonomyMaxLimit     = 200
)

// autonomyKinds maps each self-directed event kind to a human category + verb for
// the autonomy feed. Only kinds in this map appear — reactive plumbing
// (llm.*, tool.*, policy.*) is intentionally excluded so the feed reads as the
// organism's proactive life, not a firehose.
var autonomyKinds = map[event.Kind]struct{ category, verb string }{
	event.KindScheduleFired:    {"schedule", "a schedule fired"},
	event.KindStandingFired:    {"standing", "a standing order fired"},
	event.KindStandingCreated:  {"standing", "a standing order was created"},
	event.KindStandingError:    {"standing", "a standing order errored"},
	event.KindAssureVerdict:    {"assure", "a completion check ran"},
	event.KindSkillCreated:     {"skill", "a skill was learned"},
	event.KindSkillPromoted:    {"skill", "a skill was promoted"},
	event.KindSkillQuarantined: {"skill", "a skill was quarantined"},
	event.KindSkillReverted:    {"skill", "a skill change was reverted"},
	event.KindBriefingSent:     {"pulse", "a briefing was sent"},
	event.KindBoardPosted:      {"board", "an agent posted to the board"},
}

// handleAutonomyFeed serves CmdAutonomyFeed: a curated, newest-first timeline of
// the daemon's self-directed activity (schedules, standing orders, skill
// lifecycle, completion checks, briefings), folded from the journal so the Web
// UI can show the living organism acting on its own. Read-only.
func (s *Server) handleAutonomyFeed(conn net.Conn, req Request) {
	limit := autonomyDefaultLimit
	if raw, ok := req.Args["limit"]; ok {
		if v, ok := raw.(float64); ok {
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > autonomyMaxLimit {
		limit = autonomyMaxLimit
	}

	tail, err := s.k.Journal().Tail(autonomyScanN)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Walk newest-first, keeping only self-directed kinds up to the limit.
	items := make([]map[string]any, 0, limit)
	for i := len(tail) - 1; i >= 0 && len(items) < limit; i-- {
		e := tail[i]
		meta, ok := autonomyKinds[e.Kind]
		if !ok {
			continue
		}
		item := map[string]any{
			"seq":            e.Seq,
			"ts_unix_ms":     e.TSUnixMS,
			"kind":           string(e.Kind),
			"category":       meta.category,
			"title":          meta.verb,
			"correlation_id": e.CorrelationID,
		}
		if d := autonomyDetail(e); d != "" {
			item["detail"] = d
		}
		items = append(items, item)
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"items": items, "count": len(items)},
	})
}

// autonomyDetail pulls a short, human detail string from an event's payload so a
// feed row says something concrete ("intent: digest the inbox", "skill:
// diagnose-ci", "complete: true"). Best-effort — a payload that doesn't carry the
// expected field just yields no detail.
func autonomyDetail(e *event.Event) string {
	if len(e.Payload) == 0 {
		return ""
	}
	var p map[string]any
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return ""
	}
	str := func(k string) string {
		if v, ok := p[k].(string); ok {
			return v
		}
		return ""
	}
	switch e.Kind {
	case event.KindScheduleFired, event.KindStandingFired:
		if v := str("intent"); v != "" {
			return clipDetail(v)
		}
		return str("name")
	case event.KindStandingCreated, event.KindStandingError:
		return str("name")
	case event.KindSkillCreated, event.KindSkillPromoted, event.KindSkillQuarantined, event.KindSkillReverted:
		if v := str("name"); v != "" {
			return v
		}
		return str("id")
	case event.KindAssureVerdict:
		if c, ok := p["complete"].(bool); ok {
			if c {
				return "complete: true"
			}
			if g := str("gap"); g != "" {
				return "gap: " + clipDetail(g)
			}
			return "complete: false"
		}
	case event.KindBriefingSent:
		return str("subject")
	case event.KindBoardPosted:
		if topic := str("topic"); topic != "" {
			if from := str("from"); from != "" {
				return topic + " · from " + from
			}
			return topic
		}
	}
	return ""
}

func clipDetail(s string) string {
	const max = 120
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
