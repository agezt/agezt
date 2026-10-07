// SPDX-License-Identifier: MIT

package controlplane

// Shared legacy seat string-list admission.
// Workboard and OKR native admission now live in their app services.

import (
	"strings"
)

func workboardStringSliceArg(raw any) []string {
	switch xs := raw.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, raw := range xs {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
			return strings.Split(s, ",")
		}
		return nil
	}
}
