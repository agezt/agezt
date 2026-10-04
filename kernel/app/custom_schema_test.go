// SPDX-License-Identifier: MIT

package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestOperationAcceptsExplicitCustomSchemasAndOwnsBytes(t *testing.T) {
	schema := json.RawMessage(`{"type":"string","enum":["2026-10-04T00:00:00Z"]}`)
	calls := 0
	op, err := app.NewOperation(opapi.Spec{Name: "custom", ReadOnly: true, InputSchema: schema, OutputSchema: schema}, func(_ context.Context, in time.Time) (time.Time, error) { calls++; return in, nil })
	if err != nil {
		t.Fatalf("explicit custom schema rejected: %v", err)
	}
	schema[0] = '!'
	d, err := app.NewDispatcher([]app.Operation{op}, app.Dependencies{
		Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
			return opapi.Principal{Kind: opapi.Operator}, nil
		}),
		Router: routeFunc(func(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) { return ctx, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := d.Dispatch(context.Background(), opapi.Caller{}, "custom", json.RawMessage(`"2026-10-04T00:00:00Z"`), nil)
	if err != nil || value.(time.Time).UTC().Format(time.RFC3339) != "2026-10-04T00:00:00Z" {
		t.Fatalf("custom wire changed: %v %v", value, err)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "custom", json.RawMessage(`"2026-10-05T00:00:00Z"`), nil); err == nil || calls != 1 {
		t.Fatalf("custom input restriction lost: calls=%d error=%v", calls, err)
	}
	for _, bad := range []json.RawMessage{json.RawMessage(`{broken`), json.RawMessage(`{"type":"unsupported"}`)} {
		if _, err := app.NewOperation(opapi.Spec{Name: "bad", InputSchema: bad}, func(context.Context, time.Time) (output, error) { return output{}, nil }); err == nil {
			t.Errorf("invalid explicit schema registered: %s", bad)
		}
	}
}

func TestOperationValidatesCustomTerminalAndStreamOutput(t *testing.T) {
	wire := json.RawMessage(`{"type":"string","enum":["2026-10-04T00:00:00Z"]}`)
	wrong := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for _, stream := range []bool{false, true} {
		var op app.Operation
		var err error
		spec := opapi.Spec{Name: "output", ReadOnly: true, OutputSchema: wire}
		if stream {
			spec.Stream = opapi.StreamEvents
			op, err = app.NewStreamingOperation(spec, func(_ context.Context, _ struct{}, emit func(time.Time) error) (time.Time, error) {
				return wrong, emit(wrong)
			})
		} else {
			op, err = app.NewOperation(spec, func(context.Context, struct{}) (time.Time, error) { return wrong, nil })
		}
		if err != nil {
			t.Fatal(err)
		}
		d, err := app.NewDispatcher([]app.Operation{op}, app.Dependencies{Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
			return opapi.Principal{Kind: opapi.Operator}, nil
		}), Router: routeFunc(func(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) { return ctx, nil })})
		if err != nil {
			t.Fatal(err)
		}
		emissions := 0
		value, err := d.Dispatch(context.Background(), opapi.Caller{}, "output", nil, emitFunc(func(context.Context, any) error { emissions++; return nil }))
		if err == nil || value != nil || emissions != 0 {
			t.Fatalf("stream=%v malformed output escaped: %v %v emissions=%d", stream, value, err, emissions)
		}
	}
}
