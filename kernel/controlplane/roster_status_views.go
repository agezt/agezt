// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/app/binding"
	approster "github.com/agezt/agezt/kernel/app/roster"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	appschedule "github.com/agezt/agezt/kernel/app/schedule"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/boardapi"
)

func (s *Server) agentStatusViews(profiles []agentapi.Profile) map[string]map[string]any {
	return approster.NewStatus(s.collectRosterStatusSnapshot, nil).Views(profiles)
}

// collectRosterStatusSnapshot binds the host's sources to the application's
// status collection: the journal, the folded run rows, the schedules, the
// standing orders, the board's open help requests and the reaper scan.
func (s *Server) collectRosterStatusSnapshot(profiles []agentapi.Profile, reaperCut, routingCut int64) approster.StatusSnapshot {
	return approster.CollectStatus(approster.StatusSources{
		Journal:   s.journalPorts().Events.Range,
		Runs:      s.statusRunRows,
		Schedules: binding.Fill(s.k, appschedule.PortsFromKernel).Entries,
		Standing:  binding.Fill(s.k, appstanding.PortsFromKernel).Orders,
		Board:     s.statusBoardMessages,
		Reaper:    func(cut int64) agentapi.ReaperReport { return s.reaperPorts().Scan(cut, cut) },
	}, profiles, reaperCut, routingCut)
}

// statusRunRows folds the kernel's journal into the run rows the status
// collection needs. The application owns what a row means; the host only
// supplies the rows.
func (s *Server) statusRunRows() ([]approster.RunRow, error) {
	rows, err := appruns.Collect(s.journalPorts().Events.Range)
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
