// SPDX-License-Identifier: MIT

package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

func TestDispatchPreservesNullableInputAndRejectsTypedMapMismatch(t *testing.T) {
	type child struct {
		Name string `json:"name"`
	}
	type payload struct {
		Pointer *child            `json:"pointer"`
		Values  map[string]*child `json:"values"`
	}
	calls := 0
	op, err := app.NewOperation(opapi.Spec{Name: "nullable", ReadOnly: true}, func(_ context.Context, in *payload) (output, error) {
		calls++
		if in == nil {
			return output{"root-null"}, nil
		}
		if in.Pointer == nil && in.Values == nil {
			return output{"fields-null"}, nil
		}
		return output{in.Values["key"].Name}, nil
	})
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
	for _, tc := range []struct{ raw, want string }{
		{`null`, "root-null"},
		{`{"pointer":null,"values":null}`, "fields-null"},
		{`{"pointer":null,"values":{"key":{"name":"typed"}}}`, "typed"},
	} {
		value, err := d.Dispatch(context.Background(), opapi.Caller{}, "nullable", json.RawMessage(tc.raw), nil)
		if err != nil || value.(output).Value != tc.want {
			t.Fatalf("wire=%s result=%v error=%v", tc.raw, value, err)
		}
	}
	for _, raw := range []string{`{"pointer":null}`, `{"pointer":null,"values":{"key":{"name":12}}}`, `{"pointer":null,"values":{"key":{"name":"typed","extra":true}}}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "nullable", json.RawMessage(raw), nil); err == nil {
			t.Errorf("bad input accepted: %s", raw)
		}
	}
	if calls != 3 {
		t.Fatalf("invalid input reached handler: %d", calls)
	}
}
