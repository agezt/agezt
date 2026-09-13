// SPDX-License-Identifier: MIT

package main

// agt edict shared helpers: extractTenantFlag + withTenant + joinCaps.
// Carved out of edict_deny.go during the Day 188 god-file split so the
// deny file can stay focused on the deny-list CRUD ops (List/Add/Remove)
// and the show-test file can stay focused on the read-side subcommands
// (Show/Test).
// Public API unchanged.

import (
	"strings"
)

func extractTenantFlag(args []string) (tenant string, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--tenant":
			if i+1 < len(args) {
				tenant = args[i+1]
				i++ // consume the value
			}
		case strings.HasPrefix(a, "--tenant="):
			tenant = a[len("--tenant="):]
		default:
			rest = append(rest, a)
		}
	}
	return tenant, rest
}

// withTenant adds the tenant id to a control-plane args map when non-empty
// (empty routes to the primary kernel server-side). Tolerates a nil map.
func withTenant(tenant string, m map[string]any) map[string]any {
	if tenant == "" {
		return m
	}
	if m == nil {
		m = map[string]any{}
	}
	m["tenant"] = tenant
	return m
}

// joinCaps formats a capability list for the human renderer. Kept
// as a tiny helper so the test suite doesn't have to pull strings
// just for this.
func joinCaps(caps []string) string {
	if len(caps) == 0 {
		return ""
	}
	out := caps[0]
	for _, c := range caps[1:] {
		out += ", " + c
	}
	return out
}

