// SPDX-License-Identifier: MIT

// Package app implements the transport-independent typed operation pipeline.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Operation binds a typed handler and immutable metadata, before transport use.
type Operation struct {
	spec   opapi.Spec
	decode func(json.RawMessage) (any, error)
	run    func(context.Context, any, opapi.Emitter) (any, error)
}

func (o Operation) Spec() opapi.Spec {
	spec := o.spec
	spec.InputSchema = append(json.RawMessage(nil), spec.InputSchema...)
	spec.OutputSchema = append(json.RawMessage(nil), spec.OutputSchema...)
	return spec
}

// NewOperation derives its input/output metadata from the actual Go handler.
func NewOperation[I, O any](spec opapi.Spec, handler func(context.Context, I) (O, error)) (Operation, error) {
	if handler == nil {
		return Operation{}, fmt.Errorf("operation %q has no handler", spec.Name)
	}
	if spec.Stream != opapi.StreamNone {
		return Operation{}, fmt.Errorf("operation %q needs a streaming handler", spec.Name)
	}
	return NewStreamingOperation(spec, func(ctx context.Context, in I, _ func(O) error) (O, error) { return handler(ctx, in) })
}

// NewStreamingOperation retains the typed input/output contract while passing a
// checked emitter to the handler; the final return value is its terminal output.
// StreamNone is a terminal-only mode, also used by NewOperation's unary wrapper.
func NewStreamingOperation[I, O any](spec opapi.Spec, handler func(context.Context, I, func(O) error) (O, error)) (Operation, error) {
	if handler == nil {
		return Operation{}, fmt.Errorf("operation %q needs a handler", spec.Name)
	}
	return bindOperation[I, O](spec, func(ctx context.Context, in I, emitter opapi.Emitter) (O, error) {
		return handler(ctx, in, func(value O) error {
			if emitter == nil {
				return fmt.Errorf("operation %q has no emitter", spec.Name)
			}
			return emitter.Emit(ctx, value)
		})
	})
}

func bindOperation[I, O any](spec opapi.Spec, handler func(context.Context, I, opapi.Emitter) (O, error)) (Operation, error) {
	if spec.Name == "" || spec.Authz > opapi.OwnTenant || spec.Tenancy > opapi.CallerTenant || spec.Stream > opapi.StreamLive {
		return Operation{}, fmt.Errorf("invalid operation metadata for %q", spec.Name)
	}
	if spec.Authz == opapi.OwnTenant && spec.Tenancy != opapi.CallerTenant {
		return Operation{}, fmt.Errorf("tenant operation %q must route to its caller", spec.Name)
	}
	spec.Input, spec.Output = reflect.TypeFor[I](), reflect.TypeFor[O]()
	var err error
	spec.InputSchema, err = schema.FromType(spec.Input, spec.AllowUnknownInput)
	if err != nil {
		return Operation{}, fmt.Errorf("operation %q input: %w", spec.Name, err)
	}
	spec.OutputSchema, err = schema.FromType(spec.Output, true)
	if err != nil {
		return Operation{}, fmt.Errorf("operation %q output: %w", spec.Name, err)
	}
	return Operation{
		spec: spec,
		decode: func(raw json.RawMessage) (any, error) {
			if err := schema.ValidateJSON(spec.InputSchema, raw); err != nil {
				return nil, err
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			if !spec.AllowUnknownInput {
				decoder.DisallowUnknownFields()
			}
			var input I
			if err := decoder.Decode(&input); err != nil {
				return nil, err
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				return nil, fmt.Errorf("input must contain one JSON value")
			}
			return input, nil
		},
		run: func(ctx context.Context, input any, emitter opapi.Emitter) (any, error) {
			return handler(ctx, input.(I), emitter)
		},
	}, nil
}
