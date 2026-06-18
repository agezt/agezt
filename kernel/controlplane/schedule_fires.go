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
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/event"
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
	// Optional filter (M55): only firings of this schedule id.
	idFilter, _ := req.Args["id"].(string)
	// Optional status filter (M61): completed|failed|running|abandoned.
	statusFilter, _ := req.Args["status"].(string)
	// Optional intent substring filter (M80): case-insensitive contains, mirroring
	// `agt runs list --intent` (M77) so the two list surfaces filter alike.
	intentQuery, _ := req.Args["intent"].(string)
	intentQuery = strings.ToLower(intentQuery)
	// Optional time window (M65): only firings at/after now − since_ms.
	cutoff := sinceCutoff(req.Args["since_ms"])

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

	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"fires": out, "count": len(out)},
	})
}

type scheduleFiredPayload struct {
	ScheduleID      string         `json:"schedule_id"`
	Intent          string         `json:"intent"`
	Model           string         `json:"model"`
	Target          string         `json:"target"`
	Agent           string         `json:"agent"`
	Workflow        string         `json:"workflow"`
	SystemTask      string         `json:"system_task"`
	Tool            string         `json:"tool"`
	Executor        string         `json:"executor"`
	Category        string         `json:"category"`
	EffectClass     string         `json:"effect_class"`
	UsesLLM         *bool          `json:"uses_llm"`
	AutonomyRunbook map[string]any `json:"autonomy_runbook"`
}

// extractScheduleFired pulls schedule_id + intent + model out of a
// schedule.fired payload (M54; schedule_id added M55). Returns zero values on
// parse failure so a malformed firing still lists with its correlation and
// outcome. schedule_id is "" for firings journaled before M55.
func extractScheduleFired(payload json.RawMessage) scheduleFiredPayload {
	if len(payload) == 0 {
		return scheduleFiredPayload{}
	}
	var p scheduleFiredPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return scheduleFiredPayload{}
	}
	return p
}

func scheduleFiredSystemTaskInfo(name string) (cadence.SystemTaskInfo, bool) {
	name = strings.TrimSpace(name)
	for _, info := range cadence.SystemTaskInfos() {
		if info.Name == name {
			return info, true
		}
	}
	return cadence.SystemTaskInfo{}, false
}

func scheduleFiredExecutor(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.Executor) != "" {
		return strings.TrimSpace(p.Executor)
	}
	if p.Target == cadence.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok && strings.TrimSpace(info.Executor) != "" {
			return info.Executor
		}
		return "daemon"
	}
	if p.Target == cadence.TargetWorkflow {
		return "workflow"
	}
	if p.Target == cadence.TargetTool {
		return "tool"
	}
	return "agent"
}

func scheduleFiredCategory(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.Category) != "" {
		return strings.TrimSpace(p.Category)
	}
	if p.Target == cadence.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok {
			return strings.TrimSpace(info.Category)
		}
	}
	return ""
}

func scheduleFiredEffectClass(p scheduleFiredPayload) string {
	if strings.TrimSpace(p.EffectClass) != "" {
		return strings.TrimSpace(p.EffectClass)
	}
	if p.Target == cadence.TargetSystemTask {
		if info, ok := scheduleFiredSystemTaskInfo(p.SystemTask); ok {
			return strings.TrimSpace(info.EffectClass)
		}
	}
	return ""
}

func scheduleFiredUsesLLM(p scheduleFiredPayload) bool {
	if p.UsesLLM != nil {
		return *p.UsesLLM
	}
	return p.Target == "" || p.Target == cadence.TargetWorkflow
}

func scheduleFiredAction(p scheduleFiredPayload) string {
	switch p.Target {
	case cadence.TargetWorkflow:
		if p.Workflow != "" {
			return "run workflow " + p.Workflow
		}
	case cadence.TargetSystemTask:
		if p.SystemTask != "" {
			return "run system task " + p.SystemTask
		}
	case cadence.TargetTool:
		if p.Tool != "" {
			return "run tool " + p.Tool
		}
	}
	if p.Agent != "" && p.Intent != "" {
		return "wake " + p.Agent + ": " + p.Intent
	}
	if p.Intent != "" {
		return p.Intent
	}
	return p.ScheduleID
}

// handleScheduleStats aggregates scheduled-run firings (M57) — the autonomy
// analogue of handleRunsStats. Folds the journal's schedule.fired events, joins
// each with its run outcome (collectRuns), and reports counts, success rate, and
// total spend over scheduled runs. Optional args.id scopes to one schedule;
// args.since_ms windows by firing time.
func (s *Server) handleScheduleStats(conn net.Conn, req Request) {
	idFilter, _ := req.Args["id"].(string)
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

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	runs, err := s.collectRuns(k)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var total, completed, failed, running, abandoned int
	var spent int64
	failedByReason := map[string]int{}
	scheduleSet := map[string]struct{}{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindScheduleFired {
			return nil
		}
		id := extractScheduleFired(e.Payload).ScheduleID
		if idFilter != "" && id != idFilter {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		total++
		if id != "" {
			scheduleSet[id] = struct{}{}
		}
		// Join with the firing's run outcome.
		if r, ok := runs[e.CorrelationID]; ok {
			switch {
			case r.Completed:
				completed++
			case r.Failed:
				failed++
				reason := r.FailReason
				if reason == "" {
					reason = "unknown"
				}
				failedByReason[reason]++
			case r.Abandoned:
				abandoned++
			default:
				running++
			}
			spent += r.SpentMicrocents
		} else {
			running++ // fired but no run events yet (or trimmed)
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	terminal := completed + failed + abandoned
	successRate := 0.0
	if terminal > 0 {
		successRate = float64(completed) / float64(terminal)
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":            total,
			"completed":        completed,
			"failed":           failed,
			"running":          running,
			"abandoned":        abandoned,
			"terminal":         terminal,
			"success_rate":     successRate,
			"spent_microcents": spent,
			"schedules":        len(scheduleSet),
			"failed_by_reason": failedByReason,
			"window_ms":        sinceMS,
		},
	})
}
