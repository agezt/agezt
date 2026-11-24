// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/modules/extensions/toolforge"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"strings"
	"testing"
)

type forgeAuditProbe struct {
	begins, ends            int
	beginErr, endErr, cause error
	cancel                  context.CancelFunc
	records                 []opapi.AuditRecord
	factoryContexts         []context.Context
}

func (a *forgeAuditProbe) Begin(_ context.Context, r opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begins++
	a.records = append(a.records, r)
	if a.beginErr != nil {
		return nil, a.beginErr
	}
	if a.cancel != nil {
		a.cancel()
	}
	return a, nil
}
func (a *forgeAuditProbe) End(_ context.Context, cause error) error {
	a.ends++
	a.cause = cause
	return a.endErr
}
func forgeLifecycleDispatcher(t *testing.T, p *forgeWriterProbe, a *forgeAuditProbe, factory *int, kind opapi.PrincipalKind) (*app.Dispatcher, []app.Operation) {
	t.Helper()
	ops, err := ForgeLifecycleOperations(func(ctx context.Context) *ForgeLifecycle {
		*factory++
		a.factoryContexts = append(a.factoryContexts, ctx)
		return NewForgeLifecycle(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{kind}, Router: inventoryRoute{}, Audit: a})
	if err != nil {
		t.Fatal(err)
	}
	return d, ops
}
func forgeLifecycleRaw(name string) json.RawMessage {
	switch name {
	case "toolforge_draft":
		return json.RawMessage(`{"tool":{"name":"provided","code":" raw code "},"unknown":true,"correlation_id":"poison"}`)
	case "toolforge_edit":
		return json.RawMessage(`{"ref":" raw-ref ","tool":{"description":"new"},"unknown":true}`)
	case "toolforge_test":
		return json.RawMessage(`{"ref":" raw-ref ","input":" raw-input ","unknown":true}`)
	case "toolforge_quarantine":
		return json.RawMessage(`{"ref":" raw-ref ","reason":" raw-reason ","unknown":true}`)
	default:
		return json.RawMessage(`{"ref":" raw-ref ","unknown":true}`)
	}
}
func TestForgeLifecycleTypedMetadataAndOneMandatoryAuditPair(t *testing.T) {
	if _, err := ForgeLifecycleOperations(nil); err == nil {
		t.Fatal("missing provider")
	}
	p := &forgeWriterProbe{st: toolforge.ScriptTool{ID: "owned", TestedOK: true}, found: true, removed: true}
	a := &forgeAuditProbe{}
	factory := 0
	d, ops := forgeLifecycleDispatcher(t, p, a, &factory, opapi.Operator)
	inputs := []reflect.Type{reflect.TypeFor[ForgeDraftRequest](), reflect.TypeFor[ForgeEditRequest](), reflect.TypeFor[ForgeTestRequest](), reflect.TypeFor[ForgeRefRequest](), reflect.TypeFor[ForgeQuarantineRequest](), reflect.TypeFor[ForgeRefRequest]()}
	names := []string{opapi.ToolforgeDraft, "toolforge_edit", "toolforge_test", "toolforge_promote", "toolforge_quarantine", "toolforge_remove"}
	if len(ops) != 6 {
		t.Fatal(ops)
	}
	ctx := opapi.WithCorrelation(context.Background(), "owned-operation")
	for i, operation := range ops {
		spec := operation.Spec()
		output := reflect.TypeFor[ForgeMutationOutput]()
		if i == 2 {
			output = reflect.TypeFor[ForgeTestOutput]()
		} else if i == 5 {
			output = reflect.TypeFor[ForgeRemoveOutput]()
		}
		if spec.Name != names[i] || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != inputs[i] || spec.Output != output {
			t.Fatal(spec)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, spec.Name, forgeLifecycleRaw(spec.Name), nil)
		encoded, _ := json.Marshal(out)
		if err != nil || schema.ValidateJSON(spec.OutputSchema, encoded) != nil || factory != i+1 || a.begins != i+1 || a.ends != i+1 || a.cause != nil || a.records[i].Operation != spec.Name || p.corr != "owned-operation" || (i > 0 && p.ref != " raw-ref ") || a.factoryContexts[i] != ctx {
			t.Fatal(i, out, err, factory, a, p)
		}
	}
	if p.received.Name != "provided" || p.received.Code != " raw code " {
		t.Fatal(p)
	}
	if p.sample != " raw-input " || p.reason != " raw-reason " || p.ref != " raw-ref " {
		t.Fatal(p)
	}
}
func TestForgeLifecycleMandatoryAuditTenantAndCancellationBlockFactory(t *testing.T) {
	failure := errors.New("owned audit admission failure")
	for _, mode := range []string{"begin-failure", "tenant", "canceled", "cancel-after-begin"} {
		for i := 0; i < 6; i++ {
			p := &forgeWriterProbe{found: true}
			a := &forgeAuditProbe{}
			factory := 0
			kind := opapi.Operator
			ctx, cancel := context.WithCancel(context.Background())
			if mode == "begin-failure" {
				a.beginErr = failure
			}
			if mode == "tenant" {
				kind = opapi.Tenant
			}
			if mode == "canceled" {
				cancel()
			}
			if mode == "cancel-after-begin" {
				a.cancel = cancel
			}
			d, ops := forgeLifecycleDispatcher(t, p, a, &factory, kind)
			_, err := d.Dispatch(ctx, opapi.Caller{}, ops[i].Spec().Name, forgeLifecycleRaw(ops[i].Spec().Name), nil)
			cancel()
			if err == nil || factory != 0 || len(p.calls) != 0 {
				t.Fatal(mode, i, err, factory, p)
			}
			wantBegin, wantEnd := 0, 0
			if mode == "begin-failure" {
				wantBegin = 1
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
			}
			if mode == "cancel-after-begin" {
				wantBegin, wantEnd = 1, 1
				if !errors.Is(err, context.Canceled) || !errors.Is(a.cause, context.Canceled) {
					t.Fatal(err, a)
				}
			}
			if a.begins != wantBegin || a.ends != wantEnd {
				t.Fatal(mode, i, a)
			}
		}
	}
}
func TestForgeLifecycleNativeCodecErrorsBeforeFactoryAndOrder(t *testing.T) {
	p := &forgeWriterProbe{found: true}
	a := &forgeAuditProbe{}
	factory := 0
	d, _ := forgeLifecycleDispatcher(t, p, a, &factory, opapi.Operator)
	cases := []struct{ name, raw, want string }{
		{"toolforge_draft", `{}`, "args.tool required"},
		{"toolforge_edit", `{"ref":"owned"}`, "args.tool required"},
		{"toolforge_promote", `{"ref":" \t "}`, "args.ref required"},
		{"toolforge_remove", `{}`, "args.ref required"},
	}
	// A mistyped field is refused at the door, before the handler reads it and
	// before the audit sees it.
	door := []struct{ name, raw, want string }{
		{"toolforge_draft", `{"tool":false}`, "$.tool has wrong type: got boolean, want object"},
		{"toolforge_edit", `{"ref":false}`, "$.ref has wrong type: got boolean, want string"},
		{"toolforge_edit", `{"ref":"owned","tool":[]}`, "$.tool has wrong type: got array, want object"},
		{"toolforge_test", `{"ref":null}`, "$.ref has wrong type: got null, want string"},
		{"toolforge_test", `{"ref":"owned","input":null}`, "$.input has wrong type: got null, want string"},
		{"toolforge_test", `{"ref":"owned","input":false}`, "$.input has wrong type: got boolean, want string"},
		{"toolforge_quarantine", `{"ref":1}`, "$.ref has wrong type: got number, want string"},
		{"toolforge_quarantine", `{"ref":"owned","reason":null}`, "$.reason has wrong type: got null, want string"},
	}
	for _, tc := range cases {
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if err == nil || err.Error() != tc.want {
			t.Fatal(tc, err)
		}
	}
	audited := a.begins
	for _, tc := range door {
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatal(tc, err)
		}
	}
	if a.begins != audited {
		t.Fatal("a refused body was audited", a.begins, audited)
	}
	if factory != 0 || len(p.calls) != 0 || a.begins != len(cases) || a.ends != len(cases) {
		t.Fatal(factory, p, a)
	}
	// A null tool is not a tool: the door refuses it before the factory is
	// asked for one (the forge's own decoder would have read it as an empty
	// tool and drafted one).
	for _, name := range []string{opapi.ToolforgeDraft, "toolforge_edit"} {
		raw := `{"tool":null,"ref":" raw-ref "}`
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(raw), nil); err == nil {
			t.Fatal(name, "a null tool was admitted")
		} else if !strings.Contains(err.Error(), "$.tool has wrong type: got null, want object") {
			t.Fatal(name, err)
		}
	}
	if factory != 0 || p.received != (toolforge.ScriptTool{}) || p.edited != (toolforge.ScriptTool{}) {
		t.Fatal(factory, p)
	}
}
func TestForgeLifecycleOutputSchemasRequiredRootsAndAuditSettlement(t *testing.T) {
	p := &forgeWriterProbe{found: true}
	a := &forgeAuditProbe{}
	factory := 0
	d, ops := forgeLifecycleDispatcher(t, p, a, &factory, opapi.Operator)
	for _, op := range ops {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, op.Spec().Name, forgeLifecycleRaw(op.Spec().Name), nil)
		if err != nil {
			t.Fatal(err)
		}
		value := forgeJSON(t, out)
		for key := range value {
			clone := forgeJSON(t, value)
			delete(clone, key)
			raw, _ := json.Marshal(clone)
			if err := schema.ValidateJSON(op.Spec().OutputSchema, raw); err == nil {
				t.Fatal("missing root accepted", op.Spec().Name, key)
			}
		}
	}
	failure := errors.New("owned settlement failure")
	a.endErr = failure
	before := len(p.calls)
	_, err := d.Dispatch(context.Background(), opapi.Caller{}, "toolforge_remove", forgeLifecycleRaw("toolforge_remove"), nil)
	if !errors.Is(err, failure) || len(p.calls) != before+1 || a.cause != nil {
		t.Fatal(err, p, a)
	}
	p.err = errors.New("owned writer failure")
	_, err = d.Dispatch(context.Background(), opapi.Caller{}, "toolforge_remove", forgeLifecycleRaw("toolforge_remove"), nil)
	if !errors.Is(err, failure) || !errors.Is(err, p.err) || a.cause != p.err {
		t.Fatal(err, p, a)
	}
}
