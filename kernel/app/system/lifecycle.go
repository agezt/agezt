// SPDX-License-Identifier: MIT

package system

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Kernel is the primary kernel's run gate and journal check.
type Kernel interface {
	HaltWith(reason string)
	ResumeWith(reason string)
	Verify() error
}

// Lifecycle halts and resumes new runs, verifies the journal and stops the
// daemon. Shutdown schedules the daemon's exit after the response is sent.
type Lifecycle struct {
	kernel   Kernel
	shutdown func()
}

func NewLifecycle(kernel Kernel, shutdown func()) *Lifecycle {
	return &Lifecycle{kernel: kernel, shutdown: shutdown}
}

type ReasonRequest struct {
	Reason json.RawMessage `json:"reason,omitempty"`
}

// EmptyRequest takes no arguments.
type EmptyRequest struct{}

type GateOutput struct {
	OK     bool   `json:"ok"`
	Halted bool   `json:"halted"`
	Reason string `json:"reason"`
}

type OKOutput struct {
	OK bool `json:"ok"`
}

// reason reads a strict string: absent is empty, present must be a string.
func reason(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	s, ok := v.(string)
	if !ok {
		return "", errors.New("args.reason must be a string")
	}
	return s, nil
}

// Halt stops new runs, recording why.
func (l *Lifecycle) Halt(_ context.Context, in ReasonRequest) (GateOutput, error) {
	r, err := reason(in.Reason)
	if err != nil {
		return GateOutput{}, err
	}
	l.kernel.HaltWith(r)
	return GateOutput{OK: true, Halted: true, Reason: r}, nil
}

// Resume allows new runs again, recording why.
func (l *Lifecycle) Resume(_ context.Context, in ReasonRequest) (GateOutput, error) {
	r, err := reason(in.Reason)
	if err != nil {
		return GateOutput{}, err
	}
	l.kernel.ResumeWith(r)
	return GateOutput{OK: true, Reason: r}, nil
}

// Verify checks the primary journal's hash chain.
func (l *Lifecycle) Verify(context.Context, EmptyRequest) (OKOutput, error) {
	if err := l.kernel.Verify(); err != nil {
		return OKOutput{}, err
	}
	return OKOutput{OK: true}, nil
}

// Shutdown acknowledges, then schedules the daemon's exit.
func (l *Lifecycle) Shutdown(context.Context, EmptyRequest) (OKOutput, error) {
	l.shutdown()
	return OKOutput{OK: true}, nil
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

// LifecycleOperations declares the four operator-only lifecycle operations;
// halt, resume and shutdown are audited.
func LifecycleOperations(provider func(context.Context) *Lifecycle) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("lifecycle provider required")
	}
	reasonSchema := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"reason":{}}}`)
	none := json.RawMessage(`{"type":"object","additionalProperties":true}`)
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bindPrimary(&ops, opapi.Spec{Name: "halt", InputSchema: reasonSchema, HTTP: opapi.HTTP{Method: "POST", Path: "/api/halt"}}, func(ctx context.Context, in ReasonRequest) (GateOutput, error) {
				return provider(ctx).Halt(ctx, in)
			})
		},
		func() error {
			return bindPrimary(&ops, opapi.Spec{Name: "resume", InputSchema: reasonSchema, HTTP: opapi.HTTP{Method: "POST", Path: "/api/resume"}}, func(ctx context.Context, in ReasonRequest) (GateOutput, error) {
				return provider(ctx).Resume(ctx, in)
			})
		},
		func() error {
			return bindPrimary(&ops, opapi.Spec{Name: "journal_verify", ReadOnly: true, InputSchema: none}, func(ctx context.Context, in EmptyRequest) (OKOutput, error) {
				return provider(ctx).Verify(ctx, in)
			})
		},
		func() error {
			return bindPrimary(&ops, opapi.Spec{Name: "shutdown", InputSchema: none}, func(ctx context.Context, in EmptyRequest) (OKOutput, error) {
				return provider(ctx).Shutdown(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
