// SPDX-License-Identifier: MIT

package steer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/intervention"
	"github.com/agezt/agezt/kernel/platform/schema"
)

// Runs is the routed kernel's live-run control surface.
type Runs interface {
	PauseRun(corr string) bool
	ResumeRun(corr string) bool
	StepRun(corr string) bool
	SteerRun(corr, directive string, note bool) bool
	InterveneRun(intervention.Request) (intervention.Result, error)
}

// RunRequest names one in-flight run. Arguments stay raw so the legacy strict
// string codecs and their error order hold.
type RunRequest struct {
	Correlation json.RawMessage `json:"correlation,omitempty"`
}

type SteerRequest struct {
	Correlation json.RawMessage `json:"correlation,omitempty"`
	Directive   json.RawMessage `json:"directive,omitempty"`
	Mode        json.RawMessage `json:"mode,omitempty"`
}

type InterveneRequest struct {
	Correlation    json.RawMessage `json:"correlation,omitempty"`
	Primitive      json.RawMessage `json:"primitive,omitempty"`
	Directive      json.RawMessage `json:"directive,omitempty"`
	Scope          json.RawMessage `json:"scope,omitempty"`
	IdempotencyKey json.RawMessage `json:"idempotency_key,omitempty"`
	LeaseMS        json.RawMessage `json:"lease_ms,omitempty"`
}

type ControlOutput struct {
	Correlation string `json:"correlation"`
	OK          bool   `json:"ok"`
}

type SteerOutput struct {
	Correlation string `json:"correlation"`
	Mode        string `json:"mode"`
	Accepted    bool   `json:"accepted"`
}

type InterveneOutput struct {
	Primitive        string `json:"primitive"`
	Correlation      string `json:"correlation"`
	Accepted         bool   `json:"accepted"`
	Applied          bool   `json:"applied"`
	State            string `json:"state"`
	Paused           bool   `json:"paused"`
	Pending          int    `json:"pending"`
	IdempotencyKey   string `json:"idempotency_key"`
	Reason           string `json:"reason"`
	LeaseExpiresUnix *int64 `json:"lease_expires_unix,omitempty"`
}

func rawValue(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v, true
}

// optionalString is a strict string: absent is empty, present must be a string.
func optionalString(raw json.RawMessage, key string) (string, error) {
	v, present := rawValue(raw)
	if !present {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("args.%s must be a string", key)
	}
	return s, nil
}

