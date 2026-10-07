// SPDX-License-Identifier: MIT
package tools

import "github.com/agezt/agezt/kernel/platform/journalview"

// Observations owns tool journal folds over the host-selected journal.
type Observations struct{ journal journalview.Reader }

func NewObservations(journal journalview.Reader) *Observations {
	return &Observations{journal: journal}
}

type toolInvocationKey struct{ correlationID, callID string }
type LogInput struct {
	ErrorsOnly bool
	Tool       string
	SlowMS     int64
	Page       journalview.Input
}
type LogOutput struct {
	Invocations []LogItem `json:"invocations"`
	Count       int       `json:"count"`
	NextCursor  string    `json:"next_cursor"`
}
type StatsInput struct {
	Tool               string
	CutoffMS, WindowMS int64
}
type StatsOutput struct {
	Total           int                    `json:"total"`
	Errored         int                    `json:"errored"`
	ErrorRate       float64                `json:"error_rate"`
	ByTool          map[string]ToolSummary `json:"by_tool"`
	Tools           int                    `json:"tools"`
	WindowMS        int64                  `json:"window_ms"`
	ErrorsByMessage map[string]int         `json:"errors_by_message"`
	DurationMS      LatencySummary         `json:"duration_ms"`
}
