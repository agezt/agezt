// SPDX-License-Identifier: MIT
//
// kernel/controlplane request-arg string-collection helpers (argStrings, argStringMap,
// argStringList).
// Extracted from args.go during Day 211 god-file refactor (#99).
// Public API unchanged.
package controlplane

import (
	"fmt"
	"strings"
)

func argStrings(args map[string]any, keys ...string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		v, _, err := argString(args, k)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}
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
