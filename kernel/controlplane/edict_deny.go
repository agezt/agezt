// SPDX-License-Identifier: MIT
//
// Edict hard-deny rule handlers: denyRuleRows + handleEdictDenyList +
// handleEdictDenyAdd + handleEdictDenyRemove.
// Extracted from edict.go during the Day-204 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
)

// denyRuleRows serializes the engine's current hard-deny set into the
// JSON shape shared by handleEdictDenyList and the add/remove responses,
// sorted by name and tagged with whether each rule is runtime-removable.
func denyRuleRows(rules []edict.HardDenyRule) []map[string]any {
	sort.Slice(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	rows := make([]map[string]any, 0, len(rules))
	for _, r := range rules {
		var applies []any
		for _, c := range r.AppliesTo {
			applies = append(applies, string(c))
		}
		rows = append(rows, map[string]any{
			"name":       r.Name,
			"substring":  r.Substring,
			"applies_to": applies, // nil → JSON null = "every capability"
			"removable":  edict.IsRuntimeRule(r.Name),
		})
	}
	return rows
}

// handleEdictDenyList returns the hard-deny rules with a `removable` flag
// so the operator can see at a glance which rules are the immutable floor
// (built-ins + AGEZT_EDICT_DENY) versus runtime-added.
func (s *Server) handleEdictDenyList(conn net.Conn, req Request) {
	eng, _, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	rules := eng.HardDenyRules()
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"rules": denyRuleRows(rules)},
	})
}

// handleEdictDenyAdd parses a single rule in AGEZT_EDICT_DENY syntax,
// appends it to the engine, and journals a policy.changed event. A change
// to the deny floor is itself security-relevant, so it lands in the same
// hash-chained journal as the decisions it will govern.
func (s *Server) handleEdictDenyAdd(conn net.Conn, req Request) {
	spec, _, err := argString(req.Args, "rule")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	parsed, err := edict.ParseDenyRules(spec)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if len(parsed) != 1 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError,
			Error: "args.rule must specify exactly one deny rule (no ';' separators)"})
		return
	}
	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	added, err := eng.AddHardDeny(parsed[0])
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	applies := make([]string, 0, len(added.AppliesTo))
	for _, c := range added.AppliesTo {
		applies = append(applies, string(c))
	}
	count := len(eng.HardDenyRules())
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "kernel.policy",
		Kind:    event.KindPolicyChanged,
		Actor:   "operator",
		Payload: map[string]any{
			"action":     "deny.add",
			"name":       added.Name,
			"substring":  added.Substring,
			"applies_to": applies,
			"count":      count,
		},
	})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"name":       added.Name,
			"substring":  added.Substring,
			"applies_to": applies,
			"count":      count,
		},
	})
}

// handleEdictDenyRemove removes a runtime-added rule by name and journals
// the change. Removing a floor rule is refused by the engine and surfaced
// as an error here, never a silent success.
func (s *Server) handleEdictDenyRemove(conn net.Conn, req Request) {
	name, err := requiredArgString(req.Args, "name")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	removed, err := eng.RemoveHardDeny(name)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	count := len(eng.HardDenyRules())
	if removed {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "kernel.policy",
			Kind:    event.KindPolicyChanged,
			Actor:   "operator",
			Payload: map[string]any{
				"action": "deny.rm",
				"name":   name,
				"count":  count,
			},
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"removed": removed, "count": count},
	})
}
