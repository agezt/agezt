// SPDX-License-Identifier: MIT
package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/board/board"
	"github.com/agezt/agezt/kernel/runtime"
)

func (s *Server) agentStatusViews(profiles []roster.Profile) map[string]map[string]any {
	return approster.NewStatus(s.collectRosterStatusSnapshot, nil).Views(profiles)
}

// collectRosterStatusSnapshot binds the host's sources to the application's
// status collection: the journal, the folded run rows, the schedules, the
// standing orders, the board's open help requests and the reaper scan.
func (s *Server) collectRosterStatusSnapshot(profiles []roster.Profile, reaperCut, routingCut int64) approster.StatusSnapshot {
	return approster.CollectStatus(approster.StatusSources{
		Journal: s.k.Journal().Range,
		Runs:    s.statusRunRows,
		Schedules: func() []cadenceapi.Entry {
			// The schedule library is the daemon's file (W3.49): a host
			// without it holds a nil port, and the status view sees no
			// schedules rather than calling into nothing.
			if s.k.Schedules() == nil {
				return nil
			}
			return s.k.Schedules().List()
		},
		Standing: func() []standingapi.Order {
			// The standing library is the daemon's file (W3.50): a host
			// without it holds a nil port, and the status view sees no
			// standing orders rather than calling into nothing.
			if s.k.Standing() == nil {
				return nil
			}
			return s.k.Standing().List()
		},
		Board:  s.statusBoardMessages,
		Reaper: func(cut int64) runtime.ReaperReport { return s.k.ReaperScan(cut, cut) },
	}, profiles, reaperCut, routingCut)
}

// statusRunRows folds the kernel's journal into the run rows the status
// collection needs. The application owns what a row means; the host only
// supplies the rows.
func (s *Server) statusRunRows() ([]approster.RunRow, error) {
	rows, err := appruns.Collect(s.k.Journal().Range)
	if err != nil {
		return nil, err
	}
	return appruns.RunRows(rows), nil
}

// statusBoardMessages lists the board's open help requests — the escalations
// the roster status counts.
func (s *Server) statusBoardMessages() []board.Message {
	st, err := s.boardReader()
	if err != nil || st == nil {
		return nil
	}
	return st.OpenHelp(boardReadMaxLimit)
}
