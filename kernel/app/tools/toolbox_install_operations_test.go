// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/toolbox"
	"reflect"
	"testing"
)

type toolboxEmitterProbe struct {
	frames   []event.Event
	contexts []context.Context
	err      error
}

func (p *toolboxEmitterProbe) Emit(ctx context.Context, value any) error {
	p.contexts = append(p.contexts, ctx)
	p.frames = append(p.frames, value.(event.Event))
	return p.err
}
func toolboxStreamDispatcher(t *testing.T, p *toolboxInstallProbe, a *forgeAuditProbe, kind opapi.PrincipalKind, factory *int, factoryCtx *context.Context) (*app.Dispatcher, app.Operation) {
	t.Helper()
	ops, err := ToolboxInstallOperations(func(ctx context.Context) *ToolboxInstall {
		*factory++
		*factoryCtx = ctx
		return NewToolboxInstall(p, func(event.Kind, map[string]any) error { return nil })
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{kind}, Router: inventoryRoute{}, Audit: a})
	if err != nil {
		t.Fatal(err)
	}
	return d, ops[0]
}
func TestToolboxInstallTypedStreamPolicySchemaPayloadAndAudit(t *testing.T) {
	if _, err := ToolboxInstallOperations(nil); err == nil {
		t.Fatal("missing provider")
	}
	p := &toolboxInstallProbe{results: map[string]toolbox.InstallResult{" raw ": {Tool: "reported", OK: true, OutputTail: "owned"}}}
	a := &forgeAuditProbe{}
	factory := 0
	var factoryCtx context.Context
	d, op := toolboxStreamDispatcher(t, p, a, opapi.Operator, &factory, &factoryCtx)
	spec := op.Spec()
	if spec.Name != "toolbox_install" || spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamEvents || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[ToolboxInstallRequest]() || spec.Output != reflect.TypeFor[ToolboxInstallOutput]() || spec.Emission != reflect.TypeFor[event.Event]() {
		t.Fatal(spec)
	}
	emitter := &toolboxEmitterProbe{}
	ctx := context.WithValue(context.Background(), struct{ owned bool }{true}, "owned")
	out, err := d.Dispatch(ctx, opapi.Caller{}, spec.Name, json.RawMessage(`{"names":[false,"",null," raw "," raw ",1],"unknown":true,"tenant":"spoof"}`), emitter)
	raw, _ := json.Marshal(out)
	if err != nil || schema.ValidateJSON(spec.OutputSchema, raw) != nil || factory != 1 || factoryCtx != ctx || a.begins != 1 || a.ends != 1 || a.cause != nil || len(emitter.frames) != 2 || !reflect.DeepEqual(p.names, []string{" raw ", " raw "}) {
		t.Fatal(out, err, p, a, emitter)
	}
	for i, e := range emitter.frames {
		if emitter.contexts[i] != ctx || p.contexts[i] != ctx || e.CorrelationID != "" || e.Seq != 0 || e.Subject != "toolbox.install" || e.Kind != event.KindToolboxProgress {
			t.Fatal(e)
		}
		frame, _ := json.Marshal(e)
		if err := schema.ValidateJSON(spec.EmissionSchema, frame); err != nil {
			t.Fatal(err)
		}
		value := forgeJSON(t, e)
		delete(value, "payload")
		frame, _ = json.Marshal(value)
		if err := schema.ValidateJSON(spec.EmissionSchema, frame); err == nil {
			t.Fatal("missing payload accepted")
		}
		for _, key := range []string{"tool", "ok"} {
			value := forgeJSON(t, e)
			payload := value["payload"].(map[string]any)
			delete(payload, key)
			frame, _ = json.Marshal(value)
			if err := schema.ValidateJSON(spec.EmissionSchema, frame); err == nil {
				t.Fatal("missing payload key accepted", key)
			}
		}
	}
}
func TestToolboxInstallStreamAdmissionStopsFactoryAndEffects(t *testing.T) {
	failure := errors.New("owned audit failure")
	for _, mode := range []string{"begin-failure", "tenant", "canceled", "cancel-after-begin", "no-emitter"} {
		p := &toolboxInstallProbe{}
		a := &forgeAuditProbe{}
		factory := 0
		var factoryCtx context.Context
		kind := opapi.Operator
		ctx, cancel := context.WithCancel(context.Background())
		switch mode {
		case "begin-failure":
			a.beginErr = failure
		case "tenant":
			kind = opapi.Tenant
		case "canceled":
			cancel()
		case "cancel-after-begin":
			a.cancel = cancel
		}
		d, op := toolboxStreamDispatcher(t, p, a, kind, &factory, &factoryCtx)
		var emitter opapi.Emitter = &toolboxEmitterProbe{}
		if mode == "no-emitter" {
			emitter = nil
		}
		_, err := d.Dispatch(ctx, opapi.Caller{}, op.Spec().Name, json.RawMessage(`{"names":["owned"]}`), emitter)
		cancel()
		if err == nil || factory != 0 || len(p.names) != 0 {
			t.Fatal(mode, err, factory, p.names)
		}
		begins, ends := 0, 0
		if mode == "begin-failure" {
			begins = 1
			if !errors.Is(err, failure) {
				t.Fatal(err)
			}
		}
		if mode == "cancel-after-begin" {
			begins, ends = 1, 1
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		}
		if a.begins != begins || a.ends != ends {
			t.Fatal(mode, a)
		}
	}
}
func TestToolboxInstallNativeNamesCodecAndEmptyInputBeforeFactory(t *testing.T) {
	for _, raw := range []string{`{}`, `{"names":null}`, `{"names":false}`, `{"names":1}`, `{"names":"owned"}`, `{"names":{}}`, `{"names":[]}`, `{"names":[false,null,1,""]}`} {
		p := &toolboxInstallProbe{}
		a := &forgeAuditProbe{}
		factory := 0
		var factoryCtx context.Context
		d, op := toolboxStreamDispatcher(t, p, a, opapi.Operator, &factory, &factoryCtx)
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, op.Spec().Name, json.RawMessage(raw), &toolboxEmitterProbe{})
		if err == nil || err.Error() != "args.names (non-empty list) required" || factory != 0 || len(p.names) != 0 || a.begins != 1 || a.ends != 1 {
			t.Fatal(raw, err, factory, p, a)
		}
	}
}
func TestToolboxInstallStreamEmitterFailureStopsNextItemAndJoinsSettlement(t *testing.T) {
	failure := errors.New("owned emitter failure")
	endFailure := errors.New("owned settlement failure")
	p := &toolboxInstallProbe{results: map[string]toolbox.InstallResult{"one": {OK: true}, "two": {OK: true}}}
	a := &forgeAuditProbe{endErr: endFailure}
	factory := 0
	var factoryCtx context.Context
	d, op := toolboxStreamDispatcher(t, p, a, opapi.Operator, &factory, &factoryCtx)
	emitter := &toolboxEmitterProbe{err: failure}
	_, err := d.Dispatch(context.Background(), opapi.Caller{}, op.Spec().Name, json.RawMessage(`{"names":["one","two"]}`), emitter)
	if !errors.Is(err, failure) || !errors.Is(err, endFailure) || len(p.names) != 1 || a.begins != 1 || a.ends != 1 || a.cause != failure {
		t.Fatal(err, p, a, emitter)
	}
}
func TestToolboxInstallStreamSummaryRequiresThreeArrays(t *testing.T) {
	p := &toolboxInstallProbe{}
	a := &forgeAuditProbe{}
	factory := 0
	var factoryCtx context.Context
	_, op := toolboxStreamDispatcher(t, p, a, opapi.Operator, &factory, &factoryCtx)
	var declared map[string]any
	_ = json.Unmarshal(op.Spec().OutputSchema, &declared)
	if len(declared["required"].([]any)) != 3 || len(declared["properties"].(map[string]any)) != 3 {
		t.Fatal(declared)
	}
	value := forgeJSON(t, ToolboxInstallOutput{Installed: []string{}, Failed: []string{}, Skipped: []string{}})
	if len(value) != 3 {
		t.Fatal(value)
	}
	raw, _ := json.Marshal(value)
	if err := schema.ValidateJSON(op.Spec().OutputSchema, raw); err != nil {
		t.Fatal(err)
	}
	for key := range value {
		clone := forgeJSON(t, value)
		delete(clone, key)
		raw, _ := json.Marshal(clone)
		if err := schema.ValidateJSON(op.Spec().OutputSchema, raw); err == nil {
			t.Fatal("missing summary accepted", key)
		}
	}
}

func TestToolboxInstallNamesCodecKeepsRawDuplicatesAndDropsOnlyEmpty(t *testing.T) {
	got := toolboxInstallNames(json.RawMessage(`["",false," raw ",1," raw ",null," "]`))
	want := []string{" raw ", " raw ", " "}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got, want)
	}
	for _, raw := range []string{"null", "false", `"name"`, `{}`} {
		if names := toolboxInstallNames(json.RawMessage(raw)); names != nil {
			t.Fatal(raw, names)
		}
	}
	if names := toolboxInstallNames(json.RawMessage(`[]`)); names == nil || len(names) != 0 {
		t.Fatal(names)
	}
}
