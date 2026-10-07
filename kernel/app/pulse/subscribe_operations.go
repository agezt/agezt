// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"strings"
)

// SubscribeRequest retains presence-sensitive native argument validation.
type SubscribeRequest struct {
	Pattern     json.RawMessage `json:"pattern,omitempty"`
	Kinds       json.RawMessage `json:"kinds,omitempty"`
	Since       json.RawMessage `json:"since,omitempty"`
	SinceTSMS   json.RawMessage `json:"since_ts_ms,omitempty"`
	Until       json.RawMessage `json:"until,omitempty"`
	UntilTSMS   json.RawMessage `json:"until_ts_ms,omitempty"`
	Correlation json.RawMessage `json:"correlation,omitempty"`
	ReplayRate  json.RawMessage `json:"replay_rate,omitempty"`
}

// SubscribeHost supplies selected stream and optional transport lifetime hooks.
// Prepare runs after argument validation; ClientGone remains lazy after replay.
type SubscribeHost struct {
	Stream     *Stream
	Prepare    func()
	ClientGone func() <-chan struct{}
}
type SubscribeOutput struct{}

var subscribeSchema = json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"pattern":{},"kinds":{},"since":{},"since_ts_ms":{},"until":{},"until_ts_ms":{},"correlation":{},"replay_rate":{}}}`)
var eventSchema = json.RawMessage(event.WireSchema)

func subscribeNumber(raw json.RawMessage, key string, missing float64) (float64, error) {
	if len(raw) == 0 {
		return missing, nil
	}
	value, ok := controlValue(raw).(float64)
	if !ok {
		return 0, fmt.Errorf("args.%s must be a number", key)
	}
	return value, nil
}
func (in SubscribeRequest) decode() (ReplayInput, error) {
	out := ReplayInput{Pattern: ">", Since: -1, SinceTSMS: -1, Until: -1, UntilTSMS: -1}
	pattern, err := controlText(in.Pattern, "pattern", false)
	if err != nil {
		return out, err
	}
	if value := strings.TrimSpace(pattern); value != "" {
		out.Pattern = value
	}
	if len(in.Kinds) > 0 {
		list, ok := controlValue(in.Kinds).([]any)
		if !ok {
			return out, errors.New("args.kinds must be an array")
		}
		for index, item := range list {
			value, ok := item.(string)
			if !ok {
				return out, fmt.Errorf("args.kinds[%d] must be a string", index)
			}
			if value = strings.TrimSpace(value); value != "" {
				if out.Kinds == nil {
					out.Kinds = map[event.Kind]struct{}{}
				}
				out.Kinds[event.Kind(value)] = struct{}{}
			}
		}
	}
	for _, field := range []struct {
		raw    json.RawMessage
		key    string
		target *int64
	}{{in.Since, "since", &out.Since}, {in.SinceTSMS, "since_ts_ms", &out.SinceTSMS}, {in.Until, "until", &out.Until}, {in.UntilTSMS, "until_ts_ms", &out.UntilTSMS}} {
		value, err := subscribeNumber(field.raw, field.key, -1)
		if err != nil {
			return out, err
		}
		*field.target = int64(value)
	}
	correlation, err := controlText(in.Correlation, "correlation", false)
	if err != nil {
		return out, err
	}
	out.Correlation = strings.TrimSpace(correlation)
	rate, err := subscribeNumber(in.ReplayRate, "replay_rate", 0)
	if err != nil {
		return out, err
	}
	if rate > 0 {
		out.RateEPS = rate
	}
	return out, nil
}

// SubscribeOperations exposes typed StreamLive metadata and a canonical object
// terminal. Native transport projects the historical event-only wire separately.
func SubscribeOperations(provider func(context.Context) SubscribeHost) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("pulse subscription provider required")
	}
	operation, err := app.NewStreamingOperation(opapi.Spec{Name: "pulse_subscribe", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, Stream: opapi.StreamLive, AllowUnknownInput: true, InputSchema: subscribeSchema, EmissionSchema: eventSchema}, func(ctx context.Context, in SubscribeRequest, emit func(*event.Event) error) (SubscribeOutput, error) {
		decoded, err := in.decode()
		if err != nil {
			return SubscribeOutput{}, err
		}
		host := provider(ctx)
		if host.Stream == nil {
			return SubscribeOutput{}, errors.New("pulse subscription unavailable")
		}
		if host.Prepare != nil {
			host.Prepare()
		}
		return SubscribeOutput{}, host.Stream.Stream(ctx, decoded, emit, host.ClientGone)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{operation}, nil
}
