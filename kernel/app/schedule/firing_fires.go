// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"sort"
	"strings"
)

func (s *FiringService) Fires(_ context.Context, in FiresInput) (FiresOutput, error) {
	limit := in.Limit
	if limit < 1 {
		limit = 1
	}
	if limit > 1000 {
		limit = 1000
	}
	idFilter, statusFilter := in.ID, in.Status
	intentQuery := strings.ToLower(in.Intent)
	cutoff := in.CutoffMS
	cursorMS, cursorSeq, cursorOK := in.CursorMS, in.CursorSeq, in.CursorOK
	// Run outcomes, keyed by correlation — the same fold `agt runs` uses, so a
	// firing's status/duration/spend/answer never disagrees between the two views.
	runs, err := s.runs()
	if err != nil {
		return FiresOutput{}, err
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
	if err := s.journal.Range(func(e *event.Event) error {
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
					st = firingRunStatus(r)
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
		return FiresOutput{}, err
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

	out := make([]FireRecord, 0, len(fires))
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
		row := FireRecord{
			CorrelationID: f.corr,
			ScheduleID:    f.schedID, // M55: which schedule fired ("" for pre-M55 firings)
			FiredUnixMS:   f.firedMS,
			Intent:        f.intent,
			Model:         f.model,
			Target:        f.target,
			Agent:         f.agent,
			Workflow:      f.workflow,
			SystemTask:    f.systemTask,
			Tool:          f.tool,
			Executor:      f.executor,
			Category:      f.category,
			EffectClass:   f.effectClass,
			UsesLLM:       f.usesLLM,
			Action:        f.action,
			Status:        status,
			Reason:        reason,
			DurationMS:    duration,
			SpentMC:       spent,
			AnswerPreview: preview,
		}
		if len(f.autonomyRunbook) > 0 {
			row.AutonomyRunbook = f.autonomyRunbook
		}
		out = append(out, row)
	}

	var nextCursor string // A2: page past the last (oldest) emitted row when the page is full
	if n := len(fires); n > 0 {
		nextCursor = journal.NextCursor(fires[n-1].firedMS, fires[n-1].seq, n, limit)
	}
	return FiresOutput{Fires: out, Count: len(out), NextCursor: nextCursor}, nil
}
