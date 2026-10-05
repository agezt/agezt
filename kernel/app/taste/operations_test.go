// SPDX-License-Identifier: MIT
package taste_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	apptaste "github.com/agezt/agezt/kernel/app/taste"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	curated "github.com/agezt/agezt/kernel/taste"
	"reflect"
	"testing"
)

type tasteAuth struct{}

func (tasteAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type tasteRouter struct{}

func (tasteRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type tasteAudit struct {
	cause error
	calls int
}

func (a *tasteAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return tasteSpan{}, a.cause
}

type tasteSpan struct{}

func (tasteSpan) End(context.Context, error) error { return nil }
func TestTasteSpecsRetainNativePolicyAndActualSchemas(t *testing.T) {
	ops, err := apptaste.Operations(func(context.Context) *apptaste.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"taste_list": true, "taste_create": false, "taste_delete": false}
	if len(ops) != 3 {
		t.Fatalf("operations=%d", len(ops))
	}
	seen := map[string]bool{}
	for _, op := range ops {
		spec := op.Spec()
		read, found := want[spec.Name]
		if !found || seen[spec.Name] || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.HTTP.Method != "" || spec.HTTP.Path != "" {
			t.Fatalf("metadata=%+v", spec)
		}
		seen[spec.Name] = true
		raw, err := json.Marshal(reflect.Zero(spec.Output).Interface())
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"title":true,"body":null,"id":12,"tags":["a",false],"scope":[],"limit":"ignored","unused":true}`)); err != nil {
			t.Fatalf("lenient admission changed: %v", err)
		}
	}
	if _, err := apptaste.Operations(nil); err == nil {
		t.Fatal("nil provider admitted")
	}
}
func TestTasteMandatoryAuditPrecedesEveryMutation(t *testing.T) {
	store, service, _ := tasteFixture(t)
	seed, err := service.Create(context.Background(), apptaste.CreateInput{Title: "seed", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ops, err := apptaste.Operations(func(context.Context) *apptaste.Service { calls++; return service })
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("owned audit unavailable")
	audit := &tasteAudit{cause: cause}
	dispatcher, err := app.NewDispatcher(ops, app.Dependencies{Auth: tasteAuth{}, Router: tasteRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, raw string }{{"taste_create", `{"title":"new","body":"body"}`}, {"taste_delete", `{"id":"` + seed.Exemplar.ID + `"}`}} {
		if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); !errors.Is(err, cause) {
			t.Fatalf("%s cause=%v", tc.name, err)
		}
	}
	all := store.List(curated.Filter{})
	if audit.calls != 2 || calls != 0 || len(all) != 1 || all[0].ID != seed.Exemplar.ID {
		t.Fatalf("audit=%d service=%d store=%v", audit.calls, calls, all)
	}
	out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "taste_list", json.RawMessage(`{}`), nil)
	if err != nil || out.(apptaste.ListOutput).Count != 1 || audit.calls != 2 {
		t.Fatalf("read audit=%d out=%v err=%v", audit.calls, out, err)
	}
}
func TestTasteTypedNativeLenientAdmission(t *testing.T) {
	_, service, _ := tasteFixture(t)
	ops, err := apptaste.Operations(func(context.Context) *apptaste.Service { return service })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: tasteAuth{}, Router: tasteRouter{}, Audit: &tasteAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tags := range []string{`[" Writing ",true,"",null,"writing"]`, `"Writing,writing"`} {
		raw := json.RawMessage(`{"title":" fixture ","body":" body ","scope":" Agent ","tags":` + tags + `,"unused":true}`)
		value, err := d.Dispatch(context.Background(), opapi.Caller{}, "taste_create", raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		e := value.(apptaste.CreateOutput).Exemplar
		if e.Title != "fixture" || e.Body != "body" || e.Scope != "Agent" || !reflect.DeepEqual(e.Tags, []string{"Writing", "writing"}) {
			t.Fatalf("created=%+v", e)
		}
	}
	for _, limit := range []string{`null`, `"ignored"`, `0.5`, `-1`, `0`, `true`, `1`} {
		value, err := d.Dispatch(context.Background(), opapi.Caller{}, "taste_list", json.RawMessage(`{"limit":`+limit+`,"scope":true,"tag":{},"unused":true}`), nil)
		if err != nil {
			t.Fatal(err)
		}
		want := 2
		if limit == "1" {
			want = 1
		}
		if value.(apptaste.ListOutput).Count != want {
			t.Fatalf("limit=%s out=%+v", limit, value)
		}
	}
	for _, tc := range []struct{ name, raw string }{{"taste_create", `{"title":true,"body":"body"}`}, {"taste_create", `{}`}, {"taste_delete", `{"id":12}`}} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); err == nil {
			t.Fatalf("invalid %s succeeded", tc.name)
		}
	}
}

func TestTasteTypedDeleteTrimsIdentity(t *testing.T) {
	store, service, _ := tasteFixture(t)
	created, err := service.Create(context.Background(), apptaste.CreateInput{Title: "seed", Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	ops, err := apptaste.Operations(func(context.Context) *apptaste.Service { return service })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: tasteAuth{}, Router: tasteRouter{}, Audit: &tasteAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"id": " " + created.Exemplar.ID + " "})
	if err != nil {
		t.Fatal(err)
	}
	value, err := d.Dispatch(context.Background(), opapi.Caller{}, "taste_delete", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if value.(apptaste.DeleteOutput).Deleted != created.Exemplar.ID || len(store.List(curated.Filter{})) != 0 {
		t.Fatalf("delete=%+v", value)
	}
}
