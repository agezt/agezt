// SPDX-License-Identifier: MIT
//
// kernel/controlplane request-arg typed primitives (argString, argTruthy, argBool,
// requiredArgString, argFloat64, argInt64).
// Extracted from args.go during Day 211 god-file refactor (#99).
// Public API unchanged.
package controlplane

import (
	"fmt"
	"strings"
)

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
func requiredArgString(args map[string]any, key string) (string, error) {
	v, _, err := argString(args, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(v) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return v, nil
}
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
