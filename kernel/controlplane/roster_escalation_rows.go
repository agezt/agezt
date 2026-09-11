// SPDX-License-Identifier: MIT

package controlplane

// Agent escalation row helpers (M846): the journal-derived per-agent
// escalation row view consumed by handleAgentEscalations. Carved out of
// roster.go during the Day 24 god file split #9.

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) agentEscalationRows(st *board.Store, slug string, limit int, cursorTS int64, cursorID string) ([]agentEscalationRow, string) {
	msgs := st.Read("help", boardReadMaxLimit)
	metaByMessage := map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if plString(pl, "target_agent") != slug {
			return nil
		}
		msgID := plString(pl, "mailbox_message_id")
		if strings.TrimSpace(msgID) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			Agent:                          plString(pl, "agent"),
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Error:                          plString(pl, "error"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := metaByMessage[msgID]; !ok || row.Seq > cur.Seq {
			metaByMessage[msgID] = row
		}
		return nil
	})
	out := make([]agentEscalationRow, 0, len(msgs))
	for _, msg := range msgs {
		if !msg.Help {
			continue
		}
		if msg.To != slug && msg.To != board.Everyone {
			continue
		}
		replies := st.Replies(msg.ID, boardReadMaxLimit)
		acked := boardMessageAckedBy(msg, slug)
		status := "open"
		if len(replies) > 0 {
			status = "answered"
		} else if acked {
			status = "acked"
		}
		row := agentEscalationRow{
			MessageID:  msg.ID,
			From:       msg.From,
			To:         msg.To,
			Text:       msg.Text,
			TSUnixMS:   msg.TSMS,
			Status:     status,
			ReplyCount: len(replies),
			Acked:      acked,
		}
		if meta, ok := metaByMessage[msg.ID]; ok {
			row.SourceAgent = meta.Agent
			row.Mode = meta.Mode
			row.WakePhase = meta.Phase
			row.WakeReason = meta.Reason
			row.WakeError = meta.Error
			row.WakeCorrelationID = meta.TargetCorr
			row.Fingerprint = meta.Fingerprint
			row.Resolution = meta.Resolution
			row.ResolutionSummary = meta.ResolutionSummary
			row.DelegateTo = meta.DelegateTo
			row.RootAgent = meta.RootAgent
			row.ChainDepth = meta.ChainDepth
			row.IncidentID = meta.IncidentID
			row.RootIncidentID = meta.RootIncidentID
			row.ParentIncidentID = meta.ParentIncidentID
			if strings.HasPrefix(meta.Phase, "delegation_") {
				row.OriginKind = "delegated"
				row.OriginAgent = firstNonEmpty(meta.DelegatedBy, msg.From)
			} else {
				row.OriginKind = "doctor"
				row.OriginAgent = firstNonEmpty(msg.From, meta.DelegatedBy)
			}
		}
		// Prefer parsing the broken/source agent out of the message text only when
		// the event envelope didn't carry one (older history).
		if row.SourceAgent == "" {
			row.SourceAgent = escalationSourceFromText(msg.Text)
		}
		if row.RootAgent == "" {
			row.RootAgent = row.SourceAgent
		}
		if row.OriginKind == "" {
			row.OriginKind = "doctor"
			row.OriginAgent = msg.From
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSUnixMS > out[j].TSUnixMS })
	// Cursor pagination: cursor encodes (TSUnixMS, MessageID) of the LAST
	// entry on the previous page; server skips entries strictly newer-or-equal.
	if cursorTS > 0 || cursorID != "" {
		filtered := out[:0]
		for _, r := range out {
			if r.TSUnixMS > cursorTS {
				continue
			}
			if r.TSUnixMS == cursorTS && r.MessageID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		out = filtered
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		return out, strconv.FormatInt(last.TSUnixMS, 10) + ":" + last.MessageID
	}
	return out, ""
}

func boardMessageAckedBy(m board.Message, slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	for _, by := range m.AckedBy {
		if strings.ToLower(strings.TrimSpace(by)) == slug {
			return true
		}
	}
	return false
}

func escalationSourceFromText(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "Doctor "
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	if i := strings.Index(text, " for agent "); i > 0 {
		// The agent named after "for agent" is the broken agent, not the owner.
		start := i + len(" for agent ")
		if end := strings.Index(text[start:], "."); end > 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	return ""
}

func joinActivityParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func wakeRunbookActivitySuffix(pl map[string]any) string {
	raw, _ := pl["autonomy_runbook"].(map[string]any)
	if len(raw) == 0 {
		return ""
	}
	parts := []string{
		plString(raw, "trigger_contract"),
		plString(raw, "route_contract"),
		plString(raw, "recovery_contract"),
		plString(raw, "sleep_contract"),
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return "contract " + strings.Join(clean, "/")
}

