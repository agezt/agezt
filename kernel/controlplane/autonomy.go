// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

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
	event.KindSubAgentSpawned:  {"delegation", "a sub-agent was delegated"},
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

func autonomyMeta(e *event.Event) (category, title string, ok bool) {
	if e.Kind == event.KindInfo {
		if category, title, ok := autonomyDoctorMeta(e.Subject, e.Payload); ok {
			return category, title, true
		}
	}
	meta, ok := autonomyKinds[e.Kind]
	if !ok {
		return "", "", false
	}
	// Keep the feed identity-level, not a firehose: only NAMED sub-agent
	// delegations (those that ran AS a roster agent) are proactive organism life;
	// anonymous fan-out spawns are skipped.
	if e.Kind == event.KindSubAgentSpawned && strPayloadMap(e.Payload, "agent") == "" {
		return "", "", false
	}
	return meta.category, meta.verb, true
}

func autonomyDoctorMeta(subject string, payload []byte) (category, title string, ok bool) {
	if subject == "doctor.auto_repair" {
		var p map[string]any
		if json.Unmarshal(payload, &p) == nil {
			mode := strings.TrimSpace(strPayload(p, "mode"))
			phase := strings.TrimSpace(strPayload(p, "phase"))
			switch phase {
			case "routing_forced_failed_detected":
				return "doctor", "a forced-chain failure escalation was queued", true
			case "routing_force_exhausted_detected":
				return "doctor", "a forced-chain exhaustion escalation was queued", true
			case "routing_unstable_detected":
				return "doctor", "an unstable routing escalation was queued", true
			case "attempts_exhausted":
				return "doctor", "a self-repair attempt budget was exhausted", true
			case "queued":
				if mode == "degraded" {
					return "doctor", "a doctor run was queued", true
				}
				if mode == "routing" {
					return "doctor", "a routing repair was queued", true
				}
				return "doctor", "a config repair was queued", true
			case "routing_rollback_queued":
				return "doctor", "a routing rollback was queued", true
			case "completed":
				if mode == "degraded" {
					return "doctor", "a doctor run repaired an agent", true
				}
				if mode == "routing" {
					return "doctor", "a routing repair rewrote a chain", true
				}
				return "doctor", "a config repair was applied", true
			case "routing_rollback_completed":
				return "doctor", "a routing rollback restored a chain", true
			case "failed":
				if mode == "degraded" {
					return "doctor", "a doctor run failed", true
				}
				if mode == "routing" {
					return "doctor", "a routing repair failed", true
				}
				return "doctor", "a config repair failed", true
			case "routing_rollback_failed":
				return "doctor", "a routing rollback failed", true
			case "escalation_woke":
				return "doctor", "an owner agent was woken", true
			case "escalation_answered":
				return "doctor", "an owner agent answered the escalation", true
			case "resolution_applied":
				return "doctor", "an owner resolution was applied", true
			case "escalation_skipped":
				return "doctor", "an owner wake was skipped", true
			case "escalation_failed":
				return "doctor", "an owner wake failed", true
			case "resolution_failed":
				return "doctor", "a resolution follow-up failed", true
			case "delegation_queued":
				return "doctor", "a delegated follow-up was queued", true
			case "delegation_woke":
				return "doctor", "a delegated agent was woken", true
			case "delegation_failed":
				return "doctor", "a delegated wake failed", true
			default:
				return "doctor", "a doctor action ran", true
			}
		}
		return "doctor", "a doctor action ran", true
	}
	if subject == "agent.repair" {
		switch strings.TrimSpace(strPayloadMap(payload, "phase")) {
		case "requested":
			return "doctor", "an operator repair was requested", true
		case "completed":
			return "doctor", "an operator repair completed", true
		case "failed":
			return "doctor", "an operator repair failed", true
		default:
			return "doctor", "an operator repair ran", true
		}
	}
	if subject == "agent.wake" {
		switch strings.TrimSpace(strPayloadMap(payload, "phase")) {
		case "requested":
			return "doctor", "an operator wake was requested", true
		case "completed":
			return "doctor", "an operator wake completed", true
		case "failed":
			return "doctor", "an operator wake failed", true
		default:
			return "doctor", "an operator wake ran", true
		}
	}
	if subject == "agent.resolve" {
		switch strings.TrimSpace(strPayloadMap(payload, "phase")) {
		case "requested":
			return "doctor", "an operator resolution was requested", true
		case "completed":
			return "doctor", "an operator resolution completed", true
		case "failed":
			return "doctor", "an operator resolution failed", true
		default:
			return "doctor", "an operator resolution ran", true
		}
	}
	return "", "", false
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

func autonomyDoctorDetail(subject string, p map[string]any) string {
	str := func(k string) string {
		return strPayload(p, k)
	}
	forceGenSuffix := func() string {
		gen := intPayload(p, "routing_force_generation")
		if gen > 1 {
			return fmt.Sprintf(" · gen %d", gen)
		}
		return ""
	}
	if subject == "doctor.auto_repair" {
		agent := str("agent")
		mode := strings.TrimSpace(str("mode"))
		phase := strings.TrimSpace(str("phase"))
		reason := clipDetail(str("reason"))
		prefix := agent
		if prefix == "" {
			prefix = "agent"
		}
		switch phase {
		case "routing_force_exhausted_detected":
			taskType := str("routing_task_type")
			if taskType != "" {
				return prefix + " · forced chain exhausted for " + taskType + forceGenSuffix()
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · forced chain exhausted" + forceGenSuffix()
		case "routing_forced_failed_detected":
			taskType := str("routing_task_type")
			if taskType != "" {
				return prefix + " · forced chain failed for " + taskType + forceGenSuffix()
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · forced chain failed after probation" + forceGenSuffix()
		case "routing_unstable_detected":
			taskType := str("routing_task_type")
			if taskType != "" {
				return prefix + " · unstable routing detected for " + taskType
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · unstable routing detected"
		case "attempts_exhausted":
			attempt := intPayload(p, "self_repair_attempt")
			maxAttempts := intPayload(p, "self_repair_max_attempts")
			if attempt > 0 && maxAttempts > 0 {
				return fmt.Sprintf("%s · self-repair exhausted %d/%d", prefix, attempt, maxAttempts)
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · self-repair exhausted"
		case "queued":
			if mode == "degraded" {
				if reason != "" {
					return prefix + " · " + reason
				}
				return prefix + " · doctor queued"
			}
			if mode == "routing" {
				if taskType := str("routing_task_type"); taskType != "" {
					if chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → "); chain != "" {
						return prefix + " · routing queued for " + taskType + " → " + chain
					}
					return prefix + " · routing queued for " + taskType
				}
				if reason != "" {
					return prefix + " · " + reason
				}
				return prefix + " · routing repair queued"
			}
			if issues := strSlicePayload(p, "issues"); len(issues) > 0 {
				return prefix + " · " + clipDetail(issues[0])
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · repair queued"
		case "routing_rollback_queued":
			taskType := str("routing_task_type")
			chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → ")
			if taskType != "" && chain != "" {
				return prefix + " · rollback queued for " + taskType + " → " + clipDetail(chain)
			}
			if taskType != "" {
				return prefix + " · rollback queued for " + taskType
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · routing rollback queued"
		case "completed":
			if mode == "routing" {
				taskType := str("routing_task_type")
				chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → ")
				if taskType != "" && chain != "" {
					return prefix + " · rewrote " + taskType + " chain to " + clipDetail(chain)
				}
				if taskType != "" {
					return prefix + " · rewrote " + taskType + " routing chain"
				}
				if applied := strSlicePayload(p, "applied"); len(applied) > 0 {
					return prefix + " · applied " + strings.Join(applied, ", ")
				}
				return prefix + " · routing stabilized"
			}
			if applied := strSlicePayload(p, "applied"); len(applied) > 0 {
				return prefix + " · applied " + strings.Join(applied, ", ")
			}
			return prefix + " · repaired"
		case "routing_rollback_completed":
			taskType := str("routing_task_type")
			chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → ")
			if taskType != "" && chain != "" {
				return prefix + " · rolled back " + taskType + " chain to " + clipDetail(chain)
			}
			if taskType != "" {
				return prefix + " · rolled back " + taskType + " chain"
			}
			return prefix + " · routing rollback completed"
		case "failed":
			if err := clipDetail(str("error")); err != "" {
				return prefix + " · " + err
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · repair failed"
		case "routing_rollback_failed":
			if err := clipDetail(str("error")); err != "" {
				return prefix + " · rollback failed: " + err
			}
			if reason != "" {
				return prefix + " · rollback failed: " + reason
			}
			return prefix + " · routing rollback failed"
		case "escalation_woke":
			if target := str("target_agent"); target != "" {
				return prefix + " · woke " + target
			}
			return prefix + " · owner wake launched"
		case "escalation_answered":
			if res := strings.TrimSpace(str("resolution")); res != "" {
				if target := str("target_agent"); target != "" {
					if delegated := str("delegate_to"); delegated != "" && res == "delegated" {
						if summary := clipDetail(str("resolution_summary")); summary != "" {
							return prefix + " · delegated by " + target + " to " + delegated + ": " + summary
						}
						return prefix + " · delegated by " + target + " to " + delegated
					}
					if res == "force_chain" {
						taskType := str("routing_task_type")
						chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → ")
						if taskType != "" && chain != "" {
							return prefix + " · forced " + taskType + " chain by " + target + " to " + clipDetail(chain) + forceGenSuffix()
						}
						if summary := clipDetail(str("resolution_summary")); summary != "" {
							return prefix + " · force_chain by " + target + ": " + summary + forceGenSuffix()
						}
						return prefix + " · force_chain by " + target + forceGenSuffix()
					}
					if summary := clipDetail(str("resolution_summary")); summary != "" {
						return prefix + " · " + res + " by " + target + ": " + summary
					}
					return prefix + " · " + res + " by " + target
				}
				if summary := clipDetail(str("resolution_summary")); summary != "" {
					return prefix + " · " + res + ": " + summary
				}
			}
			if target := str("target_agent"); target != "" {
				return prefix + " · answered by " + target
			}
			return prefix + " · owner answered escalation"
		case "resolution_applied":
			res := strings.TrimSpace(str("resolution"))
			target := str("target_agent")
			if res == "force_chain" {
				taskType := str("routing_task_type")
				chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → ")
				if taskType != "" && chain != "" {
					if target != "" {
						return prefix + " · applied forced " + taskType + " chain by " + target + " to " + clipDetail(chain) + forceGenSuffix()
					}
					return prefix + " · applied forced " + taskType + " chain to " + clipDetail(chain) + forceGenSuffix()
				}
			}
			if res != "" {
				if summary := clipDetail(str("resolution_summary")); summary != "" {
					if target != "" {
						return prefix + " · applied " + res + " by " + target + ": " + summary
					}
					return prefix + " · applied " + res + ": " + summary
				}
				if target != "" {
					return prefix + " · applied " + res + " by " + target
				}
				return prefix + " · applied " + res
			}
			return prefix + " · owner resolution applied"
		case "escalation_skipped":
			if reason != "" {
				return prefix + " · " + reason
			}
			if target := str("target_agent"); target != "" {
				return prefix + " · skipped waking " + target
			}
			return prefix + " · owner wake skipped"
		case "escalation_failed":
			if target := str("target_agent"); target != "" {
				if reason != "" {
					return prefix + " · wake " + target + " failed: " + reason
				}
				return prefix + " · wake " + target + " failed"
			}
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · owner wake failed"
		case "resolution_failed":
			if res := str("resolution"); res != "" {
				if reason != "" {
					return prefix + " · " + res + " failed: " + reason
				}
				return prefix + " · " + res + " failed"
			}
			if reason != "" {
				return prefix + " · resolution follow-up failed: " + reason
			}
			return prefix + " · resolution follow-up failed"
		case "delegation_queued":
			if target := str("delegate_to"); target != "" {
				if by := str("delegated_by"); by != "" {
					return prefix + " · delegated by " + by + " to " + target
				}
				return prefix + " · delegation queued to " + target
			}
			return prefix + " · delegation queued"
		case "delegation_woke":
			if target := str("delegate_to"); target != "" {
				return prefix + " · woke delegated agent " + target
			}
			return prefix + " · woke delegated agent"
		case "delegation_failed":
			if target := str("delegate_to"); target != "" {
				if reason != "" {
					return prefix + " · delegated wake " + target + " failed: " + reason
				}
				return prefix + " · delegated wake " + target + " failed"
			}
			if reason != "" {
				return prefix + " · delegated wake failed: " + reason
			}
			return prefix + " · delegated wake failed"
		default:
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix
		}
	}
	if subject == "agent.repair" || subject == "agent.wake" || subject == "agent.resolve" {
		agent := str("agent")
		phase := strings.TrimSpace(str("phase"))
		reason := clipDetail(str("reason"))
		prefix := agent
		if prefix == "" {
			prefix = "agent"
		}
		noun := "operator action"
		if subject == "agent.repair" {
			noun = "operator repair"
		} else if subject == "agent.wake" {
			noun = "operator wake"
		} else if subject == "agent.resolve" {
			noun = "operator resolution"
		}
		switch phase {
		case "requested":
			if reason != "" {
				return prefix + " · " + noun + " requested: " + reason
			}
			if subject == "agent.wake" {
				if intent := clipDetail(str("intent")); intent != "" {
					return prefix + " · " + intent
				}
			}
			return prefix + " · " + noun + " requested"
		case "completed":
			if res := str("resolution"); res != "" {
				switch res {
				case "force_chain":
					if taskType := str("routing_task_type"); taskType != "" {
						if chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → "); chain != "" {
							return prefix + " · operator forced " + taskType + " chain to " + clipDetail(chain)
						}
						return prefix + " · operator forced " + taskType + " routing chain"
					}
				case "delegated":
					if to := str("delegate_to"); to != "" {
						return prefix + " · operator delegated to " + to
					}
				case "paused", "retired":
					return prefix + " · operator " + res
				}
			}
			if taskType := str("routing_task_type"); taskType != "" {
				if chain := strings.Join(strSlicePayload(p, "routing_task_model_chain"), " → "); chain != "" {
					return prefix + " · rewrote " + taskType + " chain to " + clipDetail(chain)
				}
				return prefix + " · rewrote " + taskType + " routing chain"
			}
			if applied := strSlicePayload(p, "applied"); len(applied) > 0 {
				return prefix + " · applied " + strings.Join(applied, ", ")
			}
			if answer := clipDetail(str("answer")); answer != "" {
				return prefix + " · " + answer
			}
			return prefix + " · " + noun + " completed"
		case "failed":
			if err := clipDetail(str("error")); err != "" {
				return prefix + " · " + err
			}
			if reason != "" {
				return prefix + " · " + noun + " failed: " + reason
			}
			return prefix + " · " + noun + " failed"
		default:
			if reason != "" {
				return prefix + " · " + reason
			}
			return prefix + " · " + noun
		}
	}
	return ""
}

func strPayloadMap(payload []byte, k string) string {
	if len(payload) == 0 {
		return ""
	}
	var p map[string]any
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return strPayload(p, k)
}

func strPayload(p map[string]any, k string) string {
	if v, ok := p[k].(string); ok {
		return v
	}
	return ""
}

func strSlicePayload(p map[string]any, k string) []string {
	raw, ok := p[k].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func intPayload(p map[string]any, k string) int {
	switch n := p[k].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func clipDetail(s string) string {
	const max = 120
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}
