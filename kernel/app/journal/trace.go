// SPDX-License-Identifier: MIT

package journal

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
)

// Tracer walks the routed kernel's journal from one event.
type Tracer interface {
	Why(eventID string) ([]*event.Event, error)
	ParentOf(correlation string) string
	Causes(eventID string) ([]*event.Event, error)
}

// Trace explains one event: its correlation chain, the sub-agent parent, and
// the causation chain that may cross correlations.
type Trace struct{ tracer Tracer }

func NewTrace(tracer Tracer) *Trace { return &Trace{tracer: tracer} }

type WhyRequest struct {
	EventID json.RawMessage `json:"event_id,omitempty"`
}

type WhyOutput struct {
	Events            []*event.Event `json:"events"`
	Correlation       string         `json:"correlation"`
	ParentCorrelation string         `json:"parent_correlation"`
	CausationChain    []*event.Event `json:"causation_chain"`
}

type wireWhy struct {
	Events            []wireEvent `json:"events"`
	Correlation       string      `json:"correlation"`
	ParentCorrelation string      `json:"parent_correlation"`
	CausationChain    []wireEvent `json:"causation_chain"`
}

// Why returns the event's correlation chain and its lead's correlation for a
// sub-agent run. The causation chain is best-effort: it is reported only when it
// links more than the event itself, and its failure never fails the trace.
func (t *Trace) Why(_ context.Context, in WhyRequest) (WhyOutput, error) {
	id, _ := rawValue(in.EventID).(string)
	if id == "" {
		return WhyOutput{}, errors.New("args.event_id required")
	}
	events, err := t.tracer.Why(id)
	if err != nil {
		return WhyOutput{}, err
	}
	out := WhyOutput{Events: append(make([]*event.Event, 0, len(events)), events...), CausationChain: make([]*event.Event, 0, 4)}
	if len(events) > 0 {
		out.Correlation = events[0].CorrelationID
	}
	if out.Correlation != "" {
		out.ParentCorrelation = t.tracer.ParentOf(out.Correlation)
	}
	if chain, err := t.tracer.Causes(id); err == nil && len(chain) > 1 {
		out.CausationChain = append(out.CausationChain, chain...)
	}
	return out, nil
}

// TraceOperations declares the unaudited trace read. It routes to the caller's
// tenant kernel, so a tenant walks only its own events.
func TraceOperations(provider func(context.Context) *Trace) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("trace provider required")
	}
	var ops []app.Operation
	if err := bind(&ops, opapi.Spec{Name: "why", Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"event_id":{}}}`)}, func(ctx context.Context, in WhyRequest) (WhyOutput, error) {
		return provider(ctx).Why(ctx, in)
	}); err != nil {
		return nil, err
	}
	return ops, nil
}
