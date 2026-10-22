// SPDX-License-Identifier: MIT

package memory

import "strings"

// RetentionDecision explains whether a record belongs in long-term memory.
// Journal/log output belongs in the journal, not here.
type RetentionDecision struct {
	Keep   bool   `json:"keep"`
	Reason string `json:"reason,omitempty"`
}

func assessSpec(spec RememberSpec) RetentionDecision {
	return assessRetentionForActor(Record{
		Type:       spec.Type,
		Subject:    spec.Subject,
		Content:    spec.Content,
		Tags:       spec.Tags,
		Evidence:   spec.Evidence,
		HalfLifeMS: spec.HalfLifeMS,
	}, spec.Actor)
}

func shouldFilterSpec(spec RememberSpec) bool {
	source := ""
	if spec.Tags != nil {
		source = spec.Tags["source"]
	}
	switch source {
	case "agent", "distill", "brain-distill":
		return true
	}
	switch spec.Actor {
	case "agent", "distill":
		return true
	default:
		return false
	}
}

// AssessRetention is the conservative write/cleanup filter for long-term
// memory. It is intentionally inspectable and rule-based: durable user
// preferences, curated facts, and constraints pass; task logs and transient
// execution notes do not.
func AssessRetention(r Record) RetentionDecision {
	return assessRetention(r)
}

func assessRetention(r Record) RetentionDecision {
	return assessRetentionForActor(r, r.UpdatedBy)
}

func assessRetentionForActor(r Record, actor string) RetentionDecision {
	content := strings.TrimSpace(r.Content)
	if content == "" {
		return RetentionDecision{Keep: false, Reason: "empty"}
	}
	if isSystemAgentActor(actor) || isSystemAgentActor(r.AddedBy) || isSystemAgentActor(r.UpdatedBy) || isSystemAgentActor(scopeOf(r.Tags)) {
		if d := assessSystemAgentRetention(r, content); !d.Keep {
			return d
		}
	}
	if r.Evidence == EvidenceConstraint || r.Evidence == EvidenceCurated {
		return RetentionDecision{Keep: true}
	}
	if r.Type == TypePreference {
		return RetentionDecision{Keep: true}
	}
	if sourceOf(r.Tags) == "operator" {
		return RetentionDecision{Keep: true}
	}
	lower := strings.ToLower(content)
	if len([]rune(content)) < 8 {
		return RetentionDecision{Keep: false, Reason: "too_short"}
	}
	if looksLikeExecutionLog(lower) {
		return RetentionDecision{Keep: false, Reason: "execution_log"}
	}
	if looksTransient(lower) && r.HalfLifeMS <= 0 {
		return RetentionDecision{Keep: false, Reason: "transient_without_expiry"}
	}
	if r.Subject == "" && !hasDurablePredicate(lower) {
		return RetentionDecision{Keep: false, Reason: "no_subject_or_durable_predicate"}
	}
	return RetentionDecision{Keep: true}
}

func assessSystemAgentRetention(r Record, content string) RetentionDecision {
	switch r.Type {
	case TypeObservation, TypeSummary:
		return RetentionDecision{Keep: false, Reason: "system_agent_log_type"}
	}
	lower := strings.ToLower(content)
	if looksLikeSystemAgentLog(lower) || looksLikeExecutionLog(lower) || looksTransient(lower) {
		return RetentionDecision{Keep: false, Reason: "system_agent_log_output"}
	}
	return RetentionDecision{Keep: true}
}

func isSystemAgentActor(actor string) bool {
	a := strings.ToLower(strings.TrimSpace(actor))
	if a == "" {
		return false
	}
	return strings.HasPrefix(a, "guardian-") ||
		strings.HasSuffix(a, "-sentinel") ||
		strings.Contains(a, "health-sentinel") ||
		strings.Contains(a, "system-sentinel")
}

func sourceOf(tags map[string]string) string {
	if tags == nil {
		return ""
	}
	return tags["source"]
}

func looksLikeExecutionLog(s string) bool {
	needles := []string{
		"stdout", "stderr", "exit code", "tool result", "command output",
		"i ran", "we ran", "ran command", "executed ", "completed successfully",
		"go test ./", "npm test", "cargo test", "pytest", "ls -", "dir ",
		"no relevant memory", "nothing to remember", "stored memory", "remember failed",
		"memory: low-value record rejected",
		"observation:", "summary:", "tool output", "run output", "log output",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func looksLikeSystemAgentLog(s string) bool {
	needles := []string{
		"health sweep", "system-health", "daemon healthy", "all healthy",
		"no action", "no issues", "green", "scan complete", "sweep complete",
		"recent runs", "active runs", "provider fallback", "rate limited",
		"budget exceeded", "journal stalled", "introspect", "overseer",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func looksTransient(s string) bool {
	needles := []string{
		"today", "right now", "currently", "this run", "this task",
		"temporary", "tmp", "just did", "next step",
	}
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func hasDurablePredicate(s string) bool {
	needles := []string{
		" is ", " are ", " uses ", " use ", " requires ", " needs ",
		" prefers ", " likes ", " dislikes ", " default ", " must ",
		" should ", " lives ", " located ", " owns ", " supports ",
		" stores ", " deploys ", " runs ", " belongs ", " configured ",
	}
	for _, n := range needles {
		if strings.Contains(" "+s+" ", n) {
			return true
		}
	}
	return false
}
