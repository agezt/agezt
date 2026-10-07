// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	tasks "github.com/agezt/agezt/kernel/workboard"
	"strings"
	"time"
)

// NativeServices binds selected host ports after authorization and audit.
type NativeServices struct {
	Reads     *Service
	Lifecycle *Lifecycle
	Relations *Relations
	Watch     *Watch
	Dispatch  *Dispatch
	ValidSeat func(string) bool
}

func requestText(raw any) string { s, _ := raw.(string); return strings.TrimSpace(s) }
func requestNumber(raw any) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}
func requestLimit(raw any, def int) int {
	n := def
	switch v := raw.(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case int64:
		n = int(v)
	}
	if n < 1 {
		return def
	}
	return n
}
func requestStrings(raw any) []string {
	switch xs := raw.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, raw := range xs {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case string:
		if strings.TrimSpace(xs) != "" {
			return strings.Split(xs, ",")
		}
	}
	return nil
}
func requestRawNumber(raw json.RawMessage) int {
	var value any
	_ = json.Unmarshal(raw, &value)
	return requestNumber(value)
}

// Explicit inbound correlation remains a native bridge contract, as for board.
// Otherwise domain events use the host-owned operation correlation.
func requestCorrelation(ctx context.Context, raw any) string {
	if s := requestText(raw); s != "" {
		return s
	}
	return opapi.CorrelationFromContext(ctx)
}
func nativeResult[O any](id any, out O, err error) (O, error) {
	if errors.Is(err, tasks.ErrNotFound) {
		err = fmt.Errorf("unknown workboard task: %s", requestText(id))
	}
	return out, err
}
func bindRequest[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	op, err := app.NewOperation(spec, handler)
	if err == nil {
		*ops = append(*ops, op)
	}
	return err
}

type ListRequestInput struct {
	Status          any  `json:"status,omitempty"`
	Tenant          any  `json:"tenant,omitempty"`
	Assignee        any  `json:"assignee,omitempty"`
	IncludeArchived bool `json:"include_archived,omitempty"`
	Limit           any  `json:"limit,omitempty"`
}

type IDRequestInput struct {
	ID any `json:"id,omitempty"`
}

type ClaimRequestInput struct {
	ID            any `json:"id,omitempty"`
	Agent         any `json:"agent,omitempty"`
	RunID         any `json:"run_id,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type CommentRequestInput struct {
	ID            any `json:"id,omitempty"`
	Author        any `json:"author,omitempty"`
	Body          any `json:"body,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type ReasonRequestInput struct {
	ID            any `json:"id,omitempty"`
	Actor         any `json:"actor,omitempty"`
	Reason        any `json:"reason,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type ProveRequestInput struct {
	ID            any `json:"id,omitempty"`
	Answer        any `json:"answer,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type SeatRequestInput struct {
	ID   any `json:"id,omitempty"`
	Seat any `json:"seat,omitempty"`
}

type LinkRequestInput struct {
	ID            any `json:"id,omitempty"`
	Type          any `json:"type,omitempty"`
	Target        any `json:"target,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type DependRequestInput struct {
	ID            any `json:"id,omitempty"`
	DependsOn     any `json:"depends_on,omitempty"`
	On            any `json:"on,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type ReclaimRequestInput struct {
	ID            any `json:"id,omitempty"`
	Actor         any `json:"actor,omitempty"`
	StaleAfterMS  any `json:"stale_after_ms,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type SweepRequestInput struct {
	Actor         any `json:"actor,omitempty"`
	StaleAfterMS  any `json:"stale_after_ms,omitempty"`
	Limit         any `json:"limit,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

type WatchRequestInput struct {
	ID    any `json:"id,omitempty"`
	RunID any `json:"run_id,omitempty"`
	Limit any `json:"limit,omitempty"`
}

type DispatchRequestInput struct {
	ID     any `json:"id,omitempty"`
	Agent  any `json:"agent,omitempty"`
	Reason any `json:"reason,omitempty"`
	Intent any `json:"intent,omitempty"`
}

type PolicyRequestInput struct {
	ID            any             `json:"id,omitempty"`
	Actor         any             `json:"actor,omitempty"`
	Clear         bool            `json:"clear,omitempty"`
	MaxAttempts   json.RawMessage `json:"max_attempts,omitempty"`
	EscalateTo    any             `json:"escalate_to,omitempty"`
	CorrelationID any             `json:"correlation_id,omitempty"`
}

type CreateRequestInput struct {
	Title          any             `json:"title,omitempty"`
	Description    any             `json:"description,omitempty"`
	Status         any             `json:"status,omitempty"`
	Priority       any             `json:"priority,omitempty"`
	Tenant         any             `json:"tenant,omitempty"`
	Assignee       any             `json:"assignee,omitempty"`
	Owner          any             `json:"owner,omitempty"`
	IdempotencyKey any             `json:"idempotency_key,omitempty"`
	Tags           any             `json:"tags,omitempty"`
	Artifacts      any             `json:"artifacts,omitempty"`
	Criteria       any             `json:"criteria,omitempty"`
	Seat           any             `json:"seat,omitempty"`
	EscalateTo     any             `json:"escalate_to,omitempty"`
	CorrelationID  any             `json:"correlation_id,omitempty"`
	MaxAttempts    json.RawMessage `json:"max_attempts,omitempty"`
}

var createRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"title":{},"description":{},"status":{},"priority":{},"tenant":{},"assignee":{},"owner":{},"idempotency_key":{},"tags":{},"artifacts":{},"criteria":{},"seat":{},"escalate_to":{},"correlation_id":{},"max_attempts":{}}}`)

var policyRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{},"actor":{},"clear":{"type":"boolean"},"max_attempts":{},"escalate_to":{},"correlation_id":{}}}`)

