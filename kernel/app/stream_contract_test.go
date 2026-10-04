// SPDX-License-Identifier: MIT

package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestOperationIndependentEmissionAndTerminalContracts(t *testing.T) {
	type progress struct {
		Step string `json:"step"`
	}
	type result struct {
		Done bool `json:"done"`
	}
	emission := json.RawMessage(`{"type":"object","properties":{"step":{"type":"string","enum":["loading"]}},"required":["step"],"additionalProperties":false}`)
	terminal := json.RawMessage(`{"type":"object","properties":{"done":{"type":"boolean","enum":[true]}},"required":["done"],"additionalProperties":false}`)
	cause := errors.New("transport unavailable")
	for _, mode := range []string{"valid", "derived", "bad-emission", "bad-terminal", "transport"} {
		t.Run(mode, func(t *testing.T) {
			declared := append(json.RawMessage(nil), emission...)
			declaration := opapi.Spec{Name: "progress", ReadOnly: true, Stream: opapi.StreamEvents, EmissionSchema: declared, OutputSchema: terminal}
			if mode == "derived" {
				declaration.EmissionSchema, declaration.OutputSchema = nil, nil
			}
			op, err := app.NewStreamingOperation(declaration, func(_ context.Context, _ struct{}, emit func(progress) error) (result, error) {
				step := "loading"
				if mode == "bad-emission" {
					step = "invalid"
				}
				if err := emit(progress{step}); err != nil {
					return result{}, err
				}
				return result{Done: mode != "bad-terminal"}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			declared[0] = '!'
			spec := op.Spec()
			if spec.Output != reflect.TypeFor[result]() || spec.Emission != reflect.TypeFor[progress]() {
				t.Fatalf("types conflated: output=%v emission=%v", spec.Output, spec.Emission)
			}
			spec.EmissionSchema[0] = '!'
			d, err := app.NewDispatcher([]app.Operation{op}, app.Dependencies{
				Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
					return opapi.Principal{Kind: opapi.Operator}, nil
				}),
				Router: routeFunc(func(ctx context.Context, _ opapi.Principal, spec opapi.Spec) (context.Context, error) {
					spec.EmissionSchema[0] = '!'
					return ctx, nil
				}),
			})
			if err != nil {
				t.Fatal(err)
			}
			emissions := 0
			value, err := d.Dispatch(context.Background(), opapi.Caller{}, "progress", nil, emitFunc(func(_ context.Context, value any) error {
				if value != (progress{Step: "loading"}) {
					t.Errorf("emission type/value=%v", value)
				}
				emissions++
				if mode == "transport" {
					return cause
				}
				return nil
			}))
			if mode == "valid" || mode == "derived" {
				if err != nil || value != (result{Done: true}) || emissions != 1 {
					t.Fatalf("separate contracts rejected: value=%v err=%v emissions=%d", value, err, emissions)
				}
			} else {
				if err == nil || value != nil {
					t.Fatalf("bad contract escaped: value=%v err=%v", value, err)
				}
				if mode == "bad-emission" && emissions != 0 {
					t.Fatalf("invalid frame published: %d", emissions)
				}
				if mode == "transport" && !errors.Is(err, cause) {
					t.Fatalf("transport cause lost: %v", err)
				}
			}
		})
	}
}

func TestOperationCustomEmissionSchemaWithDerivedTerminal(t *testing.T) {
	type result struct {
		Done bool `json:"done"`
	}
	spec := opapi.Spec{Name: "custom-stream", ReadOnly: true, Stream: opapi.StreamLive, EmissionSchema: json.RawMessage(`{"type":"string"}`)}
	op, err := app.NewStreamingOperation(spec, func(_ context.Context, _ struct{}, emit func(time.Time) error) (result, error) {
		return result{Done: true}, emit(time.Unix(0, 0).UTC())
	})
	if err != nil {
		t.Fatal(err)
	}
	if op.Spec().Emission != reflect.TypeFor[time.Time]() || op.Spec().Output != reflect.TypeFor[result]() {
		t.Fatal("custom emission overwrote terminal metadata")
	}
	d, err := app.NewDispatcher([]app.Operation{op}, app.Dependencies{
		Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
			return opapi.Principal{Kind: opapi.Operator}, nil
		}),
		Router: routeFunc(func(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) { return ctx, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	emissions := 0
	value, err := d.Dispatch(context.Background(), opapi.Caller{}, spec.Name, nil, emitFunc(func(_ context.Context, value any) error {
		if value != time.Unix(0, 0).UTC() {
			t.Errorf("custom emission changed: %v", value)
		}
		emissions++
		return nil
	}))
	if err != nil || value != (result{Done: true}) || emissions != 1 {
		t.Fatalf("custom frame/derived terminal rejected: %v %v %d", value, err, emissions)
	}
	for _, bad := range []json.RawMessage{json.RawMessage(`{broken`), json.RawMessage(`{"type":"unsupported"}`)} {
		spec.EmissionSchema = bad
		if _, err := app.NewStreamingOperation(spec, func(context.Context, struct{}, func(time.Time) error) (result, error) { return result{}, nil }); err == nil {
			t.Fatalf("bad emission schema registered: %s", bad)
		}
	}
	plain, err := app.NewOperation(opapi.Spec{Name: "plain", ReadOnly: true}, func(context.Context, struct{}) (result, error) { return result{}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if plain.Spec().Emission != nil || len(plain.Spec().EmissionSchema) != 0 {
		t.Fatal("unary operation advertises stream frames")
	}
}

func TestOperationTerminalOnlyModeRejectsEmission(t *testing.T) {
	op, err := app.NewStreamingOperation(opapi.Spec{Name: "unary", ReadOnly: true}, func(_ context.Context, _ struct{}, emit func(string) error) (bool, error) { return true, emit("frame") })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher([]app.Operation{op}, app.Dependencies{
		Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
			return opapi.Principal{Kind: opapi.Operator}, nil
		}),
		Router: routeFunc(func(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) { return ctx, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	emissions := 0
	value, err := d.Dispatch(context.Background(), opapi.Caller{}, "unary", nil, emitFunc(func(context.Context, any) error { emissions++; return nil }))
	if err == nil || value != nil || emissions != 0 {
		t.Fatalf("terminal-only operation emitted: value=%v err=%v emissions=%d", value, err, emissions)
	}
}
