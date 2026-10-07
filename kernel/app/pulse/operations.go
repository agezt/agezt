// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// ControlRequest preserves native presence and dual-type argument conventions.
// Decode values only after checking the selected runtime's availability.
type ControlRequest struct {
	IssueKey json.RawMessage `json:"issue_key,omitempty"`
	Approve  json.RawMessage `json:"approve,omitempty"`
	Seconds  json.RawMessage `json:"seconds,omitempty"`
	Dial     json.RawMessage `json:"dial,omitempty"`
	Hours    json.RawMessage `json:"hours,omitempty"`
	Path     json.RawMessage `json:"path,omitempty"`
	MinPct   json.RawMessage `json:"min_pct,omitempty"`
	Name     json.RawMessage `json:"name,omitempty"`
	Command  json.RawMessage `json:"command,omitempty"`
}

var controlSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"issue_key":{},"approve":{},"seconds":{},"dial":{},"hours":{},"path":{},"min_pct":{},"name":{},"command":{}}}`)

func controlValue(raw json.RawMessage) any {
	var value any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}
func controlText(raw json.RawMessage, key string, required bool) (string, error) {
	value := ""
	if len(raw) > 0 {
		var ok bool
		value, ok = controlValue(raw).(string)
		if !ok {
			return "", fmt.Errorf("args.%s must be a string", key)
		}
	}
	if required && strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("args.%s required", key)
	}
	return value, nil
}
func controlNumber(raw json.RawMessage) float64 {
	switch value := controlValue(raw).(type) {
	case float64:
		return value
	case string:
		out, _ := strconv.ParseFloat(value, 64)
		return out
	}
	return 0
}
func controlApprove(raw json.RawMessage) bool {
	switch value := controlValue(raw).(type) {
	case bool:
		return value
	case string:
		return value == "true" || value == "1"
	}
	return false
}
func bindControl[O any](ops *[]app.Operation, name string, read bool, handler func(context.Context, ControlRequest) (O, error)) error {
	operation, err := app.NewOperation(opapi.Spec{Name: name, ReadOnly: read, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: controlSchema}, handler)
	if err == nil {
		*ops = append(*ops, operation)
	}
	return err
}

// ControlOperations binds the thirteen resident controls. Live subscription is a
// separate streaming contract, whose native event-only projection is preserved.
func ControlOperations(service func(context.Context) *Controls) ([]app.Operation, error) {
	if service == nil {
		return nil, errors.New("pulse controls provider required")
	}
	var ops []app.Operation
	bindings := []func() error{
		func() error {
			return bindControl(&ops, "pulse_status", true, func(ctx context.Context, in ControlRequest) (StatusOutput, error) {
				return service(ctx).Status(ctx, StatusInput{})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_asks", true, func(ctx context.Context, in ControlRequest) (AsksOutput, error) {
				return service(ctx).Asks(ctx, AsksInput{})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_pause", false, func(ctx context.Context, in ControlRequest) (PauseOutput, error) {
				return service(ctx).Pause(ctx, PauseInput{})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_resume", false, func(ctx context.Context, in ControlRequest) (PauseOutput, error) {
				return service(ctx).Resume(ctx, PauseInput{})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_beat", false, func(ctx context.Context, in ControlRequest) (BeatOutput, error) {
				return service(ctx).Beat(ctx, BeatInput{})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_ask_resolve", false, func(ctx context.Context, in ControlRequest) (ResolveOutput, error) {
				s := service(ctx)
				if !s.Available() {
					return ResolveOutput{}, ErrDisabled
				}
				key, err := controlText(in.IssueKey, "issue_key", true)
				if err != nil {
					return ResolveOutput{}, err
				}
				return s.Resolve(ctx, ResolveInput{IssueKey: key, Approve: controlApprove(in.Approve)})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_cadence", false, func(ctx context.Context, in ControlRequest) (CadenceOutput, error) {
				s := service(ctx)
				if !s.Available() {
					return CadenceOutput{}, ErrDisabled
				}
				return s.Cadence(ctx, CadenceInput{Seconds: controlNumber(in.Seconds)})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_dial", false, func(ctx context.Context, in ControlRequest) (DialOutput, error) {
				s := service(ctx)
				if !s.Available() {
					return DialOutput{}, ErrDisabled
				}
				value, err := controlText(in.Dial, "dial", false)
				if err != nil {
					return DialOutput{}, err
				}
				return s.Dial(ctx, DialInput{Dial: value})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_quiet", false, func(ctx context.Context, in ControlRequest) (QuietOutput, error) {
				s := service(ctx)
				if !s.Available() {
					return QuietOutput{}, ErrDisabled
				}
				value, err := controlText(in.Hours, "hours", false)
				if err != nil {
					return QuietOutput{}, err
				}
				return s.Quiet(ctx, QuietInput{Hours: value})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_flush", false, func(ctx context.Context, in ControlRequest) (FlushOutput, error) {
				return service(ctx).Flush(ctx, FlushInput{})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_unwatch", false, func(ctx context.Context, in ControlRequest) (UnwatchOutput, error) {
				s := service(ctx)
				if !s.Available() {
					return UnwatchOutput{}, ErrDisabled
				}
				name, err := controlText(in.Name, "name", true)
				if err != nil {
					return UnwatchOutput{}, err
				}
				return s.Unwatch(ctx, UnwatchInput{Name: name})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_watch", false, func(ctx context.Context, in ControlRequest) (ObserverOutput, error) {
				s := service(ctx)
				if !s.WatchesAvailable() {
					return ObserverOutput{}, ErrWatchesUnavailable
				}
				path, err := controlText(in.Path, "path", true)
				if err != nil {
					return ObserverOutput{}, err
				}
				return s.Watch(ctx, WatchInput{Path: path, MinPct: controlNumber(in.MinPct)})
			})
		},
		func() error {
			return bindControl(&ops, "pulse_probe", false, func(ctx context.Context, in ControlRequest) (ObserverOutput, error) {
				s := service(ctx)
				if !s.WatchesAvailable() {
					return ObserverOutput{}, ErrWatchesUnavailable
				}
				name, err := controlText(in.Name, "name", false)
				if err != nil {
					return ObserverOutput{}, err
				}
				command, err := controlText(in.Command, "command", false)
				if err != nil {
					return ObserverOutput{}, err
				}
				return s.Probe(ctx, ProbeInput{Name: name, Command: command})
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
