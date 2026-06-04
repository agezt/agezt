// SPDX-License-Identifier: MIT

package controlplane

// System changelog (SPEC-08 §4.2, M133). A filtered projection of the journal as
// a human-readable, tamper-evident timeline of what actually changed about THIS
// system — not raw events (that's `journal tail`), but the material lifecycle
// moments an operator cares about: the system was halted/resumed, policy changed,
// a skill was promoted/quarantined/reverted, a reflection completed, the catalog
// synced, pulse was paused/resumed. Each entry carries its event id so `agt why`
// can prove and explain it. Read-only; tenant-routed for a future `--tenant`.
//
// Only event kinds that actually exist are folded. Plugin/migration/core-update
// entries from the spec's list are added when those features land and emit their
// events.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

// changelogKinds maps each material-change event kind to a stable human label.
// Membership in this map is what makes an event part of the system changelog.
var changelogKinds = map[event.Kind]string{
	event.KindHalt:                      "system HALTED",
	event.KindAnomalyDetected:           "anomaly auto-halt",
	event.KindResume:                    "system resumed",
	event.KindPolicyChanged:             "policy changed",
	event.KindSkillCreated:              "skill created",
	event.KindSkillPromoted:             "skill promoted",
	event.KindSkillQuarantined:          "skill quarantined",
	event.KindSkillReverted:             "skill reverted",
	event.KindReflectionCompleted:       "reflection completed",
	event.KindCatalogSynced:             "model catalog synced",
	event.KindCatalogSyncFailed:         "model catalog sync FAILED",
	event.KindCatalogDiscoveryCompleted: "provider discovery completed",
	event.KindCatalogDiscoveryFailed:    "provider discovery FAILED",
	event.KindPulsePaused:               "pulse paused",
	event.KindPulseResumed:              "pulse resumed",
}

func (s *Server) handleChangelog(conn net.Conn, req Request) {
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		if n := int(int64Arg(raw)); n > 0 {
			limit = n
		}
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	cutoff := sinceCutoff(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type row struct {
		ts, seq                       int64
		kind, label, detail, id, corr string
	}
	rows := make([]row, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		label, ok := changelogKinds[e.Kind]
		if !ok {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		rows = append(rows, row{
			ts: e.TSUnixMS, seq: e.Seq, kind: string(e.Kind), label: label,
			detail: changelogDetail(e.Payload), id: e.ID, corr: e.CorrelationID,
		})
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ts != rows[j].ts {
			return rows[i].ts > rows[j].ts
		}
		return rows[i].seq > rows[j].seq
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}

	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"ts_unix_ms":     r.ts,
			"kind":           r.kind,
			"label":          r.label,
			"detail":         r.detail,
			"event_id":       r.id,
			"correlation_id": r.corr,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"entries": out, "count": len(out)},
	})
}

// changelogDetail pulls a short human detail out of an event payload by probing a
// few common keys, so the timeline reads meaningfully without brittle per-kind
// decoding. Returns "" when nothing useful is present.
func changelogDetail(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p map[string]any
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	for _, key := range []string{"summary", "name", "skill_id", "id", "rule", "change", "reason", "subject", "provider", "model", "count"} {
		if v, ok := p[key]; ok {
			switch s := v.(type) {
			case string:
				if s != "" {
					return s
				}
			case float64:
				return trimFloat(s)
			}
		}
	}
	return ""
}

// trimFloat renders a JSON number without a trailing ".0" for whole values.
func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return itoa(int64(f))
	}
	b, _ := json.Marshal(f)
	return string(b)
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
