// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"sort"
	"strings"
)

type WatchStore interface {
	Get(string) (tasks.Task, bool)
	BlockingDependencies(string) ([]tasks.DependencyState, error)
}
type Watch struct {
	store   WatchStore
	journal journalview.Reader
}

func NewWatch(store WatchStore, journal journalview.Reader) *Watch {
	return &Watch{store: store, journal: journal}
}

type WatchInput struct {
	ID    string `json:"id"`
	RunID string `json:"run_id,omitempty"`
	Limit int    `json:"limit,omitempty"`
}
type EventRow struct {
	Seq           int64           `json:"seq"`
	TSUnixMS      int64           `json:"ts_unix_ms"`
	Kind          event.Kind      `json:"kind"`
	Subject       string          `json:"subject"`
	CorrelationID string          `json:"correlation_id"`
	Payload       *map[string]any `json:"payload,omitempty"`
}
type Dependency struct {
	ID        string       `json:"id"`
	Status    tasks.Status `json:"status"`
	Title     string       `json:"title,omitempty"`
	Missing   bool         `json:"missing,omitempty"`
	CreatedMS int64        `json:"created_ms,omitempty"`
}

func projectDependencies(states []tasks.DependencyState) []Dependency {
	out := make([]Dependency, 0, len(states))
	for _, state := range states {
		row := Dependency{ID: state.ID, Status: state.Status, Title: state.Title, Missing: state.Missing}
		if state.CreatedMS > 0 {
			row.CreatedMS = state.CreatedMS
		}
		out = append(out, row)
	}
	return out
}

type WatchOutput struct {
	Task                Record       `json:"task"`
	Events              []EventRow   `json:"events"`
	Count               int          `json:"count"`
	BlockedDependencies []Dependency `json:"blocked_dependencies"`
	RunID               string       `json:"run_id,omitempty"`
}

func latestRunID(task tasks.Task) string {
	if task.Claim != nil && strings.TrimSpace(task.Claim.RunID) != "" {
		return strings.TrimSpace(task.Claim.RunID)
	}
	var best string
	var bestMS int64
	for _, attempt := range task.Attempts {
		ts := attempt.StartedMS
		if attempt.FinishedMS > ts {
			ts = attempt.FinishedMS
		}
		if strings.TrimSpace(attempt.RunID) != "" && ts >= bestMS {
			bestMS = ts
			best = strings.TrimSpace(attempt.RunID)
		}
	}
	for _, link := range task.Links {
		if strings.EqualFold(link.Type, "run") && strings.TrimSpace(link.Target) != "" && link.CreatedMS >= bestMS {
			bestMS = link.CreatedMS
			best = strings.TrimSpace(link.Target)
		}
	}
	return best
}
func (s *Watch) events(taskID, runID string, limit int) []EventRow {
	if s.journal == nil {
		return nil
	}
	subject := "workboard." + taskID
	var rows []EventRow
	// Preserve the native best-effort Range behavior in this move. Read-error
	// semantics are a separate measured repair, not part of extraction.
	_ = s.journal.Range(func(e *event.Event) error {
		if e.Subject != subject && (runID == "" || e.CorrelationID != runID) {
			return nil
		}
		row := EventRow{Seq: e.Seq, TSUnixMS: e.TSUnixMS, Kind: e.Kind, Subject: e.Subject, CorrelationID: e.CorrelationID}
		if len(e.Payload) > 0 {
			var payload map[string]any
			if json.Unmarshal(e.Payload, &payload) == nil {
				row.Payload = &payload
			}
		}
		rows = append(rows, row)
		return nil
	})
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq < rows[j].Seq })
	if limit > 0 && len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return rows
}
func (s *Watch) Watch(_ context.Context, in WatchInput) (WatchOutput, error) {
	if in.ID == "" {
		return WatchOutput{}, errors.New("workboard_watch requires id")
	}
	task, found := s.store.Get(in.ID)
	if !found {
		return WatchOutput{}, fmt.Errorf("unknown workboard task: %s", in.ID)
	}
	runID := in.RunID
	if runID == "" {
		runID = latestRunID(task)
	}
	rows := s.events(task.ID, runID, in.Limit)
	blocked, _ := s.store.BlockingDependencies(task.ID)
	return WatchOutput{Task: Project(task), Events: rows, Count: len(rows), BlockedDependencies: projectDependencies(blocked), RunID: runID}, nil
}
