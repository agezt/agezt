// SPDX-License-Identifier: MIT

package controlplane

// Agent roster CRUD handlers (M783) — the management path behind `agt agent`.
// Lifecycle changes go through the kernel so every create/edit/pause/resume/
// remove is journaled (roster.*) and auditable via `agt why`. Profiles are
// addressed by ref = id OR slug everywhere, so operators can say
// `agt agent show researcher` without copying ULIDs.

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	"net"
)

type agentRepairRow = approster.RepairRow

type agentEscalationRow struct {
	MessageID         string
	From              string
	To                string
	Text              string
	TSUnixMS          int64
	Status            string
	ReplyCount        int
	Acked             bool
	SourceAgent       string
	Mode              string
	WakePhase         string
	WakeReason        string
	WakeError         string
	WakeCorrelationID string
	Fingerprint       string
	Resolution        string
	ResolutionSummary string
	DelegateTo        string
	OriginKind        string
	OriginAgent       string
	RootAgent         string
	ChainDepth        int
	IncidentID        string
	RootIncidentID    string
	ParentIncidentID  string
}

type agentRepairSummary = approster.RepairSummary

type agentRoutingPressure = approster.RoutingPressure

type agentRetryPressure = approster.RetryPressure

type agentEscalationLoad = approster.EscalationLoad

type agentWakeStatus = approster.WakeStatus

type agentLiveStatus = approster.LiveStatus

type agentLastActivity = approster.LastActivity

// agentStatusAccums holds the per-agent state that the journal-derivable
// helpers accumulate. Roster-agentList page is the single consumer; collecting
// every accumulator in a SINGLE journal.Range pass turns what used to be
// O(journalSize) per helper (and 11× that across all helpers) into one
// O(journalSize) walk. Large, busy journals were reliably tripping the
// control-plane connection's 10-minute read deadline under the previous
// 11-Range design (each Range does a callback-driven O(n) walk over every
// durable event, with JSON-unmarshal + map lookups per event). The
// single-pass dispatch below is behavior-preserving: each accumulator's
// key-set and "latest wins" semantics match the original per-helper
// implementations (the retired per-helper methods have been deleted; this
// dispatch is now the only journal-derived roster-status path).
func (s *Server) handleAgentImpact(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: s.agentImpactResult(p)})
}

// handleAgentTombstone returns a read-only death certificate for an agent: its
// identity, lifecycle/retirement record, and durable resource footprint. Portable
// archival/audit artifact — it removes and mutates nothing (NEXT.md #7).
func registerRosterCommands() {
	register(
		commandSpec{Cmd: CmdAgentAdd, Handler: func(dc *DispatchCtx) { dc.S.handleAgentAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEdit, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleAgentSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRemove, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTaskUpdate, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTaskUpdate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentImpact, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAgentImpact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTombstone, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTombstone(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepair, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepair(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEscalations, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEscalations(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentWake, Handler: func(dc *DispatchCtx) { dc.S.handleAgentWake(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentResolve, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleAgentResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRetire, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRetire(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRevive, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRevive(dc.Conn, dc.Req) }},
	)
}
