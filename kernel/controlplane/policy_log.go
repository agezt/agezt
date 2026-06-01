// SPDX-License-Identifier: MIT

package controlplane

// Policy-decision audit log (M63) — a read-only view of the journal's
// policy.decision events (the agent loop journals one per tool-call gating:
// tool, capability, allow/deny, reason, hard_denied). `agt edict show` displays
// the loaded RULES; this shows the DECISIONS those rules produced, so an
// operator can audit "what got allowed/denied recently?".

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleEdictLog(conn net.Conn, req Request) {
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	deniedOnly, _ := req.Args["denied"].(bool)

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type decision struct {
		ts                int64
		seq               int64
		actor, corr       string
		tool, capability  string
		reason            string
		allow, hardDenied bool
	}
	decisions := make([]decision, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision {
			return nil
		}
		d := decodePolicyDecision(e.Payload)
		if deniedOnly && d.allow {
			return nil
		}
		decisions = append(decisions, decision{
			ts: e.TSUnixMS, seq: e.Seq, actor: e.Actor, corr: e.CorrelationID,
			tool: d.tool, capability: d.capability, reason: d.reason,
			allow: d.allow, hardDenied: d.hardDenied,
		})
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].ts != decisions[j].ts {
			return decisions[i].ts > decisions[j].ts
		}
		return decisions[i].seq > decisions[j].seq
	})
	if len(decisions) > limit {
		decisions = decisions[:limit]
	}

	out := make([]map[string]any, 0, len(decisions))
	for _, d := range decisions {
		out = append(out, map[string]any{
			"ts_unix_ms":     d.ts,
			"actor":          d.actor,
			"correlation_id": d.corr,
			"tool":           d.tool,
			"capability":     d.capability,
			"allow":          d.allow,
			"reason":         d.reason,
			"hard_denied":    d.hardDenied,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"decisions": out, "count": len(out)},
	})
}

type policyDecisionPayload struct {
	tool, capability, reason string
	allow, hardDenied        bool
}

// decodePolicyDecision pulls the fields the agent loop writes onto a
// policy.decision event (M63). Returns zero values on parse failure.
func decodePolicyDecision(payload json.RawMessage) policyDecisionPayload {
	if len(payload) == 0 {
		return policyDecisionPayload{}
	}
	var p struct {
		Tool       string `json:"tool"`
		Capability string `json:"capability"`
		Reason     string `json:"reason"`
		Allow      bool   `json:"allow"`
		HardDenied bool   `json:"hard_denied"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return policyDecisionPayload{}
	}
	return policyDecisionPayload{
		tool: p.Tool, capability: p.Capability, reason: p.Reason,
		allow: p.Allow, hardDenied: p.HardDenied,
	}
}
