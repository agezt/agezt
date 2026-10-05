// SPDX-License-Identifier: MIT

package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/journalview"
	curated "github.com/agezt/agezt/kernel/skill"
	"time"
)

type ObservationStore interface {
	Get(string) (curated.Skill, bool, error)
	Bundles() *curated.BundleStore
	Hygiene(int64) (curated.HygieneReport, error)
}
type Observations struct {
	forge   ObservationStore
	journal journalview.Reader
}

func NewObservations(forge ObservationStore, journal journalview.Reader) *Observations {
	return &Observations{forge: forge, journal: journal}
}

type HistoryEvent struct {
	Seq           int64          `json:"seq"`
	ID            string         `json:"id"`
	Kind          event.Kind     `json:"kind"`
	CorrelationID string         `json:"correlation_id"`
	TSUnixMS      int64          `json:"ts_unix_ms"`
	Payload       map[string]any `json:"payload"`
}
type HistoryOutput struct {
	ID     string         `json:"id"`
	Events []HistoryEvent `json:"events"`
	Count  int            `json:"count"`
}

func isSkillKind(kind event.Kind) bool {
	switch kind {
	case event.KindSkillCreated, event.KindSkillPromoted, event.KindSkillQuarantined, event.KindSkillReverted, event.KindSkillRestored, event.KindSkillActivated:
		return true
	}
	return false
}
func (s *Observations) History(_ context.Context, in GetInput) (HistoryOutput, error) {
	var events []HistoryEvent
	// Malformed individual payloads retain best-effort filtering. A failed journal
	// read must not turn an empty or partial fold into successful history.
	if err := s.journal.Range(func(e *event.Event) error {
		if !isSkillKind(e.Kind) {
			return nil
		}
		var payload map[string]any
		if json.Unmarshal(e.Payload, &payload) != nil {
			return nil
		}
		if payload["id"] != in.ID && payload["restored"] != in.ID {
			return nil
		}
		events = append(events, HistoryEvent{Seq: e.Seq, ID: e.ID, Kind: e.Kind, CorrelationID: e.CorrelationID, TSUnixMS: e.TSUnixMS, Payload: payload})
		return nil
	}); err != nil {
		return HistoryOutput{}, err
	}
	return HistoryOutput{ID: in.ID, Events: events, Count: len(events)}, nil
}

type FilesOutput struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Files []string `json:"files"`
	Dir   string   `json:"dir"`
	Count int      `json:"count"`
}
type ReadFileInput struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}
type ReadFileOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Bytes   int    `json:"bytes"`
}

func (s *Observations) Files(_ context.Context, in GetInput) (FilesOutput, error) {
	sk, found, err := s.forge.Get(in.ID)
	if err != nil {
		return FilesOutput{}, err
	}
	if !found {
		return FilesOutput{}, fmt.Errorf("no skill with id %s", in.ID)
	}
	files, dir := sk.Resources, ""
	if bundles := s.forge.Bundles(); bundles != nil {
		if live, err := bundles.List(sk.Name); err == nil && live != nil {
			files = live
		}
		dir = bundles.Dir(sk.Name)
	}
	return FilesOutput{ID: sk.ID, Name: sk.Name, Files: files, Dir: dir, Count: len(files)}, nil
}
func (s *Observations) ReadFile(_ context.Context, in ReadFileInput) (ReadFileOutput, error) {
	sk, found, err := s.forge.Get(in.ID)
	if err != nil {
		return ReadFileOutput{}, err
	}
	if !found {
		return ReadFileOutput{}, fmt.Errorf("no skill with id %s", in.ID)
	}
	bundles := s.forge.Bundles()
	if bundles == nil {
		return ReadFileOutput{}, errors.New("skill bundles are not available on this daemon")
	}
	data, err := bundles.Read(sk.Name, in.Path)
	if err != nil {
		return ReadFileOutput{}, err
	}
	return ReadFileOutput{ID: sk.ID, Name: sk.Name, Path: in.Path, Content: string(data), Bytes: len(data)}, nil
}

type HygieneInput struct {
	IdleDays int `json:"idle_days,omitempty"`
}
type IdleRecord struct {
	Record
	Uses       int   `json:"uses"`
	LastUsedMS int64 `json:"last_used_ms"`
}
type HygieneOutput struct {
	IdleDays  int          `json:"idle_days"`
	Total     int          `json:"total"`
	Active    int          `json:"active"`
	Idle      []IdleRecord `json:"idle"`
	IdleCount int          `json:"idle_count"`
}

func (s *Observations) Hygiene(_ context.Context, in HygieneInput) (HygieneOutput, error) {
	days := in.IdleDays
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour).UnixMilli()
	rep, err := s.forge.Hygiene(cutoff)
	if err != nil {
		return HygieneOutput{}, err
	}
	out := HygieneOutput{IdleDays: days, Total: rep.Total, Active: rep.Active, Idle: make([]IdleRecord, 0, len(rep.Idle)), IdleCount: len(rep.Idle)}
	for _, sk := range rep.Idle {
		out.Idle = append(out.Idle, IdleRecord{Record: project(sk), Uses: sk.Metrics.Uses, LastUsedMS: sk.Metrics.LastUsedMS})
	}
	return out, nil
}
