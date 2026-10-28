// SPDX-License-Identifier: MIT
package controlplane

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
)

func (s *Server) agentStatusViews(profiles []agentapi.Profile) map[string]map[string]any {
	return approster.NewStatus(s.collectRosterStatusSnapshot, nil).Views(profiles)
}

// collectRosterStatusSnapshot binds the host's sources to the application's
// status collection: the journal, the folded run rows, the schedules, the
// standing orders, the board's open help requests and the reaper scan.
func (s *Server) collectRosterStatusSnapshot(profiles []agentapi.Profile, reaperCut, routingCut int64) approster.StatusSnapshot {
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
		Standing: appstanding.PortsFromKernel(s.k).Orders,
		Board:    s.statusBoardMessages,
		Reaper:   func(cut int64) agentapi.ReaperReport { return s.k.ReaperScan(cut, cut) },
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
func (s *Server) statusBoardMessages() []boardapi.Message {
	st, err := s.boardReader()
	if err != nil || st == nil {
		return nil
	}
	return st.OpenHelp(boardReadMaxLimit)
}
