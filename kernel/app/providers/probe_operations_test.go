// SPDX-License-Identifier: MIT

package providers_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestProbeSpecPreservesPrimaryOnlyAndTypedOptionalOutputs(t *testing.T) {
	ops, err := providers.ProbeOperations(func(context.Context) *providers.Probe { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("operations = %d", len(ops))
	}
	spec := ops[0].Spec()
	if spec.Name != "provider_probe" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Stream != opapi.StreamNone || spec.HTTP.Method != "POST" || spec.HTTP.Path != "/api/provider/probe" || spec.Output != reflect.TypeFor[providers.ProbeOutput]() {
		t.Fatalf("probe metadata = %+v", spec)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"ok":false,"error":"cannot reach endpoint: fixture"}`),
		json.RawMessage(`{"ok":true,"reachable":false,"authorized":false,"http_status":503,"models":0}`),
	} {
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("legacy output rejected: %s %v", raw, err)
		}
		var typed providers.ProbeOutput
		if err := json.Unmarshal(raw, &typed); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(typed)
		var before, after any
		_ = json.Unmarshal(raw, &before)
		_ = json.Unmarshal(encoded, &after)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("field presence changed: %s => %s", raw, encoded)
		}
	}
	if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(`{"ok":true,"models":"wrong"}`)) == nil {
		t.Fatal("typed model count erased")
	}
	if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"ignored":true}`)); err != nil {
		t.Fatal(err)
	}
}

type probeAuth struct{ principal opapi.Principal }

func (a probeAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.principal, nil
}

func TestProbeDispatchAdmitsPrimaryAndRejectsBeforeTransport(t *testing.T) {
	calls := 0
	ops, err := providers.ProbeOperations(func(context.Context) *providers.Probe {
		return providers.NewProbe(func(endpoint, header, key string, max int64) ([]byte, int, string, error) {
			calls++
			if endpoint != "https://fixture.invalid/models" || header != "Authorization" || key != "Bearer fixture" || max != 1<<20 {
				t.Fatalf("transport identity lost: %q %q %q %d", endpoint, header, key, max)
			}
			return []byte(`{"data":[]}`), 200, "application/json", nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Operator}, {Kind: opapi.Tenant, Tenant: "acme"}} {
		d, err := app.NewDispatcher(ops, app.Dependencies{Auth: probeAuth{principal}, Router: observationRouter{}})
		if err != nil {
			t.Fatal(err)
		}
		before := calls
		out, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "provider_probe", json.RawMessage(`{"url":"https://fixture.invalid","key":"fixture","ignored":true}`), nil)
		if principal.Kind == opapi.Tenant {
			if err == nil || calls != before {
				t.Fatalf("tenant entered transport: %v, calls=%d/%d", err, calls, before)
			}
			continue
		}
		if err != nil || !out.(providers.ProbeOutput).OK || calls != before+1 {
			t.Fatalf("primary read required audit or lost transport: %v %v", out, err)
		}
		for _, raw := range []json.RawMessage{json.RawMessage(`{"url":42}`), json.RawMessage(`{"url":"https://fixture.invalid","key":42}`), json.RawMessage(`{"url":null}`), json.RawMessage(`{}`)} {
			before := calls
			if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "provider_probe", raw, nil); err == nil || calls != before {
				t.Fatalf("invalid input entered transport: %s err=%v calls=%d/%d", raw, err, calls, before)
			}
		}
	}
}
