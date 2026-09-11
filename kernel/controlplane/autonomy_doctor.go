// SPDX-License-Identifier: MIT

// Doctor event detail renderer: autonomyDoctorDetail.
// Code extracted from autonomy.go during the Day-50 god-file split. Public API unchanged.
package controlplane


import (
	"fmt"
	"strings"
)


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
