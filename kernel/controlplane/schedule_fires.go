// SPDX-License-Identifier: MIT

// Control-plane schedule-fires handler + payload helpers.
// Code extracted from schedule_fires.go during the Day-86 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime"
)



// scheduleLastFiring is the most-recent firing of a schedule and its outcome
// (M56), used to annotate `agt schedule list` rows with how each schedule last
// went — not just what's scheduled.
type scheduleLastFiring struct {
	correlation string
	firedMS     int64
	status      string
	reason      string
}

// latestFiringBySchedule folds the journal into a schedule_id → most-recent
// firing map (M56), joining each firing's correlation with its run outcome from
// the shared collectRuns fold. Firings with no schedule_id (pre-M55) are skipped
// — they can't be attributed to a schedule entry.
func (s *Server) latestFiringBySchedule(k *runtime.Kernel) (map[string]scheduleLastFiring, error) {
	runs, err := s.collectRuns(k)
	if err != nil {
		return nil, err
	}
	latest := map[string]scheduleLastFiring{}
	err = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindScheduleFired {
			return nil
		}
		id := extractScheduleFired(e.Payload).ScheduleID
		if id == "" {
			return nil
		}
		if cur, ok := latest[id]; ok && cur.firedMS >= e.TSUnixMS {
			return nil // already have a newer (or same-ms) firing for this schedule
		}
		lf := scheduleLastFiring{correlation: e.CorrelationID, firedMS: e.TSUnixMS, status: "running"}
		if r, ok := runs[e.CorrelationID]; ok {
			switch {
			case r.Completed:
				lf.status = "completed"
			case r.Failed:
				lf.status = "failed"
				lf.reason = r.FailReason
			case r.Abandoned:
				lf.status = "abandoned"
			}
		}
		latest[id] = lf
		return nil
	})
	if err != nil {
		return nil, err
	}
	return latest, nil
}

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
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"]) // A2 cursor pagination
	// Optional filters: id (M55) scopes to one schedule; status (M61)
	// completed|failed|running|abandoned; intent substring (M80) is a
	// case-insensitive contains, mirroring `agt runs list --intent` (M77).
	sa, err := argStrings(req.Args, "id", "status", "intent")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	idFilter, statusFilter := sa["id"], sa["status"]
	intentQuery := strings.ToLower(sa["intent"])
	// Optional time window (M65): only firings at/after now − since_ms.
	cutoff := sinceCutoff(req.Args["since_ms"])

	// Tenant-scoped via the M39 seam: an empty tenant reads the primary journal.
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Run outcomes, keyed by correlation — the same fold `agt runs` uses, so a
	// firing's status/duration/spend/answer never disagrees between the two views.
	runs, err := s.collectRuns(k)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	type fired struct {
		corr, schedID, intent, model string
		target, agent                string
		workflow, systemTask, tool   string
		executor, category           string
		effectClass                  string
		autonomyRunbook              map[string]any
		usesLLM                      bool
		action                       string
		firedMS, seq                 int64
	}
	fires := make([]fired, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindScheduleFired {
			sf := extractScheduleFired(e.Payload)
			if idFilter != "" && sf.ScheduleID != idFilter {
				return nil // M55: filtered to a single schedule
			}
			if cutoff > 0 && e.TSUnixMS < cutoff {
				return nil // M65: outside the time window
			}
			if intentQuery != "" && !strings.Contains(strings.ToLower(sf.Intent), intentQuery) {
				return nil // M80: intent substring filter
			}
			if statusFilter != "" {
				// Status filter (M61): match the firing's run outcome. Applied
				// before sort/limit so `fires 5 --failed` returns 5 failed firings.
				st := "running"
				if r, ok := runs[e.CorrelationID]; ok {
					st = runEntryStatus(r)
				}
				if st != statusFilter {
					return nil
				}
			}
			fires = append(fires, fired{
				corr:            e.CorrelationID,
				schedID:         sf.ScheduleID,
				intent:          sf.Intent,
				model:           sf.Model,
				target:          sf.Target,
				agent:           sf.Agent,
				workflow:        sf.Workflow,
				systemTask:      sf.SystemTask,
				tool:            sf.Tool,
				executor:        scheduleFiredExecutor(sf),
				category:        scheduleFiredCategory(sf),
				effectClass:     scheduleFiredEffectClass(sf),
				autonomyRunbook: sf.AutonomyRunbook,
				usesLLM:         scheduleFiredUsesLLM(sf),
				action:          scheduleFiredAction(sf),
				firedMS:         e.TSUnixMS,
				seq:             e.Seq,
			})
		}
		return nil
	}); err != nil {
		s.fail(conn, req, err)
		return
	}

	// Newest firing first; seq breaks a same-millisecond tie.
	sort.Slice(fires, func(i, j int) bool {
		if fires[i].firedMS != fires[j].firedMS {
			return fires[i].firedMS > fires[j].firedMS
		}
		return fires[i].seq > fires[j].seq
	})
	if cursorOK { // A2: keep rows strictly older than the cursor, before the limit
		kept := fires[:0]
		for _, f := range fires {
			if journal.KeepBeforeCursor(f.firedMS, f.seq, cursorMS, cursorSeq) {
				kept = append(kept, f)
			}
		}
		fires = kept
	}
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
		row := map[string]any{
			"correlation_id": f.corr,
			"schedule_id":    f.schedID, // M55: which schedule fired ("" for pre-M55 firings)
			"fired_unix_ms":  f.firedMS,
			"intent":         f.intent,
			"model":          f.model,
			"target":         f.target,
			"agent":          f.agent,
			"workflow":       f.workflow,
			"system_task":    f.systemTask,
			"tool":           f.tool,
			"executor":       f.executor,
			"category":       f.category,
			"effect_class":   f.effectClass,
			"uses_llm":       f.usesLLM,
			"action":         f.action,
			"status":         status,
			"reason":         reason,
			"duration_ms":    duration,
			"spent_mc":       spent,
			"answer_preview": preview,
		}
		if len(f.autonomyRunbook) > 0 {
			row["autonomy_runbook"] = f.autonomyRunbook
		}
		out = append(out, row)
	}

	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(fires); n > 0 {
		nextCursor = journal.NextCursor(fires[n-1].firedMS, fires[n-1].seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"fires": out, "count": len(out), "next_cursor": nextCursor},
	})
}

