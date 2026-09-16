// SPDX-License-Identifier: MIT

package runtime

import (
	"strconv"
	"strings"
	"time"
)

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
