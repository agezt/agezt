// SPDX-License-Identifier: MIT

package board

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/boardapi"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"strings"
)

// The board's request rows live in the contract (kernel/contract/opapi): the
// topic, the cursor and the recipient are text, the page size is a number, and
// the "everything" flag is a boolean. The read routes carry their arguments as
// query arguments, so `limit` is a number there too: the query proxy coerces
// it, and a limit that is not a number is a usage error the door names
// instead of a page size the caller never asked for.
//
// SendRequestInput is the body of the board_send operation — the wire row,
// which lives in the board's own contract (kernel/contract/boardapi): the
// REST surface decodes the same row, and the contract names its fields.
type SendRequestInput = boardapi.Draft

func trim(value string) string { return strings.TrimSpace(value) }

// admittedLimit reads the page size the row asked for: the family's default
// when it did not ask for one, the cap when it asked for too many.
//
// A number above zero is the caller's page size — even a fraction, which the
// store reads as "no page at all"; a number at or below zero is no page size
// the caller asked for.
func admittedLimit(limit opapi.Limit) int {
	size := 50
	if limit.Present() && limit.Number() > 0 {
		size = int(limit.Number())
	}
	if size > 500 {
		size = 500
	}
	return size
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
			return bind(&ops, opapi.Spec{Name: opapi.BoardRead, ReadOnly: true}, func(ctx context.Context, in opapi.BoardReadRequest) (ReadOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return ReadOutput{}, err
				}
				return service.Read(ctx, ReadInput{Topic: in.Topic, Cursor: in.Cursor, Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardHelp, ReadOnly: true}, func(ctx context.Context, in opapi.BoardHelpRequest) (HelpOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return HelpOutput{}, err
				}
				return service.Help(ctx, LimitInput{Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardInbox, ReadOnly: true}, func(ctx context.Context, in opapi.BoardInboxRequest) (InboxOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return InboxOutput{}, err
				}
				return service.Inbox(ctx, InboxInput{To: trim(in.To), All: in.All, Limit: admittedLimit(in.Limit)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardGet, ReadOnly: true}, func(ctx context.Context, in opapi.BoardGetRequest) (GetOutput, error) {
				service, err := reads(ctx)
				if err != nil {
					return GetOutput{}, err
				}
				return service.Get(ctx, GetInput{ID: trim(in.ID)})
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: opapi.BoardReplies, ReadOnly: true}, func(ctx context.Context, in opapi.BoardRepliesRequest) (RepliesOutput, error) {
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
			return bind(&ops, opapi.Spec{Name: opapi.BoardAck}, func(ctx context.Context, in opapi.BoardAckRequest) (AckOutput, error) {
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
