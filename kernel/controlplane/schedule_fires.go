// SPDX-License-Identifier: MIT

package controlplane

// Scheduled-run firing history (M54) — the autonomy analogue of `agt runs list`.
// `agt schedule list` shows what's SCHEDULED; this shows what actually FIRED and
// how it turned out. A schedule firing journals a schedule.fired event carrying
// the run's correlation (cmd/agezt buildCadence), and the intent then runs
// through the normal governed loop — so each firing's outcome is exactly a
// runEntry. We walk the journal for schedule.fired events and join each with its
// run's outcome (status/duration/spend/answer) from the shared collectRuns fold.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleScheduleFires(conn net.Conn, req Request) {
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

	// Tenant-scoped via the M39 seam: an empty tenant reads the primary journal.
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	// Run outcomes, keyed by correlation — the same fold `agt runs` uses, so a
	// firing's status/duration/spend/answer never disagrees between the two views.
	runs, err := s.collectRuns(k)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	type fired struct {
		corr, intent, model string
		firedMS, seq        int64
	}
	fires := make([]fired, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindScheduleFired {
			intent, model := extractScheduleFired(e.Payload)
			fires = append(fires, fired{e.CorrelationID, intent, model, e.TSUnixMS, e.Seq})
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Newest firing first; seq breaks a same-millisecond tie.
	sort.Slice(fires, func(i, j int) bool {
		if fires[i].firedMS != fires[j].firedMS {
			return fires[i].firedMS > fires[j].firedMS
		}
		return fires[i].seq > fires[j].seq
	})
	if len(fires) > limit {
		fires = fires[:limit]
	}

	out := make([]map[string]any, 0, len(fires))
	for _, f := range fires {
		status := "running"
		reason := ""
		var duration, spent int64
		preview := ""
		// A firing whose run hasn't produced task events yet (or was trimmed)
		// stays "running" — same graceful degradation as `agt runs`.
		if r, ok := runs[f.corr]; ok {
			switch {
			case r.Completed:
				status = "completed"
				if r.StartedUnixMS > 0 {
					duration = r.CompletedUnixMS - r.StartedUnixMS
				}
			case r.Failed:
				status = "failed"
				reason = r.FailReason
				if r.StartedUnixMS > 0 && r.FailedUnixMS >= r.StartedUnixMS {
					duration = r.FailedUnixMS - r.StartedUnixMS
				}
			case r.Abandoned:
				status = "abandoned"
			}
			spent = r.SpentMicrocents
			preview = r.AnswerPreview
		}
		out = append(out, map[string]any{
			"correlation_id": f.corr,
			"fired_unix_ms":  f.firedMS,
			"intent":         f.intent,
			"model":          f.model,
			"status":         status,
			"reason":         reason,
			"duration_ms":    duration,
			"spent_mc":       spent,
			"answer_preview": preview,
		})
	}

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"fires": out, "count": len(out)},
	})
}

// extractScheduleFired pulls intent + model out of a schedule.fired payload
// (M54). Returns ("","") on parse failure so a malformed firing still lists with
// its correlation and outcome, just without the intent text.
func extractScheduleFired(payload json.RawMessage) (intent, model string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		Intent string `json:"intent"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.Intent, p.Model
}
