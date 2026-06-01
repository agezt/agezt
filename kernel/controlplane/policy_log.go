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
	"time"

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
	toolFilter, _ := req.Args["tool"].(string)      // M74: scope to one tool
	capFilter, _ := req.Args["capability"].(string) // M74: scope to one capability
	cutoff := sinceCutoff(req.Args["since_ms"])     // M65: optional time window

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
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		d := decodePolicyDecision(e.Payload)
		if deniedOnly && d.allow {
			return nil
		}
		if toolFilter != "" && d.tool != toolFilter {
			return nil
		}
		if capFilter != "" && d.capability != capFilter {
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

// sinceCutoff converts an optional since_ms request arg into an absolute
// cutoff timestamp (M65): now − since_ms. Returns 0 when absent/zero, meaning
// "no window / all-time". Shared by the windowed log + stats folds so they apply
// the same clock (the server's, which also stamps event TSUnixMS).
func sinceCutoff(arg any) int64 {
	var sinceMS int64
	switch v := arg.(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}
	if sinceMS <= 0 {
		return 0
	}
	return time.Now().UnixMilli() - sinceMS
}

// handleEdictStats aggregates policy decisions (M64) — total/allowed/denied/
// hard-denied, denial rate, and a denied-by-capability breakdown. Optional
// since_ms windows by decision time. Tenant-scoped via kernelFor.
func (s *Server) handleEdictStats(conn net.Conn, req Request) {
	sinceMS := int64(0)
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}
	var cutoff int64
	if sinceMS > 0 {
		cutoff = time.Now().UnixMilli() - sinceMS
	}
	toolFilter, _ := req.Args["tool"].(string)      // M76: scope to one tool
	capFilter, _ := req.Args["capability"].(string) // M76: scope to one capability

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var total, allowed, denied, hardDenied int
	deniedByCap := map[string]int{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyDecision {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		d := decodePolicyDecision(e.Payload)
		if toolFilter != "" && d.tool != toolFilter {
			return nil
		}
		if capFilter != "" && d.capability != capFilter {
			return nil
		}
		total++
		if d.allow {
			allowed++
			return nil
		}
		denied++
		if d.hardDenied {
			hardDenied++
		}
		capName := d.capability
		if capName == "" {
			capName = "unknown"
		}
		deniedByCap[capName]++
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	denialRate := 0.0
	if total > 0 {
		denialRate = float64(denied) / float64(total)
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":                total,
			"allowed":              allowed,
			"denied":               denied,
			"hard_denied":          hardDenied,
			"denial_rate":          denialRate,
			"denied_by_capability": deniedByCap,
			"window_ms":            sinceMS,
		},
	})
}
