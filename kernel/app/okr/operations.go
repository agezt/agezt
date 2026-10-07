// SPDX-License-Identifier: MIT
package okr

import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	objectives "github.com/agezt/agezt/kernel/okr"
	"strings"
)

type ListRequestInput struct {
	Status          any  `json:"status,omitempty"`
	Tenant          any  `json:"tenant,omitempty"`
	IncludeArchived bool `json:"include_archived,omitempty"`
	Limit           any  `json:"limit,omitempty"`
}
type ShowRequestInput struct {
	ID any `json:"id,omitempty"`
}
type CreateRequestInput struct {
	Title         any `json:"title,omitempty"`
	Description   any `json:"description,omitempty"`
	Owner         any `json:"owner,omitempty"`
	Tenant        any `json:"tenant,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}
type KeyResultRequestInput struct {
	ID            any `json:"id,omitempty"`
	Title         any `json:"title,omitempty"`
	Target        any `json:"target,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}
type LinkRequestInput struct {
	ID            any `json:"id,omitempty"`
	KeyResult     any `json:"key_result,omitempty"`
	Task          any `json:"task,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}
type ArchiveRequestInput struct {
	ID            any `json:"id,omitempty"`
	CorrelationID any `json:"correlation_id,omitempty"`
}

func requestText(raw any) string { value, _ := raw.(string); return strings.TrimSpace(value) }
func requestNumber(raw any) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	}
	return 0
}
func requestLimit(raw any) int {
	limit := 200
	switch value := raw.(type) {
	case float64:
		limit = int(value)
	case int:
		limit = value
	case int64:
		limit = int(value)
	}
	if limit < 1 {
		return 200
	}
	return limit
}
func requestCorrelation(ctx context.Context, raw any) string {
	if value := requestText(raw); value != "" {
		return value
	}
	return opapi.CorrelationFromContext(ctx)
}
func nativeResult(id any, out ObjectiveOutput, err error) (ObjectiveOutput, error) {
	if errors.Is(err, objectives.ErrNotFound) {
		err = fmt.Errorf("unknown objective: %s", requestText(id))
	}
	return out, err
}
func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	op, err := app.NewOperation(spec, handler)
	if err == nil {
		*ops = append(*ops, op)
	}
	return err
}
func Operations(reads func(context.Context) *Service, writes func(context.Context) *Lifecycle) ([]app.Operation, error) {
	if reads == nil || writes == nil {
		return nil, errors.New("OKR service providers required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_list", ReadOnly: true}, func(ctx context.Context, in ListRequestInput) (ListOutput, error) {
				return reads(ctx).List(ctx, ListInput{Status: objectives.Status(requestText(in.Status)), Tenant: requestText(in.Tenant), IncludeArchived: in.IncludeArchived, Limit: requestLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_show", ReadOnly: true}, func(ctx context.Context, in ShowRequestInput) (ShowOutput, error) {
				return reads(ctx).Show(ctx, ShowInput{ID: requestText(in.ID)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_create"}, func(ctx context.Context, in CreateRequestInput) (ObjectiveOutput, error) {
				title := requestText(in.Title)
				if title == "" {
					return ObjectiveOutput{}, errors.New("okr_create requires title")
				}
				return writes(ctx).Create(ctx, CreateInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), Spec: objectives.CreateSpec{Title: title, Description: requestText(in.Description), Owner: requestText(in.Owner), Tenant: requestText(in.Tenant)}})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_keyresult"}, func(ctx context.Context, in KeyResultRequestInput) (ObjectiveOutput, error) {
				id, title := requestText(in.ID), requestText(in.Title)
				if id == "" || title == "" {
					return ObjectiveOutput{}, errors.New("okr_keyresult requires id and title")
				}
				out, err := writes(ctx).KeyResult(ctx, KeyResultInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: id, Title: title, Target: requestNumber(in.Target)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_link"}, func(ctx context.Context, in LinkRequestInput) (ObjectiveOutput, error) {
				out, err := writes(ctx).Link(ctx, LinkInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), KeyResult: requestText(in.KeyResult), Task: requestText(in.Task)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_unlink"}, func(ctx context.Context, in LinkRequestInput) (ObjectiveOutput, error) {
				out, err := writes(ctx).Unlink(ctx, LinkInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID), KeyResult: requestText(in.KeyResult), Task: requestText(in.Task)})
				return nativeResult(in.ID, out, err)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "okr_archive"}, func(ctx context.Context, in ArchiveRequestInput) (ObjectiveOutput, error) {
				out, err := writes(ctx).Archive(ctx, ArchiveInput{CorrelationID: requestCorrelation(ctx, in.CorrelationID), ID: requestText(in.ID)})
				return nativeResult(in.ID, out, err)
			})
		},
	}
	for _, binding := range bindings {
		if err := binding(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
