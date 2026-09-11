// SPDX-License-Identifier: MIT

// Autonomy feed handler: handleAutonomyFeed + autonomyDetail.
// Code extracted from autonomy.go during the Day-50 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)


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
		s.fail(conn, req, err)
		return
	}

	// Walk newest-first, keeping only self-directed kinds up to the limit.
	items := make([]map[string]any, 0, limit)
	for i := len(tail) - 1; i >= 0 && len(items) < limit; i-- {
		e := tail[i]
		category, title, ok := autonomyMeta(e)
		if !ok {
			continue
		}
		item := map[string]any{
			"seq":            e.Seq,
			"ts_unix_ms":     e.TSUnixMS,
			"kind":           string(e.Kind),
			"subject":        e.Subject,
			"category":       category,
			"title":          title,
			"correlation_id": e.CorrelationID,
		}
		if category == "doctor" {
			var p map[string]any
			if json.Unmarshal(e.Payload, &p) == nil {
				if v := strPayload(p, "agent"); v != "" {
					item["agent"] = v
				}
				if v := strPayload(p, "target_agent"); v != "" {
					item["target_agent"] = v
				}
				if v := strPayload(p, "delegate_to"); v != "" {
					item["delegate_to"] = v
				}
				if v := strPayload(p, "delegated_by"); v != "" {
					item["delegated_by"] = v
				}
				if v := strPayload(p, "root_agent"); v != "" {
					item["root_agent"] = v
				}
				if v := strPayload(p, "incident_id"); v != "" {
					item["incident_id"] = v
				}
				if v := strPayload(p, "root_incident_id"); v != "" {
					item["root_incident_id"] = v
				}
				if v := strPayload(p, "parent_incident_id"); v != "" {
					item["parent_incident_id"] = v
				}
				if v := strPayload(p, "phase"); v != "" {
					item["phase"] = v
				}
				if v := strPayload(p, "mode"); v != "" {
					item["mode"] = v
				}
				if v := strPayload(p, "resolution"); v != "" {
					item["resolution"] = v
				}
				if v := strPayload(p, "routing_task_type"); v != "" {
					item["routing_task_type"] = v
				}
				if chain := strSlicePayload(p, "routing_task_model_chain"); len(chain) > 0 {
					item["routing_task_model_chain"] = chain
				}
				if n, ok := p["routing_force_generation"].(float64); ok {
					item["routing_force_generation"] = int(n)
				}
				if n, ok := p["chain_depth"].(float64); ok {
					item["chain_depth"] = int(n)
				}
			}
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
		return strPayload(p, k)
	}
	if e.Kind == event.KindInfo {
		if d := autonomyDoctorDetail(e.Subject, p); d != "" {
			return d
		}
	}
	switch e.Kind {
	case event.KindScheduleFired, event.KindStandingFired:
		if v := str("intent"); v != "" {
			return clipDetail(v)
		}
		return str("name")
	case event.KindSubAgentSpawned:
		var parts []string
		if agent := str("agent"); agent != "" {
			parts = append(parts, agent)
		}
		if by := str("delegated_by"); by != "" {
			parts = append(parts, "by "+by)
		}
		if task := clipDetail(str("task")); task != "" {
			parts = append(parts, task)
		}
		return strings.Join(parts, " · ")
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
