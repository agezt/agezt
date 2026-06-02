// SPDX-License-Identifier: MIT

package controlplane

import (
	"fmt"
	"strings"
)

// Typed accessors for request args (decoded JSON, so values are string / bool /
// float64 / []any / map[string]any). Each distinguishes three cases:
//   - absent      → ok=false, err=nil  (caller uses its default)
//   - present, OK → ok=true,  err=nil
//   - present, wrong type → ok=true, err!=nil  (a client-side mistake the caller
//     should REPORT, not silently swallow — a mistyped `dry_run` that fell through
//     to its zero value would execute a run the operator meant to only preview)
//
// The previous inline `v, _ := args[k].(T)` form collapsed "absent" and "wrong
// type" into the same zero value, turning typos into silent wrong behavior.

// argString extracts a string arg. The value is returned verbatim (not trimmed);
// callers trim as needed.
func argString(args map[string]any, key string) (string, bool, error) {
	v, present := args[key]
	if !present {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, fmt.Errorf("args.%s must be a string", key)
	}
	return s, true, nil
}

// argBool extracts a boolean arg.
func argBool(args map[string]any, key string) (bool, bool, error) {
	v, present := args[key]
	if !present {
		return false, false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, true, fmt.Errorf("args.%s must be a boolean", key)
	}
	return b, true, nil
}

// argInt64 extracts an integer arg. JSON numbers decode to float64, so that's the
// accepted form (an integer-valued float); a non-numeric present value is an error.
func argInt64(args map[string]any, key string) (int64, bool, error) {
	v, present := args[key]
	if !present {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, true, fmt.Errorf("args.%s must be a number", key)
	}
	return int64(f), true, nil
}

// argStringList extracts a JSON array of strings, trimming each element and
// skipping empties. A present-but-non-array value is an error (not silently an
// empty list — for `tools` that would scope the run to NO tools). A non-string
// element is likewise an error rather than silently dropped.
func argStringList(args map[string]any, key string) ([]string, bool, error) {
	v, present := args[key]
	if !present {
		return nil, false, nil
	}
	list, ok := v.([]any)
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
