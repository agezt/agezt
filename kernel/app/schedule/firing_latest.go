// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"github.com/agezt/agezt/kernel/event"
)

func (s *FiringService) Latest(_ context.Context) (map[string]LastFiring, error) {
	runs, err := s.runs()
	if err != nil {
		return nil, err
	}
	latest := map[string]LastFiring{}
	err = s.journal.Range(func(e *event.Event) error {
		if e.Kind != event.KindScheduleFired {
			return nil
		}
		id := extractScheduleFired(e.Payload).ScheduleID
		if id == "" {
			return nil
		}
		if cur, ok := latest[id]; ok && cur.FiredMS >= e.TSUnixMS {
			return nil // already have a newer (or same-ms) firing for this schedule
		}
		lf := LastFiring{FiredMS: e.TSUnixMS, Status: "running"}
		if r, ok := runs[e.CorrelationID]; ok {
			switch {
			case r.Completed:
				lf.Status = "completed"
			case r.Failed:
				lf.Status = "failed"
				lf.Reason = r.FailReason
			case r.Abandoned:
				lf.Status = "abandoned"
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
