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
// argTruthy reports whether a control-plane arg is set to a truthy value,
// accepting either a real bool (the canonical form) or a common string form
// ("on"/"true"/"yes"/"1") for clients that send the toggle as text. Anything
// else, including absent, is false. Lenient by design — it gates an opt-in
// convenience (trust-this-run), never a security-relevant default.
func argTruthy(v any) bool {
	switch t := v.(type) {
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

// requiredArgString extracts a string arg that must be present and non-empty —
// the id/name-shaped inputs nearly every mutating handler starts with. The
// error reads exactly like the hand-written "args.<key> required" messages it
// replaces, so client-visible wording is unchanged.
func requiredArgString(args map[string]any, key string) (string, error) {
	v, _, err := argString(args, key)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return v, nil
}

// argFloat64 extracts a numeric arg (JSON numbers decode to float64).
func argFloat64(args map[string]any, key string) (float64, bool, error) {
	v, present := args[key]
	if !present {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, true, fmt.Errorf("args.%s must be a number", key)
	}
	return f, true, nil
}

// argStringMap extracts a JSON object whose values are strings. A non-string
// value is an error rather than silently dropped (the previous inline form
// skipped them, so a mistyped tag value vanished without a trace).
func argStringMap(args map[string]any, key string) (map[string]string, bool, error) {
	v, present := args[key]
	if !present {
		return nil, false, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("args.%s must be an object", key)
	}
	out := make(map[string]string, len(m))
	for k, e := range m {
		s, ok := e.(string)
		if !ok {
			return nil, true, fmt.Errorf("args.%s.%s must be a string", key, k)
		}
		out[k] = s
	}
	return out, true, nil
}

// argDryRun reads the confirm-first dry_run toggle shared by the destructive
// maintenance commands (prune/tidy/clean/collect): ABSENT defaults to true
// (preview — the safe direction), an explicit bool is honored, and the string
// forms "false"/"0" mean false (CLI clients send the flag as text). Any other
// present value is an error — a mistyped dry_run that silently fell through to
// a default is exactly the typo class the typed accessors exist to surface.
func argDryRun(args map[string]any) (bool, error) {
	v, present := args["dry_run"]
	if !present {
		return true, nil
	}
	switch t := v.(type) {
	case bool:
		return t, nil
	case string:
		return !(t == "false" || t == "0"), nil
	default:
		return true, fmt.Errorf("args.dry_run must be a boolean")
	}
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
