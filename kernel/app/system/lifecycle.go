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

// Lifecycle halts and resumes new runs, verifies the journal and stops the
// daemon. Shutdown schedules the daemon's exit after the response is sent.
//
// The service takes the daemon's GATE — the halt, the resume and the
// hash-chain check — and not the handle those three doors live behind: a
// service that needs three verbs does not need the whole kernel.
type Lifecycle struct {
	gate     Gate
	shutdown func()
}

// NewLifecycle binds the daemon's gate and the daemon's exit.
func NewLifecycle(gate Gate, shutdown func()) *Lifecycle {
	return &Lifecycle{gate: gate, shutdown: shutdown}
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

// gateOrRefuse returns the daemon's gate, or the refusal.
//
// A host with no daemon holds no gate: halting a daemon that is not there is
// not a halt, and the refusal is the honest answer.
func (l *Lifecycle) gateOrRefuse() (Gate, error) {
	if l.gate == nil {
		return nil, ErrNoGate
	}
	return l.gate, nil
}

// Halt stops new runs, recording why.
func (l *Lifecycle) Halt(_ context.Context, in ReasonRequest) (GateOutput, error) {
	gate, err := l.gateOrRefuse()
	if err != nil {
		return GateOutput{}, err
	}
	r, err := reason(in.Reason)
	if err != nil {
		return GateOutput{}, err
	}
	gate.HaltWith(r)
	return GateOutput{OK: true, Halted: true, Reason: r}, nil
}

// Resume allows new runs again, recording why.
func (l *Lifecycle) Resume(_ context.Context, in ReasonRequest) (GateOutput, error) {
	gate, err := l.gateOrRefuse()
	if err != nil {
		return GateOutput{}, err
	}
	r, err := reason(in.Reason)
	if err != nil {
		return GateOutput{}, err
	}
	gate.ResumeWith(r)
	return GateOutput{OK: true, Reason: r}, nil
}

// Verify checks the primary journal's hash chain.
func (l *Lifecycle) Verify(context.Context, EmptyRequest) (OKOutput, error) {
	gate, err := l.gateOrRefuse()
	if err != nil {
		return OKOutput{}, err
	}
	if err := gate.Verify(); err != nil {
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
