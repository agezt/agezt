// SPDX-License-Identifier: MIT

package world_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	appworld "github.com/agezt/agezt/kernel/app/world"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type worldAuth struct{}

func (worldAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

func TestWorldTypedResolveRetainsLegacyLimits(t *testing.T) {
	_, service := worldFixture(t)
	for i := 0; i < 105; i++ {
		if _, err := service.Add(context.Background(), appworld.AddInput{Name: fmt.Sprintf("fixture-%03d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	operations, err := appworld.Operations(func(context.Context) *appworld.Service { return service }, func(context.Context) *appworld.LogService { return appworld.NewLog(logReader{}) })
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := app.NewDispatcher(operations, app.Dependencies{Auth: worldAuth{}, Router: worldRouter{}, Audit: &worldAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		limit float64
		count int
	}{{0, 10}, {-1, 10}, {1, 1}, {1.9, 1}, {.5, 105}, {2000, 100}} {
		raw, err := json.Marshal(map[string]any{"query": "fixture", "limit": tc.limit})
		if err != nil {
			t.Fatal(err)
		}
		value, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "world_resolve", raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		out := value.(appworld.ResolveOutput)
		if out.Count != tc.count || len(out.Results) != tc.count {
			t.Errorf("typed resolve limit=%v count=%d/%d want=%d", tc.limit, out.Count, len(out.Results), tc.count)
		}
	}
}

type worldRouter struct{}

func (worldRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type worldAudit struct {
	cause error
	calls int
}

func (a *worldAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	if a.cause != nil {
		return nil, a.cause
	}
	return worldSpan{}, nil
}

type worldSpan struct{}

func (worldSpan) End(context.Context, error) error { return nil }

func TestWorldSpecsRetainCompleteTypedMetadataAndSchemas(t *testing.T) {
	operations, err := appworld.Operations(func(context.Context) *appworld.Service { return nil }, func(context.Context) *appworld.LogService { return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"world_add": false, "world_edit": false, "world_relate": false, "world_forget": false, "world_get": true, "world_list": true, "world_resolve": true, "world_neighbors": true, "world_log": true}
	if len(operations) != 9 {
		t.Fatalf("world operation count=%d", len(operations))
	}
	seen := map[string]bool{}
	for _, operation := range operations {
		spec := operation.Spec()
		readOnly, exists := want[spec.Name]
		if !exists || seen[spec.Name] || spec.ReadOnly != readOnly || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input == nil || spec.Output == nil {
			t.Fatalf("world metadata=%+v", spec)
		}
		seen[spec.Name] = true
		if spec.Name == "world_log" {
			if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("world log scope=%+v", spec)
			}
		} else if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary graph scope=%+v", spec)
		}
		raw, err := json.Marshal(reflect.Zero(spec.Output).Interface())
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("%s output schema=%v", spec.Name, err)
		}
		var input json.RawMessage
		switch spec.Name {
		case "world_add":
			input = json.RawMessage(`{"name":"fixture","ignored":true}`)
		case "world_edit", "world_forget", "world_get":
			input = json.RawMessage(`{"id":"fixture","ignored":true}`)
		case "world_relate":
			input = json.RawMessage(`{"from":"a","to":"b","ignored":true}`)
		case "world_resolve", "world_neighbors":
			input = json.RawMessage(`{"query":"fixture","ignored":true}`)
		default:
			input = json.RawMessage(`{"ignored":true}`)
		}
		if err := schema.ValidateJSON(spec.InputSchema, input); err != nil {
			t.Fatalf("%s compatible input=%v", spec.Name, err)
		}
		if spec.Name == "world_add" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"name":"fixture","aliases":null}`)) == nil {
			t.Fatal("null aliases admitted")
		}
		if spec.Name == "world_edit" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"id":"fixture","attrs":null}`)) == nil {
			t.Fatal("null attrs admitted")
		}
	}
	if _, err := appworld.Operations(nil, nil); err == nil {
		t.Fatal("nil world providers admitted")
	}
}

func TestWorldMutationAuditPrecedesEveryServiceEffect(t *testing.T) {
	s, service := worldFixture(t)
	calls := 0
	operations, err := appworld.Operations(func(context.Context) *appworld.Service { calls++; return service }, func(context.Context) *appworld.LogService { calls++; return appworld.NewLog(logReader{}) })
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("fixture world audit unavailable")
	audit := &worldAudit{cause: cause}
	dispatcher, err := app.NewDispatcher(operations, app.Dependencies{Auth: worldAuth{}, Router: worldRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, raw string }{{"world_add", `{"name":"fixture"}`}, {"world_edit", `{"id":"missing"}`}, {"world_relate", `{"from":"a","to":"b"}`}, {"world_forget", `{"id":"missing"}`}} {
		if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); !errors.Is(err, cause) {
			t.Errorf("%s audit cause=%v", tc.name, err)
		}
	}
	if audit.calls != 4 || calls != 0 || s.Count() != 0 {
		t.Fatalf("effect before audit: audit=%d calls=%d entities=%d", audit.calls, calls, s.Count())
	}
}

func TestWorldTypedAdmissionAndReadOnlyCompatibility(t *testing.T) {
	s, service := worldFixture(t)
	calls := 0
	operations, err := appworld.Operations(func(context.Context) *appworld.Service { calls++; return service }, func(context.Context) *appworld.LogService { calls++; return appworld.NewLog(logReader{}) })
	if err != nil {
		t.Fatal(err)
	}
	audit := &worldAudit{}
	dispatcher, err := app.NewDispatcher(operations, app.Dependencies{Auth: worldAuth{}, Router: worldRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, raw string }{{"world_add", `{"name":true}`}, {"world_add", `{"name":"fixture","aliases":null}`}, {"world_add", `{"name":"fixture","aliases":[true]}`}, {"world_edit", `{"id":"fixture","attrs":null}`}, {"world_edit", `{"id":"fixture","attrs":{"bad":true}}`}, {"world_relate", `{"from":true,"to":"b"}`}, {"world_resolve", `{"query":"fixture","limit":"bad"}`}, {"world_log", `{"kind":true}`}} {
		if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); err == nil {
			t.Errorf("%s invalid input admitted", tc.name)
		}
	}
	if calls != 0 || audit.calls != 0 || s.Count() != 0 {
		t.Fatalf("invalid input reached effects: calls=%d audit=%d entities=%d", calls, audit.calls, s.Count())
	}
	for _, tc := range []struct{ name, raw string }{{"world_get", `{"id":"missing"}`}, {"world_list", `{}`}, {"world_resolve", `{"query":"fixture"}`}, {"world_neighbors", `{"query":"fixture"}`}, {"world_log", `{"limit":"ignored","since_ms":"ignored"}`}} {
		if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); err != nil {
			t.Errorf("%s read=%v", tc.name, err)
		}
	}
	if audit.calls != 0 {
		t.Fatalf("read-only graph audited=%d", audit.calls)
	}
	out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "world_add", json.RawMessage(`{"name":"fixture","aliases":[" ","  Display Alias  "]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	added := out.(appworld.AddOutput)
	got, err := service.Get(context.Background(), appworld.GetInput{ID: added.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entity.Aliases) != 1 || got.Entity.Aliases[0] != "Display Alias" {
		t.Fatalf("alias normalization=%v", got.Entity.Aliases)
	}
}
