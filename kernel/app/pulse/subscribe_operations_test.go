// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"reflect"
	"testing"
)

func TestPulseSubscribeRequestPresenceDefaultsAndNativeValidation(t *testing.T) {
	cases := []struct {
		raw   string
		want  ReplayInput
		error string
	}{
		{`{}`, unboundedReplay(), ""},
		{`{"pattern":"  ","kinds":[]}`, unboundedReplay(), ""},
		{`{"pattern":" owned.* ","kinds":[" "," workflow.started ","workflow.started"],"since":1.9,"since_ts_ms":2.9,"until":3.9,"until_ts_ms":4.9,"correlation":" owned ","replay_rate":2}`, ReplayInput{Pattern: "owned.*", Kinds: map[event.Kind]struct{}{event.KindWorkflowStarted: {}}, Since: 1, SinceTSMS: 2, Until: 3, UntilTSMS: 4, Correlation: "owned", RateEPS: 2}, ""},
		{`{"replay_rate":-4}`, unboundedReplay(), ""},
		{`{"pattern":null}`, ReplayInput{}, "args.pattern must be a string"},
		{`{"kinds":null}`, ReplayInput{}, "args.kinds must be an array"},
		{`{"kinds":true}`, ReplayInput{}, "args.kinds must be an array"},
		{`{"kinds":["valid",true]}`, ReplayInput{}, "args.kinds[1] must be a string"},
		{`{"since":"1"}`, ReplayInput{}, "args.since must be a number"},
		{`{"since_ts_ms":null}`, ReplayInput{}, "args.since_ts_ms must be a number"},
		{`{"until":false}`, ReplayInput{}, "args.until must be a number"},
		{`{"until_ts_ms":"4"}`, ReplayInput{}, "args.until_ts_ms must be a number"},
		{`{"correlation":true}`, ReplayInput{}, "args.correlation must be a string"},
		{`{"replay_rate":"2"}`, ReplayInput{}, "args.replay_rate must be a number"},
		{`{"pattern":true,"kinds":true}`, ReplayInput{}, "args.pattern must be a string"},
		{`{"since":true,"until":true,"correlation":true}`, ReplayInput{}, "args.since must be a number"},
	}
	for _, tc := range cases {
		var in SubscribeRequest
		if err := json.Unmarshal([]byte(tc.raw), &in); err != nil {
			t.Fatal(err)
		}
		got, err := in.decode()
		if tc.error != "" {
			if err == nil || err.Error() != tc.error {
				t.Fatal(tc.raw, got, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc.raw, got, tc.want, err)
		}
	}
}

type subscribeAuth struct{ principal opapi.Principal }

func (a subscribeAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.principal, nil
}

type subscribeRoute struct{}

func (subscribeRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type subscribeEmit struct {
	events []*event.Event
	cause  error
}

func (e *subscribeEmit) Emit(_ context.Context, value any) error {
	e.events = append(e.events, value.(*event.Event))
	return e.cause
}
func TestPulseSubscribeTypedContractOwnsCanonicalOutputAndAdmission(t *testing.T) {
	if _, err := SubscribeOperations(nil); err == nil {
		t.Fatal("nil provider admitted")
	}
	prepared, watchers, canceled, providers, subscribed := 0, 0, 0, 0, 0
	ev := streamEvent(0, false)
	ev.Payload = json.RawMessage(`{"nested":[false,0,null,{"raw":"owned"}]}`)
	j := &streamJournal{events: []*event.Event{ev}}
	service := NewStream(j, func(pattern string, buffer int) (Subscription, error) {
		subscribed++
		if pattern != ">" || buffer != 4096 {
			t.Fatal(pattern, buffer)
		}
		return Subscription{Cancel: func() { canceled++ }}, nil
	}, nil)
	operations, err := SubscribeOperations(func(context.Context) SubscribeHost {
		providers++
		return SubscribeHost{Stream: service, Prepare: func() { prepared++ }, ClientGone: func() <-chan struct{} { watchers++; return nil }}
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := operations[0].Spec()
	if len(operations) != 1 || spec.Name != "pulse_subscribe" || !spec.ReadOnly || spec.Stream != opapi.StreamLive || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Input != reflect.TypeFor[SubscribeRequest]() || spec.Output != reflect.TypeFor[SubscribeOutput]() || spec.Emission != reflect.TypeFor[*event.Event]() || len(spec.EmissionSchema) == 0 || !spec.AllowUnknownInput {
		t.Fatal(spec)
	}
	d, err := app.NewDispatcher(operations, app.Dependencies{Auth: subscribeAuth{opapi.Principal{Kind: opapi.Operator}}, Router: subscribeRoute{}})
	if err != nil {
		t.Fatal(err)
	}
	emitter := &subscribeEmit{}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(`{"since":0,"until":1,"unused":true}`), emitter)
	encoded, _ := json.Marshal(out)
	if err != nil || string(encoded) != "{}" || !reflect.DeepEqual(emitter.events, []*event.Event{ev}) || providers != 1 || prepared != 1 || subscribed != 1 || canceled != 1 || watchers != 0 {
		t.Fatal(out, err, emitter, providers, prepared, subscribed, canceled, watchers)
	}
	for _, mode := range []string{"validation", "missing-emitter", "cancel", "tenant"} {
		ctx := context.Background()
		raw := json.RawMessage(`{"until":0}`)
		port := emitter
		dispatcher := d
		switch mode {
		case "validation":
			raw = json.RawMessage(`{"kinds":[false]}`)
		case "missing-emitter":
			port = nil
		case "cancel":
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		case "tenant":
			dispatcher, _ = app.NewDispatcher(operations, app.Dependencies{Auth: subscribeAuth{opapi.Principal{Kind: opapi.Tenant, Tenant: "acme"}}, Router: subscribeRoute{}})
		}
		var emit opapi.Emitter
		if port != nil {
			emit = port
		}
		if _, err := dispatcher.Dispatch(ctx, opapi.Caller{Tenant: "acme"}, spec.Name, raw, emit); err == nil {
			t.Fatal(mode, "unexpected admission")
		}
	}
	if providers != 1 || prepared != 1 || subscribed != 1 || canceled != 1 || watchers != 0 {
		t.Fatal(providers, prepared, subscribed, canceled, watchers)
	}
	cause := errors.New("emit failed")
	emitter.cause = cause
	_, err = d.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(`{"since":0,"until":1}`), emitter)
	var replayError ReplayError
	if !errors.As(err, &replayError) || !errors.Is(err, cause) || canceled != 2 {
		t.Fatal(err, canceled)
	}
	// A missing selected stream fails before transport preparation.
	missing, _ := SubscribeOperations(func(context.Context) SubscribeHost {
		return SubscribeHost{Prepare: func() { t.Fatal("prepared missing stream") }}
	})
	dispatcher, _ := app.NewDispatcher(missing, app.Dependencies{Auth: subscribeAuth{opapi.Principal{Kind: opapi.Operator}}, Router: subscribeRoute{}})
	if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, spec.Name, json.RawMessage(`{}`), emitter); err == nil || err.Error() != "pulse subscription unavailable" {
		t.Fatal(err)
	}
}
