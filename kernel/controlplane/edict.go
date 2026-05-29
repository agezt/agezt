// SPDX-License-Identifier: MIT

package controlplane

// Edict policy snapshot handler. Read-only — surfaces what the
// engine actually loaded so operators can confirm their config
// (env vars, custom HardDeny rules) took effect. The handler
// never returns sensitive data: capability names + level names +
// hard-deny substrings are all operator-supplied to begin with.

import (
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/edict"
)

func (s *Server) handleEdictShow(conn net.Conn, req Request) {
	eng := s.k.Edict()
	levels := eng.Levels()

	// Sort capabilities for deterministic output — operators
	// reading `agt edict show` repeatedly shouldn't see the row
	// order flicker between calls.
	caps := make([]string, 0, len(levels))
	for c := range levels {
		caps = append(caps, string(c))
	}
	sort.Strings(caps)
	levelRows := make(map[string]any, len(caps))
	for _, c := range caps {
		levelRows[c] = levels[edict.Capability(c)].String()
	}

	rules := eng.HardDenyRules()
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Name < rules[j].Name
	})
	hardRows := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		var applies []any
		for _, c := range r.AppliesTo {
			applies = append(applies, string(c))
		}
		hardRows = append(hardRows, map[string]any{
			"name":       r.Name,
			"substring":  r.Substring,
			"applies_to": applies, // nil → JSON null = "applies to every capability"
		})
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"ask_policy": askPolicyLabel(eng.AskPolicy()),
			"levels":     levelRows,
			"hard_deny":  hardRows,
		},
	})
}

// askPolicyLabel maps the AskPolicy enum to the operator-facing
// strings AGEZT_APPROVAL_MODE accepts. Kept here (vs adding
// String() on the enum) because the daemon-facing env var
// vocabulary belongs to the daemon, not the kernel policy engine.
func askPolicyLabel(p edict.AskPolicy) string {
	switch p {
	case edict.AskAllow:
		return "allow"
	case edict.AskDeny:
		return "deny"
	case edict.AskPrompt:
		return "prompt"
	default:
		return "unknown"
	}
}