// requiredString rejects a blank value but returns it untrimmed.
func requiredString(raw json.RawMessage, key string) (string, error) {
	s, err := optionalString(raw, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return s, nil
}

// Service steers runs on the kernel the request was routed to.
type Service struct{ runs Runs }

func New(runs Runs) *Service { return &Service{runs: runs} }

func (s *Service) control(in RunRequest, act func(string) bool) (ControlOutput, error) {
	corr, err := requiredString(in.Correlation, "correlation")
	if err != nil {
		return ControlOutput{}, err
	}
	return ControlOutput{Correlation: corr, OK: act(corr)}, nil
}

func (s *Service) Pause(_ context.Context, in RunRequest) (ControlOutput, error) {
	return s.control(in, s.runs.PauseRun)
}

func (s *Service) Resume(_ context.Context, in RunRequest) (ControlOutput, error) {
	return s.control(in, s.runs.ResumeRun)
}

func (s *Service) Step(_ context.Context, in RunRequest) (ControlOutput, error) {
	return s.control(in, s.runs.StepRun)
}

// Steer injects a directive: mode "note" is a soft BTW (read it, stay on task);
// anything else is a forceful steer that re-prioritises.
func (s *Service) Steer(_ context.Context, in SteerRequest) (SteerOutput, error) {
	corr, err := requiredString(in.Correlation, "correlation")
	if err != nil {
		return SteerOutput{}, err
	}
	directive, err := requiredString(in.Directive, "directive")
	if err != nil {
		return SteerOutput{}, err
	}
	mode, err := optionalString(in.Mode, "mode")
	if err != nil {
		return SteerOutput{}, err
	}
	note := mode == "note"
	out := SteerOutput{Correlation: corr, Mode: "steer", Accepted: s.runs.SteerRun(corr, directive, note)}
	if note {
		out.Mode = "note"
	}
	return out, nil
}

func (s *Service) Intervene(_ context.Context, in InterveneRequest) (InterveneOutput, error) {
	corr, err := requiredString(in.Correlation, "correlation")
	if err != nil {
		return InterveneOutput{}, err
	}
	var fields [4]string
	for i, f := range []struct {
		raw json.RawMessage
		key string
	}{{in.Primitive, "primitive"}, {in.Directive, "directive"}, {in.Scope, "scope"}, {in.IdempotencyKey, "idempotency_key"}} {
		if fields[i], err = optionalString(f.raw, f.key); err != nil {
			return InterveneOutput{}, err
		}
	}
	var lease time.Duration
	if v, present := rawValue(in.LeaseMS); present {
		ms, ok := v.(float64)
		if !ok {
			return InterveneOutput{}, errors.New("args.lease_ms must be a number")
		}
		if ms > 0 {
			lease = time.Duration(ms) * time.Millisecond
		}
	}
	res, err := s.runs.InterveneRun(intervention.Request{
		Primitive:      intervention.Primitive(fields[0]),
		CorrelationID:  corr,
		Directive:      fields[1],
		Lease:          lease,
		Scope:          fields[2],
		IdempotencyKey: fields[3],
	})
	if err != nil {
		return InterveneOutput{}, err
	}
	out := InterveneOutput{
		Primitive:      string(res.Primitive),
		Correlation:    res.CorrelationID,
		Accepted:       res.Accepted,
		Applied:        res.Applied,
		State:          res.State,
		Paused:         res.Paused,
		Pending:        res.Pending,
		IdempotencyKey: res.IdempotencyKey,
		Reason:         res.Reason,
	}
	if !res.LeaseExpires.IsZero() {
		unix := res.LeaseExpires.Unix()
		out.LeaseExpiresUnix = &unix
	}
	return out, nil
}

func bind[I, O any](ops *[]app.Operation, spec opapi.Spec, handler func(context.Context, I) (O, error)) error {
	output, err := schema.FromType(reflect.TypeFor[O](), false)
	if err != nil {
		return err
	}
	spec.OutputSchema = output
	spec.Authz, spec.Tenancy, spec.AllowUnknownInput = opapi.OwnTenant, opapi.CallerTenant, true
	op, err := app.NewOperation(spec, handler)
	if err != nil {
		return err
	}
	*ops = append(*ops, op)
	return nil
}

// Operations declares the five steering writes. Each routes to the caller's
// tenant kernel, so a tenant can steer its own runs without the primary token.
func Operations(provider func(context.Context) *Service) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("steer provider required")
	}
	runInput := json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"correlation":{}}}`)
	var ops []app.Operation
	for _, b := range []func() error{
		func() error {
			return bind(&ops, opapi.Spec{Name: "run_pause", InputSchema: runInput, HTTP: opapi.HTTP{Method: "POST", Path: "/api/run/pause"}}, func(ctx context.Context, in RunRequest) (ControlOutput, error) {
				return provider(ctx).Pause(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "run_resume", InputSchema: runInput, HTTP: opapi.HTTP{Method: "POST", Path: "/api/run/resume"}}, func(ctx context.Context, in RunRequest) (ControlOutput, error) {
				return provider(ctx).Resume(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "run_step", InputSchema: runInput, HTTP: opapi.HTTP{Method: "POST", Path: "/api/run/step"}}, func(ctx context.Context, in RunRequest) (ControlOutput, error) {
				return provider(ctx).Step(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "run_steer", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"correlation":{},"directive":{},"mode":{}}}`), HTTP: opapi.HTTP{Method: "POST", Path: "/api/run/steer"}}, func(ctx context.Context, in SteerRequest) (SteerOutput, error) {
				return provider(ctx).Steer(ctx, in)
			})
		},
		func() error {
			return bind(&ops, opapi.Spec{Name: "run_intervene", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"correlation":{},"primitive":{},"directive":{},"scope":{},"idempotency_key":{},"lease_ms":{}}}`)}, func(ctx context.Context, in InterveneRequest) (InterveneOutput, error) {
				return provider(ctx).Intervene(ctx, in)
			})
		},
	} {
		if err := b(); err != nil {
			return nil, err
		}
	}
	return ops, nil
}
