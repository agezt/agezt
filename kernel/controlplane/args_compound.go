package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane request-arg
//             string-collection helper (argStringList).
//             Extracted from args.go during Day 211 god-file refactor (#99). Public
//             API unchanged.

import (
	"fmt"
	"strings"
)

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
