// SPDX-License-Identifier: MIT

package board

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

// The request inputs are the console's own contract: every field names the
// type it is read as, so a mistyped body is refused at the door instead of
// arriving as a silent default. The read routes carry their arguments as
// query arguments, so `limit` is a number there too: the query proxy coerces
// it, and a limit that is not a number is a usage error the door names
// instead of a page size the caller never asked for.
type ReadRequestInput struct {
	Topic  string `json:"topic,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  any    `json:"limit,omitempty"`
}
type LimitRequestInput struct {
	Limit any `json:"limit,omitempty"`
}
type InboxRequestInput struct {
	To    string `json:"to,omitempty"`
	All   bool   `json:"all,omitempty"`
	Limit any    `json:"limit,omitempty"`
}
type GetRequestInput struct {
	ID string `json:"id,omitempty"`
}
type RepliesRequestInput struct {
	ID    string `json:"id,omitempty"`
	Limit any    `json:"limit,omitempty"`
}
type AckRequestInput struct {
	ID string `json:"id,omitempty"`
	By string `json:"by,omitempty"`
}
type SendRequestInput struct {
	Text          string `json:"text,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	Topic         string `json:"topic,omitempty"`
	ReplyTo       string `json:"reply_to,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Help          bool   `json:"help,omitempty"`
}

// The board's read bodies name the type each field is read as: the topic,
// the cursor and the recipient are text, the page size is a number, and the
// "everything" flag is a boolean.
var (
	readRequestSchema    = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"topic":{"type":"string"},"cursor":{"type":"string"},"limit":{"type":"number"}}}`)
	limitRequestSchema   = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"limit":{"type":"number"}}}`)
	inboxRequestSchema   = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"to":{"type":"string"},"all":{"type":"boolean"},"limit":{"type":"number"}}}`)
	repliesRequestSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"id":{"type":"string"},"limit":{"type":"number"}}}`)
)

func trim(value string) string { return strings.TrimSpace(value) }
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
			return bind(&ops, opapi.Spec{Name: opapi.BoardRead, ReadOnly: true, InputSchema: readRequestSchema}, func(ctx context.Context, in ReadRequestInput) (ReadOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return ReadOutput{}, err
				}
				return service.Read(ctx, ReadInput{Topic: in.Topic, Cursor: in.Cursor, Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardHelp, ReadOnly: true, InputSchema: limitRequestSchema}, func(ctx context.Context, in LimitRequestInput) (HelpOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return HelpOutput{}, err
				}
				return service.Help(ctx, LimitInput{Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardInbox, ReadOnly: true, InputSchema: inboxRequestSchema}, func(ctx context.Context, in InboxRequestInput) (InboxOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return InboxOutput{}, err
				}
				return service.Inbox(ctx, InboxInput{To: trim(in.To), All: in.All, Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardGet, ReadOnly: true}, func(ctx context.Context, in GetRequestInput) (GetOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return GetOutput{}, err
				}
				return service.Get(ctx, GetInput{ID: trim(in.ID)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardReplies, ReadOnly: true, InputSchema: repliesRequestSchema}, func(ctx context.Context, in RepliesRequestInput) (RepliesOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return RepliesOutput{}, err
				}
				return service.Replies(ctx, RepliesInput{ID: trim(in.ID), Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardSend}, func(ctx context.Context, in SendRequestInput) (SendOutput, error) {
				service, err := writes(ctx)
				if err != nil {
					return SendOutput{}, err
				}
				return service.Send(ctx, SendInput{Text: trim(in.Text), From: trim(in.From), To: trim(in.To), Topic: trim(in.Topic), ReplyTo: trim(in.ReplyTo), CorrelationID: trim(in.CorrelationID), Help: in.Help})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardAck}, func(ctx context.Context, in AckRequestInput) (AckOutput, error) {
				service, err := writes(ctx)
				if err != nil {
					return AckOutput{}, err
				}
				return service.Ack(ctx, AckInput{ID: trim(in.ID), By: trim(in.By)})
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
