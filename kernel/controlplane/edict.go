// SPDX-License-Identifier: MIT
//
// Edict control-plane surface: edictFor + handleEdictShow + handleEdictTest
// + askPolicyLabel + registerEdictCommands (the dispatch registration for
// ALL edict handlers, including those in edict_deny.go / edict_set.go).
// The hard-deny handlers live in edict_deny.go; the runtime setters live
// in edict_set.go.
// Extracted from edict.go during the Day-204 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/runtime"
)

// edictFor resolves the Edict engine (and owning kernel, for journaling to
// the right bus) for a request's optional "tenant" arg: empty → the primary
// kernel, else the named tenant's isolated engine (M22). On error it writes
// the response and returns ok=false, so callers just `if !ok { return }`.
// Every `agt edict` command routes through here, giving per-tenant policy
// management for free across show/test/deny/level/mode.
func (s *Server) edictFor(conn net.Conn, req Request) (*edict.Engine, *runtime.Kernel, bool) {
	tenantID, _, err := argString(req.Args, "tenant")
	if err != nil {
		s.fail(conn, req, err)
		return nil, nil, false
	}
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.fail(conn, req, err)
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
	capRaw := req.Args["capability"]
	capStr, _ := capRaw.(string)
	if capStr == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.capability required"})
		return
	}
	inputRaw := req.Args["input"]
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

// askPolicyLabel maps the AskPolicy enum to the operator-facing strings
// AGEZT_APPROVAL_MODE accepts — now a thin alias over AskPolicy.String().
func askPolicyLabel(p edict.AskPolicy) string { return p.String() }

// registerEdictCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerEdictCommands() {
	register(
		commandSpec{Cmd: CmdEdictShow, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictTest, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictTest(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictDenyList, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictDenyList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictDenyAdd, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictDenyAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictDenyRemove, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictDenyRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictSetLevel, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictSetLevel(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictSetMode, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictSetMode(dc.Conn, dc.Req) }},
	)
}
