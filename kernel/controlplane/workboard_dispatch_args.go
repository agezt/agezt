// SPDX-License-Identifier: MIT

package controlplane

// Workboard arg-parsing + retry helpers: retryPolicyFromArgs +
// retryDecisionView + workboardCorr + intArgAllowZero +
// workboardStringSliceArg. Carved out of workboard_dispatch.go
// during the Day 195 god-file split so the main file can stay
// focused on the dispatch + watch internals and the register file
// can stay focused on registerWorkboardCommands.
// Public API unchanged.

import (
	"strings"

	"github.com/agezt/agezt/kernel/workboard"
)

func retryPolicyFromArgs(args map[string]any) *workboard.RetryPolicy {
	if _, ok := args["max_attempts"]; !ok && stringArg(args, "escalate_to") == "" {
		return nil
	}
	return &workboard.RetryPolicy{
		MaxAttempts: intArgAllowZero(args["max_attempts"]),
		EscalateTo:  stringArg(args, "escalate_to"),
	}
}

func retryDecisionView(d workboard.RetryDecision) map[string]any {
	out := map[string]any{
		"action":        d.Action,
		"failure_count": d.FailureCount,
		"retry":         d.Retry,
		"exhausted":     d.Exhausted,
	}
	if d.MaxAttempts > 0 {
		out["max_attempts"] = d.MaxAttempts
	}
	if d.NextAttempt > 0 {
		out["next_attempt"] = d.NextAttempt
	}
	if d.EscalateTo != "" {
		out["escalate_to"] = d.EscalateTo
	}
	if d.Reason != "" {
		out["reason"] = d.Reason
	}
	return out
}

func workboardCorr(s *Server, req Request) string {
	if corr := stringArg(req.Args, "correlation_id"); corr != "" {
		return corr
	}
	return s.k.NewCorrelation()
}

func intArgAllowZero(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func workboardStringSliceArg(raw any) []string {
	switch xs := raw.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, raw := range xs {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return strings.Split(s, ",")
		}
		return nil
	}
}

// registerWorkboardCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
