// SPDX-License-Identifier: MIT

package runs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The readers below are the run command's strict argument contract: a flag
// that arrives with the wrong JSON type is a usage error, never a silent
// default. The messages are the ones the native run handler used, so the CLI
// keeps reporting the same text.

// argString reads a present string argument; the value is returned as sent.
func argString(raw json.RawMessage, key string) (string, bool, error) {
	if len(raw) == 0 {
		return "", false, nil
	}
	s, ok := rawValue(raw).(string)
	if !ok {
		return "", true, fmt.Errorf("args.%s must be a string", key)
	}
	return s, true, nil
}

// argInt64 reads a present numeric argument, truncated toward zero.
func argInt64(raw json.RawMessage, key string) (int64, bool, error) {
	if len(raw) == 0 {
		return 0, false, nil
	}
	f, ok := rawValue(raw).(float64)
	if !ok {
		return 0, true, fmt.Errorf("args.%s must be a number", key)
	}
	return int64(f), true, nil
}

// argBool reads a present boolean argument. A string "true" is a type error
// here: a mistyped dry_run must not silently execute the run.
func argBool(raw json.RawMessage, key string) (bool, bool, error) {
	if len(raw) == 0 {
		return false, false, nil
	}
	b, ok := rawValue(raw).(bool)
	if !ok {
		return false, true, fmt.Errorf("args.%s must be a boolean", key)
	}
	return b, true, nil
}

// argStringList reads a present array of strings; blank entries are dropped.
// A present-but-not-an-array value is an error, not a silent empty list.
func argStringList(raw json.RawMessage, key string) ([]string, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	list, ok := rawValue(raw).([]any)
	if !ok {
		return nil, true, fmt.Errorf("args.%s must be an array", key)
	}
	out := make([]string, 0, len(list))
	for i, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, true, fmt.Errorf("args.%s[%d] must be a string", key, i)
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out, true, nil
}

// argText reads a present argument as text; a non-string reads as empty.
func argText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	s, _ := rawValue(raw).(string)
	return s
}

// argTruthy reads a lenient truthy flag: a boolean, or the strings "on",
// "true", "yes" and "1".
func argTruthy(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	switch t := rawValue(raw).(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "on", "true", "yes", "1":
			return true
		}
	}
	return false
}

// parseCapList parses an auto-approve grant — a comma/space-separated string
// OR a JSON array of strings — into a capability set.
func parseCapList(raw json.RawMessage) map[string]bool {
	if len(raw) == 0 {
		return nil
	}
	add := func(out map[string]bool, s string) {
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
			if f = strings.TrimSpace(f); f != "" {
				out[f] = true
			}
		}
	}
	out := map[string]bool{}
	switch t := rawValue(raw).(type) {
	case string:
		add(out, t)
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				add(out, s)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
