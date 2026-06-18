// SPDX-License-Identifier: MIT

package runtime

import (
	"strconv"
	"strings"
	"time"
)

// agentConfigIssue describes one malformed runtime override on an agent
// profile. Unknown AGEZT_* keys are allowed as generic config overlays; only
// the runtime knobs this process actually consumes are validated here.
type agentConfigIssue struct {
	Key   string
	Value string
	Issue string
}

var knownAgentRuntimeOverrideKeys = []string{
	"AGEZT_MODEL",
	"AGEZT_MAX_ITER",
	"AGEZT_MAX_AUTO_CONTINUE",
	"AGEZT_AUTO_CONTINUE_WAIT",
	"AGEZT_PARALLEL_TOOLS",
	"AGEZT_TOOL_DISCOVERY_MAX",
	"AGEZT_CONTEXT_BUDGET",
	"AGEZT_OBSERVATION_DELTAS",
	"AGEZT_DISABLE_HEURISTIC_BYPASS",
}

func agentConfigOverrideRaw(overrides map[string]string, key string) (string, bool) {
	if overrides == nil {
		return "", false
	}
	raw, ok := overrides[strings.TrimSpace(strings.ToUpper(key))]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(raw), true
}

func agentConfigStringValue(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	return raw, true
}

func agentConfigBoolValue(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on", "enabled":
		return true, true
	case "0", "false", "no", "off", "disabled":
		return false, true
	default:
		return false, false
	}
}

func agentConfigIntValue(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return n, true
}

func agentConfigDurationValue(raw string) (time.Duration, bool) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return d, true
}

func agentRuntimeConfigIssues(overrides map[string]string) []agentConfigIssue {
	var out []agentConfigIssue
	for _, key := range knownAgentRuntimeOverrideKeys {
		raw, ok := agentConfigOverrideRaw(overrides, key)
		if !ok {
			continue
		}
		switch key {
		case "AGEZT_MODEL":
			if _, ok := agentConfigStringValue(raw); !ok {
				out = append(out, agentConfigIssue{Key: key, Value: raw, Issue: "value is blank"})
			}
		case "AGEZT_MAX_ITER", "AGEZT_MAX_AUTO_CONTINUE", "AGEZT_PARALLEL_TOOLS", "AGEZT_TOOL_DISCOVERY_MAX", "AGEZT_CONTEXT_BUDGET":
			if _, ok := agentConfigIntValue(raw); !ok {
				out = append(out, agentConfigIssue{Key: key, Value: raw, Issue: "must be an integer"})
			}
		case "AGEZT_AUTO_CONTINUE_WAIT":
			if _, ok := agentConfigDurationValue(raw); !ok {
				out = append(out, agentConfigIssue{Key: key, Value: raw, Issue: "must be a Go duration like 250ms, 2s, or 1m30s"})
			}
		case "AGEZT_OBSERVATION_DELTAS", "AGEZT_DISABLE_HEURISTIC_BYPASS":
			if _, ok := agentConfigBoolValue(raw); !ok {
				out = append(out, agentConfigIssue{Key: key, Value: raw, Issue: "must be a boolean like true/false or 1/0"})
			}
		}
	}
	return out
}
