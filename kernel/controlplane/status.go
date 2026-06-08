// SPDX-License-Identifier: MIT

package controlplane

// Daemon health overview. Operators (and CI smoke tests) want a
// single round-trip that answers "is the daemon up, what version
// is it, and what's it doing right now?" — instead of dialing
// CmdVersion + CmdBudget + CmdToolList separately and reconciling
// the results client-side.
//
// Read-only. No mutation, no side effects on the kernel.

import (
	"encoding/json"
	"net"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/event"
)

// countProviderFallbacks folds the journal for provider.fallback events (M280):
// how many times the governor fell back from a primary provider to a backup
// because the primary errored, plus the most recent reason. Until this was
// surfaced, a misconfigured/incompatible provider could silently serve every run
// from the mock fallback (the M279 dotted-tool-name bug was invisible this way).
func (s *Server) countProviderFallbacks() (int, string) {
	count := 0
	last := ""
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindProviderFallback {
			return nil
		}
		count++
		var p struct {
			Reason string `json:"reason"`
			Failed string `json:"failed"`
		}
		if json.Unmarshal(e.Payload, &p) == nil {
			if r := p.Reason; r != "" {
				last = strutil.Ellipsis(r, 160, "…")
			}
		}
		return nil
	})
	return count, last
}

func (s *Server) handleStatus(conn net.Conn, req Request) {
	headSeq, _ := s.k.Journal().Head() // (seq, hash); hash unused here
	// Journal.Head returns -1 on an empty journal (nextSeq-1), which
	// is a leaky implementation detail for an operator dashboard.
	// Clamp to 0 so `agt status` renders "head seq=0" on a fresh
	// install rather than the confusing "-1".
	if headSeq < 0 {
		headSeq = 0
	}

	uptime := time.Since(s.k.StartTime())
	// Integer seconds — sub-second precision adds noise without
	// helping any operator workflow. Floor (not round) so a 0.9s
	// uptime renders as 0, matching "just started".
	uptimeSecs := int64(uptime / time.Second)

	// Delegation governance (M49): surface the active depth / fan-out / spend
	// ceilings (M46–M48) so an operator can see what's in effect — they were
	// silent until a delegation tripped one. 0 fan-out / spend = unbounded.
	dl := s.k.SubAgentLimits()

	// Autonomy + actionable signals (M130): how many scheduled intents are armed
	// (and how many enabled), and how many HITL approvals are waiting on the
	// operator right now. Both are cheap in-memory reads. Scheduled autonomy and
	// a blocking approval queue were invisible in the at-a-glance status until now.
	schedTotal, schedEnabled := 0, 0
	if sched := s.k.Schedules(); sched != nil {
		for _, e := range sched.List() {
			schedTotal++
			if e.Enabled {
				schedEnabled++
			}
		}
	}
	pendingApprovals := 0
	if ap := s.k.Approvals(); ap != nil {
		pendingApprovals = ap.PendingCount()
	}

	// Provider fallbacks (M280): make silent primary→backup fallbacks visible so
	// a provider that errors on every request (and gets masked by the always-on
	// mock fallback) is caught at a glance instead of via a journal dig.
	fbCount, fbLast := s.countProviderFallbacks()

	result := map[string]any{
		"daemon":         brand.Version,
		"protocol":       brand.ProtocolVersion,
		"model":          s.k.Model(),
		"uptime_seconds": uptimeSecs,
		"halted":         s.k.IsHalted(),
		"active_runs":    s.k.ActiveRuns(),
		"tools":          len(s.k.Tools()),
		"memory_records": s.k.Memory().Count(),
		"world_entities": s.k.World().Count(),
		"active_skills":  s.k.Forge().Count(),
		"journal_head":   headSeq,
		"schedules": map[string]any{
			"total":   schedTotal,
			"enabled": schedEnabled,
		},
		"pending_approvals":  pendingApprovals,
		"provider_fallbacks": map[string]any{"count": fbCount, "last_reason": fbLast},
		"delegation": map[string]any{
			"enabled":              dl.Enabled,
			"max_depth":            dl.MaxDepth,
			"max_fanout":           dl.MaxFanout,
			"max_spend_microcents": dl.MaxSpendMicrocents,
			"max_total":            dl.MaxTotal,
		},
	}
	// Tenant count only when multi-tenancy is enabled (M130) — a single-tenant
	// daemon shouldn't show a tenant line at all.
	if s.tenants != nil {
		result["tenants"] = s.tenants.Count()
	}
	// Network-exposed HTTP servers (M137) — so `agt status` / the doctor exposure
	// check can flag a non-loopback bind (the agent reachable beyond localhost).
	if len(s.httpBindings) > 0 {
		servers := make([]map[string]any, 0, len(s.httpBindings))
		for _, b := range s.httpBindings {
			servers = append(servers, map[string]any{
				"name": b.Name, "addr": b.Addr, "loopback": b.Loopback,
			})
		}
		result["http_servers"] = servers
	}

	// Configured messaging channels (M141) — Telegram / Slack / Discord. So an
	// operator can confirm what's listening (and on what addr / allowlist) from the
	// status dashboard rather than the boot banner. Omitted when none configured.
	if len(s.channels) > 0 {
		chans := make([]map[string]any, 0, len(s.channels))
		for _, c := range s.channels {
			chans = append(chans, map[string]any{
				"kind": c.Kind, "inbound": c.Inbound, "addr": c.Addr, "allowlist": c.Allowlist,
			})
		}
		result["channels"] = chans
	}

	// AWS credential chain (M307): which keyless/ambient layer engaged (IRSA,
	// SSO, assume-role, IMDS). So an operator on EKS can confirm IRSA is live
	// from `agt status` instead of grepping the boot banner. Omitted when AWS
	// credentials aren't configured.
	if s.credChain != "" {
		result["cred_chain"] = s.credChain
	}

	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}
