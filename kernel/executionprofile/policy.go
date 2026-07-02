// SPDX-License-Identifier: MIT

package executionprofile

import (
	"os"
	"strings"
)

type ProfilePolicy struct {
	Allow map[string]bool
	Deny  map[string]bool
}

func PolicyFromEnv() ProfilePolicy {
	return ParseProfilePolicy(os.Getenv("AGEZT_EXEC_PROFILE_ALLOW"), os.Getenv("AGEZT_EXEC_PROFILE_DENY"))
}

func ParseProfilePolicy(allowRaw, denyRaw string) ProfilePolicy {
	return ProfilePolicy{
		Allow: profileSet(allowRaw),
		Deny:  profileSet(denyRaw),
	}
}

func (p ProfilePolicy) Allows(id string) (bool, string) {
	id = normalizeProfileID(id)
	if id == "" {
		return true, ""
	}
	if p.Deny["*"] || p.Deny[id] {
		return false, "denied by AGEZT_EXEC_PROFILE_DENY"
	}
	if len(p.Allow) > 0 && !p.Allow["*"] && !p.Allow[id] {
		return false, "not listed in AGEZT_EXEC_PROFILE_ALLOW"
	}
	return true, ""
}

func (p ProfilePolicy) Empty() bool {
	return len(p.Allow) == 0 && len(p.Deny) == 0
}

func profileSet(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	}) {
		id := normalizeProfileID(part)
		if id == "" {
			continue
		}
		if id == "all" {
			id = "*"
		}
		out[id] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeProfileID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func RoutableRunProfileIDsForPolicy(inv Inventory, policy ProfilePolicy) []string {
	ids := RoutableRunProfileIDsFor(inv)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if ok, _ := policy.Allows(id); ok {
			out = append(out, id)
		}
	}
	return out
}
