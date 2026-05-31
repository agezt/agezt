// SPDX-License-Identifier: MIT

package controlplane

// Edict policy snapshot handler. Read-only — surfaces what the
// engine actually loaded so operators can confirm their config
// (env vars, custom HardDeny rules) took effect. The handler
// never returns sensitive data: capability names + level names +
// hard-deny substrings are all operator-supplied to begin with.

import (
	"net"
	"slices"
	"sort"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

// edictFor resolves the Edict engine (and owning kernel, for journaling to
// the right bus) for a request's optional "tenant" arg: empty → the primary
// kernel, else the named tenant's isolated engine (M22). On error it writes
// the response and returns ok=false, so callers just `if !ok { return }`.
// Every `agt edict` command routes through here, giving per-tenant policy
// management for free across show/test/deny/level/mode.
func (s *Server) edictFor(conn net.Conn, req Request) (*edict.Engine, *runtime.Kernel, bool) {
	tenantID, _ := req.Args["tenant"].(string)
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return nil, nil, false
	}
	return k.Edict(), k, true
}

func (s *Server) handleEdictShow(conn net.Conn, req Request) {
	eng, _, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
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

// handleEdictTest dry-runs a policy decision. The Outcome shape
// flattens onto the same JSON the runtime journals — operators
// who've gotten used to reading policy.decision events get the
// same vocabulary here.
func (s *Server) handleEdictTest(conn net.Conn, req Request) {
	capRaw, _ := req.Args["capability"]
	capStr, _ := capRaw.(string)
	if capStr == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.capability required"})
		return
	}
	inputRaw, _ := req.Args["input"]
	input, _ := inputRaw.(string) // empty string is a valid probe

	eng, _, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	out := eng.Decide(edict.Capability(capStr), input)
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"decision":          string(out.Decision),
			"capability":        string(out.Capability),
			"level":             out.Level.String(),
			"reason":            out.Reason,
			"hard_denied":       out.HardDenied,
			"hard_deny_rule":    out.HardDenyRule,
			"would_ask":         out.WouldAsk,
			"requires_approval": out.RequiresApproval,
		},
	})
}

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
	spec, _ := req.Args["rule"].(string)
	parsed, err := edict.ParseDenyRules(spec)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	name, _ := req.Args["name"].(string)
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	removed, err := eng.RemoveHardDeny(name)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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

// handleEdictSetLevel changes a capability's trust level at runtime and
// journals a policy.changed event. The capability must be known (a typo
// would otherwise create a default-deny phantom entry); the level string
// is parsed leniently (L0..L4 or aliases). The previous level is captured
// for the event + response so the change is fully reconstructable.
func (s *Server) handleEdictSetLevel(conn net.Conn, req Request) {
	capStr, _ := req.Args["capability"].(string)
	if capStr == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.capability required"})
		return
	}
	cap := edict.Capability(capStr)
	if !slices.Contains(edict.AllCapabilities(), cap) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError,
			Error: "unknown capability " + capStr + " (see `edict show` for the governed set)"})
		return
	}
	levelStr, _ := req.Args["level"].(string)
	lvl, err := edict.ParseTrustLevel(levelStr)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	from := "unset"
	if prev, ok := eng.Level(cap); ok {
		from = prev.String()
	}
	eng.SetLevel(cap, lvl)
	to := lvl.String()

	_, _ = k.Bus().Publish(event.Spec{
		Subject: "kernel.policy",
		Kind:    event.KindPolicyChanged,
		Actor:   "operator",
		Payload: map[string]any{
			"action":     "level.set",
			"capability": capStr,
			"from":       from,
			"to":         to,
		},
	})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"capability": capStr,
			"from":       from,
			"to":         to,
		},
	})
}

// handleEdictSetMode changes the engine-wide approval mode at runtime and
// journals a policy.changed event. The mode string is parsed leniently
// (allow/deny/prompt); the previous mode is captured for the event +
// response so the change is fully reconstructable (and replayable, M20).
func (s *Server) handleEdictSetMode(conn net.Conn, req Request) {
	modeStr, _ := req.Args["mode"].(string)
	mode, err := edict.ParseAskPolicy(modeStr)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	from := eng.AskPolicy().String()
	eng.SetAskPolicy(mode)
	to := mode.String()

	_, _ = k.Bus().Publish(event.Spec{
		Subject: "kernel.policy",
		Kind:    event.KindPolicyChanged,
		Actor:   "operator",
		Payload: map[string]any{
			"action": "mode.set",
			"from":   from,
			"to":     to,
		},
	})
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"from": from, "to": to},
	})
}

// askPolicyLabel maps the AskPolicy enum to the operator-facing strings
// AGEZT_APPROVAL_MODE accepts — now a thin alias over AskPolicy.String().
func askPolicyLabel(p edict.AskPolicy) string { return p.String() }
