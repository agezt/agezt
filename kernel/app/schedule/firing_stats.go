// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"github.com/agezt/agezt/kernel/event"
)

func (s *FiringService) Stats(_ context.Context, in StatsInput) (StatsOutput, error) {
	idFilter, sinceMS, cutoff := in.ID, in.SinceMS, in.CutoffMS
	runs, err := s.runs()
	if err != nil {
		return StatsOutput{}, err
	}

	var total, completed, failed, running, abandoned int
	var spent int64
	failedByReason := map[string]int{}
	scheduleSet := map[string]struct{}{}
	if err := s.journal.Range(func(e *event.Event) error {
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
		return StatsOutput{}, err
	}

	terminal := completed + failed + abandoned
	successRate := 0.0
	if terminal > 0 {
		successRate = float64(completed) / float64(terminal)
	}

	return StatsOutput{Total: total, Completed: completed, Failed: failed, Running: running, Abandoned: abandoned, Terminal: terminal, SuccessRate: successRate, SpentMicrocents: spent, Schedules: len(scheduleSet), FailedByReason: failedByReason, WindowMS: sinceMS}, nil
}
