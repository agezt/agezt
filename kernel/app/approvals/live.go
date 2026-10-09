// SPDX-License-Identifier: MIT

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Broker is the primary kernel's live approval registry.
type Broker interface {
	Pending() []approval.Request
	Resolve(id string, decision approval.Decision, reason, resolvedBy string) error
}

// Live lists the waiting approval requests and resolves them.
type Live struct{ broker Broker }

func NewLive(broker Broker) *Live { return &Live{broker: broker} }

// PendingRequest takes no arguments.
type PendingRequest struct{}

type PendingRow struct {
	ID                    string             `json:"id"`
	Capability            string             `json:"capability"`
	ToolName              string             `json:"tool_name"`
	Input                 string             `json:"input"`
	Reason                string             `json:"reason"`
	Actor                 string             `json:"actor"`
	CorrelationID         string             `json:"correlation_id"`
	CreatedUnix           int64              `json:"created_unix"`
	TimeoutUnix           int64              `json:"timeout_unix"`
	EffectClass           string             `json:"effect_class"`
	PredictedEffects      []string           `json:"predicted_effects"`
	AffectedResources     []string           `json:"affected_resources"`
	RollbackNotes         string             `json:"rollback_notes"`
	Confidence            float64            `json:"confidence"`
	CanonicalIntent       string             `json:"canonical_intent"`
	HarmfulInterpretation string             `json:"harmful_interpretation"`
	AmbiguityScore        float64            `json:"ambiguity_score"`
	RegretAxes            map[string]float64 `json:"regret_axes"`
	ConfirmationPrompt    string             `json:"confirmation_prompt"`
}

type PendingOutput struct {
	Pending []PendingRow `json:"pending"`
	Count   int          `json:"count"`
}

type DecideRequest struct {
	ID       json.RawMessage `json:"id,omitempty"`
	Decision json.RawMessage `json:"decision,omitempty"`
	Reason   json.RawMessage `json:"reason,omitempty"`
}

type DecideOutput struct {
	OK       bool   `json:"ok"`
	ID       string `json:"id"`
	Decision string `json:"decision"`
}

// lenientString reads a JSON string; anything else is empty.
func lenientString(raw json.RawMessage) string {
	s, _ := rawValue(raw).(string)
	return s
}

// Pending lists the waiting requests, oldest first, with the intent and effect
// metadata an operator decides on.
func (l *Live) Pending(context.Context, PendingRequest) (PendingOutput, error) {
	pending := l.broker.Pending()
	out := PendingOutput{Pending: make([]PendingRow, 0, len(pending)), Count: len(pending)}
	for _, p := range pending {
		out.Pending = append(out.Pending, PendingRow{
			ID: p.ID, Capability: p.Capability, ToolName: p.ToolName, Input: p.Input, Reason: p.Reason,
			Actor: p.Actor, CorrelationID: p.CorrelationID, CreatedUnix: p.CreatedAt.Unix(), TimeoutUnix: p.Timeout.Unix(),
			EffectClass: p.EffectClass, PredictedEffects: p.PredictedEffects, AffectedResources: p.AffectedResources,
			RollbackNotes: p.RollbackNotes, Confidence: p.Confidence, CanonicalIntent: p.CanonicalIntent,
			HarmfulInterpretation: p.HarmfulInterpretation, AmbiguityScore: p.AmbiguityScore, RegretAxes: p.RegretAxes,
			ConfirmationPrompt: p.ConfirmationPrompt,
		})
	}
	return out, nil
}

// Decide grants or denies one waiting request as the operator. The id, decision
// and reason are read leniently; an empty id is checked before the decision.
func (l *Live) Decide(_ context.Context, in DecideRequest) (DecideOutput, error) {
	id, name, reason := lenientString(in.ID), lenientString(in.Decision), lenientString(in.Reason)
	if id == "" {
		return DecideOutput{}, errors.New("args.id required")
	}
	var decision approval.Decision
	switch name {
	case "grant":
		decision = approval.DecisionGrant
	case "deny":
		decision = approval.DecisionDeny
	default:
		return DecideOutput{}, errors.New(`args.decision must be "grant" or "deny"`)
	}
	if err := l.broker.Resolve(id, decision, reason, "operator"); err != nil {
		return DecideOutput{}, err
	}
	return DecideOutput{OK: true, ID: id, Decision: name}, nil
}

func bindPrimary[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	output, err := schema.FromType(reflect.TypeFor[O](), false)
	if err != nil {
		return err
	}
	spec.OutputSchema, spec.AllowUnknownInput = output, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// LiveOperations declares the primary-only pending list (unaudited) and the
// audited decision.
func LiveOperations(provider func(context.Context) *Live) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("approvals live provider required")
	}
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bindPrimary(&ops, opapi.Spec{Name: "approvals", ReadOnly: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/approvals"}}, func(ctx context.Context, in PendingRequest) (PendingOutput, error) {
				return provider(ctx).Pending(ctx, in)
			})
		},
		func() error {
			return bindPrimary(&ops, opapi.Spec{Name: "decide", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{},"decision":{},"reason":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/decide"}}, func(ctx context.Context, in DecideRequest) (DecideOutput, error) {
				return provider(ctx).Decide(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
