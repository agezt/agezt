// SPDX-License-Identifier: MIT

package board

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

type ReadRequestInput struct {
	Topic  string `json:"topic,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  any    `json:"limit,omitempty"`
}
type LimitRequestInput struct {
	Limit any `json:"limit,omitempty"`
}
type InboxRequestInput struct {
	To    any  `json:"to,omitempty"`
	All   bool `json:"all,omitempty"`
	Limit any  `json:"limit,omitempty"`
}
type GetRequestInput struct {
	ID any `json:"id,omitempty"`
}
type RepliesRequestInput struct {
	ID    any `json:"id,omitempty"`
	Limit any `json:"limit,omitempty"`
}
type AckRequestInput struct {
	ID any `json:"id,omitempty"`
	By any `json:"by,omitempty"`
}
type SendRequestInput struct {
	Text          any  `json:"text,omitempty"`
	From          any  `json:"from,omitempty"`
	To            any  `json:"to,omitempty"`
	Topic         any  `json:"topic,omitempty"`
	ReplyTo       any  `json:"reply_to,omitempty"`
	CorrelationID any  `json:"correlation_id,omitempty"`
	Help          bool `json:"help,omitempty"`
}

func text(raw any) string { value, _ := raw.(string); return strings.TrimSpace(value) }
func admittedLimit(raw any) int {
	limit := 50
	if value, ok := raw.(float64); ok && value > 0 {
		limit = int(value)
	}
	if limit > 500 {
		limit = 500
	}
	return limit
}
func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	spec.AllowUnknownInput = true
	op, err := app.NewOperation(spec, handler)
	if err == nil {
		*ops = append(*ops, op)
	}
	return err
}
func Operations(reads func(context.Context) (*Service, error), writes func(context.Context) (*Service, error)) ([]app.Operation, error) {
	if reads == nil || writes == nil {
		return nil, errors.New("board service providers required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_read", ReadOnly: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/board"}}, func(ctx context.Context, in ReadRequestInput) (ReadOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return ReadOutput{}, err
				}
				return service.Read(ctx, ReadInput{Topic: in.Topic, Cursor: in.Cursor, Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_help", ReadOnly: true, HTTP: opapi.HTTP{Method: "GET", Path: "/api/board/help"}}, func(ctx context.Context, in LimitRequestInput) (HelpOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return HelpOutput{}, err
				}
				return service.Help(ctx, LimitInput{Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_inbox", ReadOnly: true}, func(ctx context.Context, in InboxRequestInput) (InboxOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return InboxOutput{}, err
				}
				return service.Inbox(ctx, InboxInput{To: text(in.To), All: in.All, Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_get", ReadOnly: true}, func(ctx context.Context, in GetRequestInput) (GetOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return GetOutput{}, err
				}
				return service.Get(ctx, GetInput{ID: text(in.ID)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_replies", ReadOnly: true}, func(ctx context.Context, in RepliesRequestInput) (RepliesOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return RepliesOutput{}, err
				}
				return service.Replies(ctx, RepliesInput{ID: text(in.ID), Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_send", HTTP: opapi.HTTP{Method: "POST", Path: "/api/board/send"}}, func(ctx context.Context, in SendRequestInput) (SendOutput, error) {
				service, err := writes(ctx)
				if err != nil {
					return SendOutput{}, err
				}
				return service.Send(ctx, SendInput{Text: text(in.Text), From: text(in.From), To: text(in.To), Topic: text(in.Topic), ReplyTo: text(in.ReplyTo), CorrelationID: text(in.CorrelationID), Help: in.Help})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "board_ack", HTTP: opapi.HTTP{Method: "POST", Path: "/api/board/ack"}}, func(ctx context.Context, in AckRequestInput) (AckOutput, error) {
				service, err := writes(ctx)
				if err != nil {
					return AckOutput{}, err
				}
				return service.Ack(ctx, AckInput{ID: text(in.ID), By: text(in.By)})
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
