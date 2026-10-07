// SPDX-License-Identifier: MIT

package standing

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/event"
	"strings"
)

type Journal interface {
	Range(func(*event.Event) error) error
}
type Observations struct{ journal Journal }

func NewObservations(journal Journal) *Observations { return &Observations{journal: journal} }

type WhyInput struct{ ID string }
type LifeEvent struct {
	Seq           int64          `json:"seq"`
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	CorrelationID string         `json:"correlation_id"`
	TSUnixMS      int64          `json:"ts_unix_ms"`
	Payload       map[string]any `json:"payload"`
}
type WhyOutput struct {
	ID     string      `json:"id"`
	Events []LifeEvent `json:"events"`
	Count  int         `json:"count"`
}

func (s *Observations) Why(_ context.Context, in WhyInput) (WhyOutput, error) {
	var events []LifeEvent
	// Preserve legacy best-effort history, including partial rows on range failure.
	_ = s.journal.Range(func(e *event.Event) error {
		if !strings.HasPrefix(string(e.Kind), "standing.") {
			return nil
		}
		var payload map[string]any
		if json.Unmarshal(e.Payload, &payload) != nil {
			return nil
		}
		if payload["id"] != in.ID {
			return nil
		}
		events = append(events, LifeEvent{Seq: e.Seq, ID: e.ID, Kind: string(e.Kind), CorrelationID: e.CorrelationID, TSUnixMS: e.TSUnixMS, Payload: payload})
		return nil
	})
	return WhyOutput{ID: in.ID, Events: events, Count: len(events)}, nil
}
