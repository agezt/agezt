package controlplane

// Provenance: SPDX-License-Identifier: MIT kernel/controlplane request-arg
//             specialised helpers (argLimit). Extracted from
//             args.go during Day 211 god-file refactor (#99). Public API unchanged.

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
