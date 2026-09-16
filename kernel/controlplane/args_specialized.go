// SPDX-License-Identifier: MIT
//
// kernel/controlplane request-arg specialised helpers (argLimit, argDryRun, argFlag).
// Extracted from args.go during Day 211 god-file refactor (#99).
// Public API unchanged.
package controlplane

import (
	"fmt"
	"strings"
)

func argLimit(args map[string]any, def, max int) (int, error) {
	f, _, err := argFloat64(args, "limit")
	if err != nil {
		return 0, err
	}
	limit := def
	if f > 0 {
		limit = int(f)
	}
	if limit > max {
		limit = max
	}
	return limit, nil
}
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
func argFlag(args map[string]any, key string) (bool, bool, error) {
	v, present := args[key]
	if !present {
		return false, false, nil
	}
	switch t := v.(type) {
	case bool:
		return t, true, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes":
			return true, true, nil
		case "false", "0", "no":
			return false, true, nil
		}
	}
	return false, true, fmt.Errorf("args.%s must be a boolean", key)
}
