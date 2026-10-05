// SPDX-License-Identifier: MIT

package taste

import (
	"context"
	"errors"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// Request fields retain the native protocol's lenient admission. Store use cases
// receive the normalized typed inputs, and result schemas use actual store fields.
type ListRequestInput struct {
	Scope any `json:"scope,omitempty"`
	Tag   any `json:"tag,omitempty"`
	Limit any `json:"limit,omitempty"`
}
type CreateRequestInput struct {
	Title any `json:"title,omitempty"`
	Body  any `json:"body,omitempty"`
	Scope any `json:"scope,omitempty"`
	Tags  any `json:"tags,omitempty"`
}
type DeleteRequestInput struct {
	ID any `json:"id,omitempty"`
}

func text(raw any) string {
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}
func listLimit(raw any) int {
	n := 200
	switch v := raw.(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	case int64:
		n = int(v)
	}
	if n < 1 {
		n = 200
	}
	return n
}
func tags(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, raw := range values {
			if value := text(raw); value != "" {
				out = append(out, value)
			}
		}
		return out
	default:
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return strings.Split(value, ",")
		}
		return nil
	}
}
func bind[I, O any](operations *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	operation, err := app.NewOperation(spec, handler)
	if err == nil {
		*operations = append(*operations, operation)
	}
	return err
}
func Operations(service func(context.Context) *Service) ([]app.Operation, error) {
	if service == nil {
		return nil, errors.New("taste service provider required")
	}
	var operations []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&operations, opapi.Spec{Name: "taste_list", ReadOnly: true}, func(ctx context.Context, in ListRequestInput) (ListOutput, error) {
				return service(ctx).List(ctx, ListInput{Scope: text(in.Scope), Tag: text(in.Tag), Limit: listLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "taste_create"}, func(ctx context.Context, in CreateRequestInput) (CreateOutput, error) {
				return service(ctx).Create(ctx, CreateInput{Title: text(in.Title), Body: text(in.Body), Scope: text(in.Scope), Tags: tags(in.Tags)})
			})
		},
		func() error {
			return bind(&operations, opapi.Spec{Name: "taste_delete"}, func(ctx context.Context, in DeleteRequestInput) (DeleteOutput, error) {
				return service(ctx).Delete(ctx, DeleteInput{ID: text(in.ID)})
			})
		},
	}
	for _, binding := range bindings {
		if err := binding(); err != nil {
			return nil, err
		}
	}
	return operations, nil
}