func Operations(provider func(context.Context) NativeServices) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("workboard service provider required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_list", ReadOnly: true}, func(ctx context.Context, in ListRequestInput) (ListOutput, error) {
				var status tasks.Status
				if raw := requestText(in.Status); raw != "" {
					st, err := tasks.ParseStatus(raw)
					if err != nil {
						return ListOutput{}, err
					}
					status = st
				}
				return provider(ctx).Reads.List(ctx, ListInput{Status: status, Tenant: requestText(in.Tenant), Assignee: requestText(in.Assignee), IncludeArchived: in.IncludeArchived, Limit: requestLimit(in.Limit, 100)})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_lanes", ReadOnly: true}, func(ctx context.Context, in ListRequestInput) (LanesOutput, error) {
				var status tasks.Status
				if raw := requestText(in.Status); raw != "" {
					st, err := tasks.ParseStatus(raw)
					if err != nil {
						return LanesOutput{}, err
					}
					status = st
				}
				return provider(ctx).Reads.Lanes(ctx, ListInput{Status: status, Tenant: requestText(in.Tenant), IncludeArchived: in.IncludeArchived, Limit: requestLimit(in.Limit, 500)})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_show", ReadOnly: true}, func(ctx context.Context, in IDRequestInput) (ShowOutput, error) {
				return provider(ctx).Reads.Show(ctx, ShowInput{ID: requestText(in.ID)})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_create", InputSchema: createRequestSchema}, func(ctx context.Context, in CreateRequestInput) (CreateOutput, error) {
				title := requestText(in.Title)
				if title == "" {
					return CreateOutput{}, errors.New("workboard_create requires title")
				}
				var status tasks.Status
				if raw := requestText(in.Status); raw != "" {
					st, err := tasks.ParseStatus(raw)
					if err != nil {
						return CreateOutput{}, err
					}
					status = st
				}
				services := provider(ctx)
				seatID := requestText(in.Seat)
				if !services.ValidSeat(seatID) {
					return CreateOutput{}, errors.New("unknown execution seat: " + seatID)
				}
				var policy *tasks.RetryPolicy
				if len(in.MaxAttempts) > 0 || requestText(in.EscalateTo) != "" {
					policy = &tasks.RetryPolicy{MaxAttempts: requestRawNumber(in.MaxAttempts), EscalateTo: requestText(in.EscalateTo)}
				}
				out, err := services.Lifecycle.Create(ctx, CreateInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), Spec: tasks.CreateSpec{Title: title, Description: requestText(in.Description), Status: status, Priority: requestNumber(in.Priority), Tenant: requestText(in.Tenant), Assignee: requestText(in.Assignee), Owner: requestText(in.Owner), IdempotencyKey: requestText(in.IdempotencyKey), Tags: requestStrings(in.Tags), Artifacts: requestStrings(in.Artifacts), AcceptanceCriteria: requestStrings(in.Criteria), Seat: seatID, RetryPolicy: policy}})
				return nativeResult(nil, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_claim"}, func(ctx context.Context, in ClaimRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Claim(ctx, ClaimInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Agent: requestText(in.Agent), RunID: requestText(in.RunID)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_heartbeat"}, func(ctx context.Context, in ClaimRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Heartbeat(ctx, ClaimInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Agent: requestText(in.Agent), RunID: requestText(in.RunID)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_comment"}, func(ctx context.Context, in CommentRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Comment(ctx, CommentInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Author: requestText(in.Author), Body: requestText(in.Body)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_block"}, func(ctx context.Context, in ReasonRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Block(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Actor: requestText(in.Actor), Reason: requestText(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_fail"}, func(ctx context.Context, in ReasonRequestInput) (FailOutput, error) {
				out, err := provider(ctx).Lifecycle.Fail(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Actor: requestText(in.Actor), Reason: requestText(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_unblock"}, func(ctx context.Context, in ReasonRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Unblock(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Actor: requestText(in.Actor), Reason: requestText(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_complete"}, func(ctx context.Context, in ReasonRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Complete(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Actor: requestText(in.Actor), Reason: requestText(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_archive"}, func(ctx context.Context, in ReasonRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Archive(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Actor: requestText(in.Actor), Reason: requestText(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_prove"}, func(ctx context.Context, in ProveRequestInput) (TaskOutput, error) {
				id := requestText(in.ID)
				if id == "" {
					return TaskOutput{}, errors.New("workboard_prove requires id")
				}
				ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
				defer cancel()
				out, err := provider(ctx).Lifecycle.Prove(ctx, ProveInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: id, Answer: requestText(in.Answer)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_seat"}, func(ctx context.Context, in SeatRequestInput) (TaskOutput, error) {
				id := requestText(in.ID)
				if id == "" {
					return TaskOutput{}, errors.New("workboard_seat requires id")
				}
				services := provider(ctx)
				seatID := requestText(in.Seat)
				if !services.ValidSeat(seatID) {
					return TaskOutput{}, errors.New("unknown execution seat: " + seatID)
				}
				out, err := services.Lifecycle.Seat(ctx, SeatInput{ID: id, Seat: seatID})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_link"}, func(ctx context.Context, in LinkRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Relations.Link(ctx, LinkInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Type: requestText(in.Type), Target: requestText(in.Target)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_policy", InputSchema: policyRequestSchema}, func(ctx context.Context, in PolicyRequestInput) (TaskOutput, error) {
				id := requestText(in.ID)
				if id == "" {
					return TaskOutput{}, errors.New("workboard_policy requires id")
				}
				var policy *tasks.RetryPolicy
				if !in.Clear {
					if len(in.MaxAttempts) == 0 {
						return TaskOutput{}, errors.New("workboard_policy requires max_attempts or clear")
					}
					max := requestRawNumber(in.MaxAttempts)
					if max < 1 {
						return TaskOutput{}, errors.New("workboard_policy max_attempts must be positive")
					}
					policy = &tasks.RetryPolicy{MaxAttempts: max, EscalateTo: requestText(in.EscalateTo)}
				}
				out, err := provider(ctx).Relations.Policy(ctx, PolicyInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: id, Actor: requestText(in.Actor), Policy: policy})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_depend"}, func(ctx context.Context, in DependRequestInput) (TaskOutput, error) {
				parent := requestText(in.DependsOn)
				if parent == "" {
					parent = requestText(in.On)
				}
				out, err := provider(ctx).Relations.Depend(ctx, DependInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), DependsOn: parent})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_reclaim"}, func(ctx context.Context, in ReclaimRequestInput) (TaskOutput, error) {
				out, err := provider(ctx).Relations.Reclaim(ctx, ReclaimInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), Actor: requestText(in.Actor), StaleAfterMS: requestLimit(in.StaleAfterMS, 10*60*1000)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_sweep"}, func(ctx context.Context, in SweepRequestInput) (SweepOutput, error) {
				limit := requestLimit(in.Limit, 100)
				if limit > 1000 {
					limit = 1000
				}
				actor := requestText(in.Actor)
				if actor == "" {
					actor = "workboard-sweeper"
				}
				return provider(ctx).Relations.Sweep(ctx, SweepInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), Actor: actor, StaleAfterMS: requestLimit(in.StaleAfterMS, 10*60*1000), Limit: limit})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_watch", ReadOnly: true}, func(ctx context.Context, in WatchRequestInput) (WatchOutput, error) {
				id := requestText(in.ID)
				if id == "" {
					return WatchOutput{}, errors.New("workboard_watch requires id")
				}
				limit := requestLimit(in.Limit, 50)
				if limit > 200 {
					limit = 200
				}
				return provider(ctx).Watch.Watch(ctx, WatchInput{ID: id, RunID: requestText(in.RunID), Limit: limit})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: "workboard_dispatch"}, func(ctx context.Context, in DispatchRequestInput) (DispatchOutput, error) {
				out, err := provider(ctx).Dispatch.Dispatch(ctx, DispatchInput{ID: requestText(in.ID), Agent: requestText(in.Agent), Reason: requestText(in.Reason), Intent: requestText(in.Intent)})
				return nativeResult(in.ID, out, err)
			})
		},
	}
	for _, bind := range bindings {
		if err := bind(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
