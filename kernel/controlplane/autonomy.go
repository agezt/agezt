// SPDX-License-Identifier: MIT

// Autonomy feed core: constants + autonomyKinds map + autonomyMeta + autonomyDoctorMeta.
// Code extracted from autonomy.go during the Day-50 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
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
	event.KindSkillRestored:    {"skill", "a skill checkpoint was restored"},
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