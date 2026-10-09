package controlplane

// Provenance: SPDX-License-Identifier: MIT Edict control-plane surface: edictFor +
//             handleEdictShow + handleEdictTest + askPolicyLabel +
//             registerEdictCommands (the dispatch registration for ALL edict
//             handlers, including those in edict_deny.go / edict_set.go). The
//             hard-deny handlers live in edict_deny.go; the runtime setters live in
//             edict_set.go. Extracted from edict.go during the Day-204 god-file
//             split. Public API unchanged.

import (
	"net"

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

// askPolicyLabel maps the AskPolicy enum to the operator-facing strings
// AGEZT_APPROVAL_MODE accepts — now a thin alias over AskPolicy.String().
func askPolicyLabel(p edict.AskPolicy) string { return p.String() }

// registerEdictCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerEdictCommands() {
	register(
		commandSpec{Cmd: CmdEdictDenyAdd, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictDenyAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictDenyRemove, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictDenyRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictSetLevel, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictSetLevel(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdEdictSetMode, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleEdictSetMode(dc.Conn, dc.Req) }},
	)
}
