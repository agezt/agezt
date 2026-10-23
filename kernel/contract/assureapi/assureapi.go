// SPDX-License-Identifier: MIT

package assureapi

import (
	"encoding/json"
	"strings"
)

// DefaultMaxAttempts / MaxMaxAttempts bound the retry loop. The ceiling stops a
// pathological task (one a verifier never deems complete) from looping forever
// and burning budget.
const (
	DefaultMaxAttempts = 3
	MaxMaxAttempts     = 10
)

// Verdict is the completion judgement for one attempt.
type Verdict struct {
	// Complete reports whether the answer fully accomplishes the task.
	Complete bool `json:"complete"`
	// Gap describes what is still missing when not complete — fed back into the
	// next attempt so the agent knows exactly what to finish.
	Gap string `json:"gap,omitempty"`
}

// Attempt records one run+verify cycle.
type Attempt struct {
	N       int     `json:"n"`
	Answer  string  `json:"answer"`
	Verdict Verdict `json:"verdict"`
}

// Result is the outcome of an assured run.
type Result struct {
	// Answer is the last attempt's answer (the best the loop produced).
	Answer string `json:"answer"`
	// Complete reports whether a verdict deemed the task done within the budget.
	Complete bool `json:"complete"`
	// Attempts is how many run+verify cycles actually ran.
	Attempts int `json:"attempts"`
	// Gap is the last unmet gap when the loop exhausted without completing.
	Gap string `json:"gap,omitempty"`
	// History is the per-attempt trail (useful for `agt why` / debugging).
	History []Attempt `json:"history,omitempty"`
}

// ParseVerdict extracts a Verdict from a model reply that SHOULD be strict JSON
// {"complete":bool,"gap":"..."} but in practice may arrive wrapped in prose or a
// ```json fence. It isolates the first balanced-looking {...} span and unmarshals
// it. The bool is false when no JSON object could be parsed — the caller decides
// how to treat an unparseable verdict (the kernel treats it as "not complete" so
// the bounded loop tries again rather than declaring a false success).
func ParseVerdict(reply string) (Verdict, bool) {
	s := strings.TrimSpace(reply)
	// Drop a leading ```json / ``` fence and trailing fence if present.
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return Verdict{}, false
	}
	var v Verdict
	if err := json.Unmarshal([]byte(s[start:end+1]), &v); err != nil {
		return Verdict{}, false
	}
	return v, true
}
