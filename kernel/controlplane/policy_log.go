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
	"time"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleEdictLog(conn net.Conn, req Request) {
	deniedOnly, _ := req.Args["denied"].(bool)
	toolFilter, _ := req.Args["tool"].(string)      // M74: scope to one tool
	capFilter, _ := req.Args["capability"].(string) // M74: scope to one capability
	s.projectJournal(conn, req, "decisions", func(e *event.Event) (map[string]any, bool) {
		if e.Kind != event.KindPolicyDecision {
			return nil, false
		}
		d := decodePolicyDecision(e.Payload)
		if deniedOnly && d.allow {
			return nil, false
		}
		if toolFilter != "" && d.tool != toolFilter {
			return nil, false
		}
		if capFilter != "" && d.capability != capFilter {
			return nil, false
		}
		return map[string]any{
			"actor":          e.Actor,
			"correlation_id": e.CorrelationID,
			"tool":           d.tool,
			"capability":     d.capability,
			"allow":          d.allow,
			"reason":         d.reason,
			"hard_denied":    d.hardDenied,
		}, true
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
