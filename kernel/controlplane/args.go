package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane request-arg typed
//             primitives (argString). Extracted from args.go during Day 211 god-file
//             split (#99). Public API unchanged. The other readers (argBool,
//             argInt64, argStringList, parseCapList) moved to kernel/app/runs
//             with the run command they served.

import (
	"fmt"
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
