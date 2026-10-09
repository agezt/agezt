// SPDX-License-Identifier: MIT

// Package seats manages the execution seats a workboard task can be dispatched
// under: the seeded built-ins plus operator-defined custom seats, each pinning
// an isolation profile, a model chain and an optional tool allowlist.
package seats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/seat"
)

// Store is the primary kernel's seat store.
type Store interface {
	List() []seat.Seat
	Create(spec seat.Seat) (seat.Seat, error)
	Delete(id string) error
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

func decode(raw json.RawMessage) any {
	var v any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// text is a trimmed string argument; any other value reads as "".
func text(raw json.RawMessage) string {
	str, _ := decode(raw).(string)
	return strings.TrimSpace(str)
}

// list reads an array of strings, keeping the trimmed non-blank ones, or a
// non-blank comma-separated string, split as given; anything else is none.
func list(raw json.RawMessage) []string {
	switch xs := decode(raw).(type) {
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
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

type ListRequest struct{}

type ListOutput struct {
	Seats []seat.Seat `json:"seats"`
	Count int         `json:"count"`
}

func (s *Service) List(_ context.Context, _ ListRequest) (ListOutput, error) {
	seats := append([]seat.Seat{}, s.store.List()...)
	return ListOutput{Seats: seats, Count: len(seats)}, nil
}

type CreateRequest struct {
	ID               json.RawMessage `json:"id,omitempty"`
	Name             json.RawMessage `json:"name,omitempty"`
	Description      json.RawMessage `json:"description,omitempty"`
	ExecutionProfile json.RawMessage `json:"execution_profile,omitempty"`
	ModelChain       json.RawMessage `json:"model_chain,omitempty"`
	Tools            json.RawMessage `json:"tools,omitempty"`
	RestrictTools    json.RawMessage `json:"restrict_tools,omitempty"`
}

type SeatOutput struct {
	Seat seat.Seat `json:"seat"`
}

// Create adds a custom seat; the store validates and normalises it.
// restrict_tools, when present, must be a boolean.
func (s *Service) Create(_ context.Context, in CreateRequest) (SeatOutput, error) {
	spec := seat.Seat{
		ID:               text(in.ID),
		Name:             text(in.Name),
		Description:      text(in.Description),
		ExecutionProfile: text(in.ExecutionProfile),
		ModelChain:       list(in.ModelChain),
		Tools:            list(in.Tools),
	}
	if len(in.RestrictTools) > 0 {
		b, ok := decode(in.RestrictTools).(bool)
		if !ok {
			return SeatOutput{}, fmt.Errorf("args.restrict_tools must be a boolean")
		}
		spec.RestrictTools = b
	}
	made, err := s.store.Create(spec)
	if err != nil {
		return SeatOutput{}, err
	}
	return SeatOutput{Seat: made}, nil
}

type DeleteRequest struct {
	ID json.RawMessage `json:"id,omitempty"`
}

type DeleteOutput struct {
	Deleted string `json:"deleted"`
}

// Delete removes a custom seat; the store refuses built-ins.
func (s *Service) Delete(_ context.Context, in DeleteRequest) (DeleteOutput, error) {
	id := text(in.ID)
	if id == "" {
		return DeleteOutput{}, errors.New("seat_delete requires id")
	}
	if err := s.store.Delete(id); err != nil {
		return DeleteOutput{}, err
	}
	return DeleteOutput{Deleted: id}, nil
}

// Operations declares the read-only list and the two audited edits, all
// operator-only and without Web UI routes.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("seat provider required")
	}
	listOut, err := schema.FromType(reflect.TypeFor[ListOutput](), false)
	if err != nil {
		return nil, err
	}
	seatOut, err := schema.FromType(reflect.TypeFor[SeatOutput](), false)
	if err != nil {
		return nil, err
	}
	deleteOut, err := schema.FromType(reflect.TypeFor[DeleteOutput](), false)
	if err != nil {
		return nil, err
	}
	listOp, err := app.NewOperation(opapi.Spec{Name: "seat_list", ReadOnly: true, OutputSchema: listOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true}, func(ctx context.Context, in ListRequest) (ListOutput, error) {
		return provider(ctx).List(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	createOp, err := app.NewOperation(opapi.Spec{Name: "seat_create", OutputSchema: seatOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{},"name":{},"description":{},"execution_profile":{},"model_chain":{},"tools":{},"restrict_tools":{}}}`)}, func(ctx context.Context, in CreateRequest) (SeatOutput, error) {
		return provider(ctx).Create(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	deleteOp, err := app.NewOperation(opapi.Spec{Name: "seat_delete", OutputSchema: deleteOut, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{}}}`)}, func(ctx context.Context, in DeleteRequest) (DeleteOutput, error) {
		return provider(ctx).Delete(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{listOp, createOp, deleteOp}, nil
}
