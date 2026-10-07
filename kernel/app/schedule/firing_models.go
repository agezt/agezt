// SPDX-License-Identifier: MIT

package schedule

import (
	"github.com/agezt/agezt/kernel/event"
)

type FiringJournal interface {
	Range(func(*event.Event) error) error
}
type FiringRun struct {
	Completed, Failed, Abandoned                                  bool
	StartedUnixMS, CompletedUnixMS, FailedUnixMS, SpentMicrocents int64
	FailReason, AnswerPreview                                     string
}

func firingRunStatus(r FiringRun) string {
	switch {
	case r.Completed:
		return "completed"
	case r.Failed:
		return "failed"
	case r.Abandoned:
		return "abandoned"
	default:
		return "running"
	}
}

type FiringService struct {
	journal FiringJournal
	runs    func() (map[string]FiringRun, error)
}

func NewFiringService(journal FiringJournal, runs func() (map[string]FiringRun, error)) *FiringService {
	return &FiringService{journal: journal, runs: runs}
}

type FiresInput struct {
	Limit               int
	CursorMS, CursorSeq int64
	CursorOK            bool
	ID, Status, Intent  string
	CutoffMS            int64
}
type FireRecord struct {
	CorrelationID   string         `json:"correlation_id"`
	ScheduleID      string         `json:"schedule_id"`
	FiredUnixMS     int64          `json:"fired_unix_ms"`
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
	UsesLLM         bool           `json:"uses_llm"`
	Action          string         `json:"action"`
	Status          string         `json:"status"`
	Reason          string         `json:"reason"`
	DurationMS      int64          `json:"duration_ms"`
	SpentMC         int64          `json:"spent_mc"`
	AnswerPreview   string         `json:"answer_preview"`
	AutonomyRunbook map[string]any `json:"autonomy_runbook,omitempty"`
}
type FiresOutput struct {
	Fires      []FireRecord `json:"fires"`
	Count      int          `json:"count"`
	NextCursor string       `json:"next_cursor"`
}
type StatsInput struct {
	ID                string
	SinceMS, CutoffMS int64
}
type StatsOutput struct {
	Total           int            `json:"total"`
	Completed       int            `json:"completed"`
	Failed          int            `json:"failed"`
	Running         int            `json:"running"`
	Abandoned       int            `json:"abandoned"`
	Terminal        int            `json:"terminal"`
	SuccessRate     float64        `json:"success_rate"`
	SpentMicrocents int64          `json:"spent_microcents"`
	Schedules       int            `json:"schedules"`
	FailedByReason  map[string]int `json:"failed_by_reason"`
	WindowMS        int64          `json:"window_ms"`
}
