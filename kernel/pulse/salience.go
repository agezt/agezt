// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/state"
)

// salienceNS is the state namespace for the novelty seen-cache.
const salienceNS = "pulse_seen"

// Salience turns a stream of deltas into the few that matter (SPEC-03 §4) —
// the single most important component for being neither annoying nor useless.
// v1 is rules-first (severity + novelty suppression) with an optional cheap-
// LLM refine; the full world-model relevance/decay signals land with Memory.
type Salience struct {
	state      *state.FileStore
	provider   agent.Provider // optional; only used when useLLM
	model      string
	dial       Dial
	useLLM     bool
	noveltyTTL time.Duration
	now        func() time.Time
}

// Score weighs one delta into a value + reason + recommended disposition.
// Deterministic given (delta, seen-cache, clock); the optional LLM refine is
// skipped unless a provider is wired and useLLM is set, so tests stay stable.
func (s *Salience) Score(ctx context.Context, d Delta) Score {
	// Novelty suppression: if we've surfaced this same issue recently,
	// drop it (journal only) rather than ping again (SPEC-03 §4.2).
	if s.seenRecently(d.IssueKey()) {
		return Score{Value: 0.1, Reason: "already surfaced recently (novelty suppression)", Disposition: DispDrop}
	}

	value, disp := scoreFromSeverity(d.Severity())
	reason := "severity=" + string(d.Severity())

	if s.useLLM && s.provider != nil {
		if v, r, ok := s.refineWithLLM(ctx, d); ok {
			value = v
			disp = dispositionForValue(v)
			reason = "llm: " + r
		}
	}
	return Score{Value: value, Reason: reason, Disposition: disp}
}

// scoreFromSeverity is the deterministic rules baseline.
func scoreFromSeverity(sev Severity) (float64, Disposition) {
	switch sev {
	case SevCritical:
		return 0.95, DispAlert
	case SevHigh:
		return 0.80, DispAlert
	case SevLow:
		return 0.25, DispDigest
	default: // medium
		return 0.50, DispNotify
	}
}

// dispositionForValue maps an LLM 0..1 score back onto a disposition band.
func dispositionForValue(v float64) Disposition {
	switch {
	case v >= 0.85:
		return DispAlert
	case v >= 0.45:
		return DispNotify
	case v >= 0.20:
		return DispDigest
	default:
		return DispDrop
	}
}

// Route applies the user's dial and quiet-hours to a disposition, deciding
// what actually reaches the user (SPEC-03 §4.3, §6.3). Pure function.
func Route(dial Dial, disp Disposition, quietHoursActive bool) Delivery {
	var del Delivery
	switch disp {
	case DispAlert, DispAct:
		del = DeliverNow // breaks through every dial
	case DispNotify:
		switch dial {
		case DialQuiet:
			del = DeliverDigest
		default: // balanced, chatty
			del = DeliverNow
		}
	case DispDigest:
		switch dial {
		case DialQuiet:
			del = DeliverDrop
		case DialChatty:
			del = DeliverNow
		default: // balanced
			del = DeliverDigest
		}
	default: // drop
		del = DeliverDrop
	}

	// Quiet hours: only alert/act break through; everything else that would
	// have sent now is held for the digest instead.
	if quietHoursActive && del == DeliverNow && disp != DispAlert && disp != DispAct {
		del = DeliverDigest
	}
	return del
}

// seenEntry is the stored novelty record.
type seenEntry struct {
	LastMS int64 `json:"last_ms"`
}

func (s *Salience) seenRecently(issueKey string) bool {
	if s.state == nil {
		return false
	}
	raw, ok, err := s.state.Get(salienceNS, seenKey(issueKey))
	if err != nil || !ok {
		return false
	}
	var e seenEntry
	if json.Unmarshal(raw, &e) != nil {
		return false
	}
	age := s.now().UnixMilli() - e.LastMS
	return age >= 0 && age < s.noveltyTTL.Milliseconds()
}

// MarkSeen records that an issue was surfaced now, so a repeat within the TTL
// is suppressed. Called by the engine after a delivery (now or digest).
func (s *Salience) MarkSeen(issueKey string) {
	if s.state == nil {
		return
	}
	_ = s.state.Set(salienceNS, seenKey(issueKey), seenEntry{LastMS: s.now().UnixMilli()})
}

// seenKey sanitizes an issue key into a state key (namespaces/keys avoid path
// separators; the store validates namespaces but keys are freer — still keep
// it tidy).
func seenKey(issueKey string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(issueKey)
}

// --- optional LLM refine --------------------------------------------------

const salienceSystem = `You score how much a system change matters to the operator, 0.0 (ignore) to 1.0 (urgent). ` +
	`Return ONLY JSON: {"score":0.0,"reason":"..."}. Consider severity, blast radius, and whether action is needed.`

func (s *Salience) refineWithLLM(ctx context.Context, d Delta) (float64, string, bool) {
	user := "Change: " + d.Summary
	if d.Before != "" || d.After != "" {
		user += " (was: " + d.Before + " → now: " + d.After + ")"
	}
	resp, err := s.provider.Complete(ctx, agent.CompletionRequest{
		Model:    s.model,
		System:   salienceSystem,
		Messages: []agent.Message{{Role: agent.RoleUser, Content: user}},
		TaskType: "salience",
	})
	if err != nil {
		return 0, "", false
	}
	start := strings.IndexByte(resp.Message.Content, '{')
	end := strings.LastIndexByte(resp.Message.Content, '}')
	if start < 0 || end <= start {
		return 0, "", false
	}
	var out struct {
		Score  float64 `json:"score"`
		Reason string  `json:"reason"`
	}
	if json.Unmarshal([]byte(resp.Message.Content[start:end+1]), &out) != nil {
		return 0, "", false
	}
	if out.Score < 0 {
		out.Score = 0
	}
	if out.Score > 1 {
		out.Score = 1
	}
	return out.Score, out.Reason, true
}
