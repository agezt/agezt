// SPDX-License-Identifier: MIT

package workboard

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	tasks "github.com/agezt/agezt/kernel/modules/work/workboard"
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

// The workboard's request rows live in the contract (kernel/contract/opapi):
// the task's id, the actor and the reason are text, the counts are numbers
// that know whether they were sent at all, and the tag lists are lists of
// texts — or the ONE comma-separated string a caller sends instead of a list.
//
// The family's own rules stay here, because they are the family's vocabulary:
// a count that is not a count is the family's default, a sweep is capped at
// the family's cap, and a page of frames is capped at the family's cap.

// text reads a text field of the request row: the family's texts are trimmed.
func text(value string) string { return strings.TrimSpace(value) }

// admittedLimit reads a count the row asked for: the family's default when the
// row did not ask for one — or asked for one that is not a count.
//
// A count at or below zero is no count the caller asked for, and a fraction
// truncates to no count at all, so both read as the family's default.
func admittedLimit(limit opapi.Limit, fallback int) int {
	if !limit.Present() {
		return fallback
	}
	size := int(limit.Number())
	if size < 1 {
		return fallback
	}
	return size
}

// Explicit inbound correlation remains a native bridge contract, as for board.
// Otherwise domain events use the host-owned operation correlation.
func requestCorrelation(ctx context.Context, value string) string {
	if trimmed := text(value); trimmed != "" {
		return trimmed
	}
	return opapi.CorrelationFromContext(ctx)
}
func nativeResult[O any](id string, out O, err error) (O, error) {
	if errors.Is(err, tasks.ErrNotFound) {
		err = fmt.Errorf("unknown workboard task: %s", text(id))
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

func Operations(provider func(context.Context) NativeServices) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("workboard service provider required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardList, ReadOnly: true}, func(ctx context.Context, in opapi.WorkboardListRequest) (ListOutput, error) {
				var status tasks.Status
				if raw := text(in.Status); raw != "" {
					st, err := tasks.ParseStatus(raw)
					if err != nil {
						return ListOutput{}, err
					}
					status = st
				}
				return provider(ctx).Reads.List(ctx, ListInput{Status: status, Tenant: text(in.Tenant), Assignee: text(in.Assignee), IncludeArchived: in.IncludeArchived, Limit: admittedLimit(in.Limit, 100)})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardLanes, ReadOnly: true}, func(ctx context.Context, in opapi.WorkboardListRequest) (LanesOutput, error) {
				var status tasks.Status
				if raw := text(in.Status); raw != "" {
					st, err := tasks.ParseStatus(raw)
					if err != nil {
						return LanesOutput{}, err
					}
					status = st
				}
				return provider(ctx).Reads.Lanes(ctx, ListInput{Status: status, Tenant: text(in.Tenant), IncludeArchived: in.IncludeArchived, Limit: admittedLimit(in.Limit, 500)})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardShow, ReadOnly: true}, func(ctx context.Context, in opapi.WorkboardShowRequest) (ShowOutput, error) {
				return provider(ctx).Reads.Show(ctx, ShowInput{ID: text(in.ID)})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardCreate}, func(ctx context.Context, in opapi.WorkboardCreateRequest) (CreateOutput, error) {
				title := text(in.Title)
				if title == "" {
					return CreateOutput{}, errors.New("workboard_create requires title")
				}
				var status tasks.Status
				if raw := text(in.Status); raw != "" {
					st, err := tasks.ParseStatus(raw)
					if err != nil {
						return CreateOutput{}, err
					}
					status = st
				}
				services := provider(ctx)
				seatID := text(in.Seat)
				if !services.ValidSeat(seatID) {
					return CreateOutput{}, errors.New("unknown execution seat: " + seatID)
				}
				var policy *tasks.RetryPolicy
				if in.MaxAttempts.Present() || text(in.EscalateTo) != "" {
					policy = &tasks.RetryPolicy{MaxAttempts: int(in.MaxAttempts.Number()), EscalateTo: text(in.EscalateTo)}
				}
				out, err := services.Lifecycle.Create(ctx, CreateInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), Spec: tasks.CreateSpec{Title: title, Description: text(in.Description), Status: status, Priority: int(in.Priority), Tenant: text(in.Tenant), Assignee: text(in.Assignee), Owner: text(in.Owner), IdempotencyKey: text(in.IdempotencyKey), Tags: opapi.TextListValue(in.Tags), Artifacts: opapi.TextListValue(in.Artifacts), AcceptanceCriteria: opapi.TextListValue(in.Criteria), Seat: seatID, RetryPolicy: policy}})
				return nativeResult("", out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardClaim}, func(ctx context.Context, in opapi.WorkboardClaimRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Claim(ctx, ClaimInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Agent: text(in.Agent), RunID: text(in.RunID)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardHeartbeat}, func(ctx context.Context, in opapi.WorkboardClaimRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Heartbeat(ctx, ClaimInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Agent: text(in.Agent), RunID: text(in.RunID)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardComment}, func(ctx context.Context, in opapi.WorkboardCommentRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Comment(ctx, CommentInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Author: text(in.Author), Body: text(in.Body)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardBlock}, func(ctx context.Context, in opapi.WorkboardReasonRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Block(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Actor: text(in.Actor), Reason: text(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardFail}, func(ctx context.Context, in opapi.WorkboardReasonRequest) (FailOutput, error) {
				out, err := provider(ctx).Lifecycle.Fail(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Actor: text(in.Actor), Reason: text(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardUnblock}, func(ctx context.Context, in opapi.WorkboardReasonRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Unblock(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Actor: text(in.Actor), Reason: text(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardComplete}, func(ctx context.Context, in opapi.WorkboardReasonRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Complete(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Actor: text(in.Actor), Reason: text(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardArchive}, func(ctx context.Context, in opapi.WorkboardReasonRequest) (TaskOutput, error) {
				out, err := provider(ctx).Lifecycle.Archive(ctx, ReasonInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Actor: text(in.Actor), Reason: text(in.Reason)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardProve}, func(ctx context.Context, in opapi.WorkboardProveRequest) (TaskOutput, error) {
				id := text(in.ID)
				if id == "" {
					return TaskOutput{}, errors.New("workboard_prove requires id")
				}
				ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
				defer cancel()
				out, err := provider(ctx).Lifecycle.Prove(ctx, ProveInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: id, Answer: text(in.Answer)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardSeat}, func(ctx context.Context, in opapi.WorkboardSeatRequest) (TaskOutput, error) {
				id := text(in.ID)
				if id == "" {
					return TaskOutput{}, errors.New("workboard_seat requires id")
				}
				services := provider(ctx)
				seatID := text(in.Seat)
				if !services.ValidSeat(seatID) {
					return TaskOutput{}, errors.New("unknown execution seat: " + seatID)
				}
				out, err := services.Lifecycle.Seat(ctx, SeatInput{ID: id, Seat: seatID})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardLink}, func(ctx context.Context, in opapi.WorkboardLinkRequest) (TaskOutput, error) {
				out, err := provider(ctx).Relations.Link(ctx, LinkInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Type: text(in.Type), Target: text(in.Target)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardPolicy}, func(ctx context.Context, in opapi.WorkboardPolicyRequest) (TaskOutput, error) {
				id := text(in.ID)
				if id == "" {
					return TaskOutput{}, errors.New("workboard_policy requires id")
				}
				var policy *tasks.RetryPolicy
				if !in.Clear {
					if !in.MaxAttempts.Present() {
						return TaskOutput{}, errors.New("workboard_policy requires max_attempts or clear")
					}
					max := int(in.MaxAttempts.Number())
					if max < 1 {
						return TaskOutput{}, errors.New("workboard_policy max_attempts must be positive")
					}
					policy = &tasks.RetryPolicy{MaxAttempts: max, EscalateTo: text(in.EscalateTo)}
				}
				out, err := provider(ctx).Relations.Policy(ctx, PolicyInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: id, Actor: text(in.Actor), Policy: policy})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardDepend}, func(ctx context.Context, in opapi.WorkboardDependRequest) (TaskOutput, error) {
				parent := text(in.DependsOn)
				if parent == "" {
					parent = text(in.On)
				}
				out, err := provider(ctx).Relations.Depend(ctx, DependInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), DependsOn: parent})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardReclaim}, func(ctx context.Context, in opapi.WorkboardReclaimRequest) (TaskOutput, error) {
				out, err := provider(ctx).Relations.Reclaim(ctx, ReclaimInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: text(in.ID), Actor: text(in.Actor), StaleAfterMS: admittedLimit(in.StaleAfterMS, 10*60*1000)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardSweep}, func(ctx context.Context, in opapi.WorkboardSweepRequest) (SweepOutput, error) {
				limit := admittedLimit(in.Limit, 100)
				if limit > 1000 {
					limit = 1000
				}
				actor := text(in.Actor)
				if actor == "" {
					actor = "workboard-sweeper"
				}
				return provider(ctx).Relations.Sweep(ctx, SweepInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), Actor: actor, StaleAfterMS: admittedLimit(in.StaleAfterMS, 10*60*1000), Limit: limit})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardWatch, ReadOnly: true}, func(ctx context.Context, in opapi.WorkboardWatchRequest) (WatchOutput, error) {
				id := text(in.ID)
				if id == "" {
					return WatchOutput{}, errors.New("workboard_watch requires id")
				}
				limit := admittedLimit(in.Limit, 50)
				if limit > 200 {
					limit = 200
				}
				return provider(ctx).Watch.Watch(ctx, WatchInput{ID: id, RunID: text(in.RunID), Limit: limit})
			})
		},
		func() error {
			return bindRequest(&ops, opapi.Spec{Name: opapi.WorkboardDispatch}, func(ctx context.Context, in opapi.WorkboardDispatchRequest) (DispatchOutput, error) {
				out, err := provider(ctx).Dispatch.Dispatch(ctx, DispatchInput{ID: text(in.ID), Agent: text(in.Agent), Reason: text(in.Reason), Intent: text(in.Intent)})
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
