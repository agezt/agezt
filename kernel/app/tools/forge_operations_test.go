// SPDX-License-Identifier: MIT
package tools

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/toolforge"
	"reflect"
	"testing"
)

func forgeDispatcher(t *testing.T, kind opapi.PrincipalKind, p *forgeReaderProbe, factory *int, audit *inventoryForbiddenAudit) (*app.Dispatcher, []app.Operation) {
	t.Helper()
	ops, err := ForgeReadOperations(func(context.Context) *ForgeCatalog { *factory++; return NewForgeCatalog(p) })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: inventoryAuth{kind}, Router: inventoryRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	return d, ops
}
func TestForgeReadOperationsTypedPoliciesSchemasAndUnauditedAdmission(t *testing.T) {
	if _, err := ForgeReadOperations(nil); err == nil {
		t.Fatal("missing provider")
	}
	port := &forgeReaderProbe{rows: []toolforge.ScriptTool{{ID: "owned", Name: "named", Code: "body"}}, found: true}
	factory := 0
	audit := &inventoryForbiddenAudit{}
	d, ops := forgeDispatcher(t, opapi.Operator, port, &factory, audit)
	if len(ops) != 2 {
		t.Fatal(ops)
	}
	for i, operation := range ops {
		spec := operation.Spec()
		input, output := reflect.TypeFor[ForgeListInput](), reflect.TypeFor[ForgeListOutput]()
		name := "toolforge_list"
		raw := `{"unknown":true,"tenant":"spoof"}`
		if i == 1 {
			input, output = reflect.TypeFor[ForgeShowRequest](), reflect.TypeFor[ForgeShowOutput]()
			name = "toolforge_show"
			raw = `{"ref":" owned ","unknown":true,"tenant":"spoof"}`
		}
		if spec.Name != name || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != input || spec.Output != output {
			t.Fatal(spec)
		}
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(raw), nil)
		if err != nil {
			t.Fatal(name, err)
		}
		encoded, _ := json.Marshal(out)
		if err = schema.ValidateJSON(spec.OutputSchema, encoded); err != nil {
			t.Fatal(string(encoded), err)
		}
	}
	if factory != 2 || port.listCalls != 1 || port.getCalls != 1 || port.getRef != "owned" || audit.calls != 0 {
		t.Fatal(factory, port, audit.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tenant, _ := forgeDispatcher(t, opapi.Tenant, port, &factory, audit)
	for _, op := range ops {
		for _, dispatch := range []*app.Dispatcher{d, tenant} {
			use := context.Background()
			if dispatch == d {
				use = ctx
			}
			if _, err := dispatch.Dispatch(use, opapi.Caller{}, op.Spec().Name, json.RawMessage(`{"ref":"owned"}`), nil); err == nil {
				t.Fatal("admission passed", op.Spec().Name)
			}
		}
	}
	if factory != 2 || port.listCalls != 1 || port.getCalls != 1 || audit.calls != 0 {
		t.Fatal(factory, port, audit.calls)
	}
}
func TestForgeShowNativeRefCodecRejectsBeforeFactory(t *testing.T) {
	port := &forgeReaderProbe{rows: []toolforge.ScriptTool{{}}, found: true}
	factory := 0
	d, _ := forgeDispatcher(t, opapi.Operator, port, &factory, &inventoryForbiddenAudit{})
	cases := []struct{ raw, want string }{{`{}`, "args.ref required"}, {`{"ref":null}`, "args.ref must be a string"}, {`{"ref":false}`, "args.ref must be a string"}, {`{"ref":1}`, "args.ref must be a string"}, {`{"ref":[]}`, "args.ref must be a string"}, {`{"ref":{}}`, "args.ref must be a string"}, {`{"ref":""}`, "args.ref required"}, {`{"ref":" \t "}`, "args.ref required"}}
	for _, tc := range cases {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "toolforge_show", json.RawMessage(tc.raw), nil); err == nil || err.Error() != tc.want {
			t.Fatal(tc.raw, err, tc.want)
		}
	}
	if factory != 0 || port.getCalls != 0 || port.listCalls != 0 {
		t.Fatal(factory, port)
	}
	for _, raw := range []string{`{"ref":"id"}`, `{"ref":"  id  "}`, `{"ref":"ö"}`, `{"ref":"id","ignored":true}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "toolforge_show", json.RawMessage(raw), nil); err != nil {
			t.Fatal(raw, err)
		}
	}
	if factory != 4 || port.getCalls != 4 || port.listCalls != 0 {
		t.Fatal(factory, port)
	}
	port.found = false
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "toolforge_show", json.RawMessage(`{"ref":"  missing  "}`), nil); err == nil || err.Error() != "unknown script tool:   missing  " || port.getRef != "missing" {
		t.Fatal(err, port)
	}

}
func TestForgeReadOutputSchemasRequireLightAndDetailFields(t *testing.T) {
	_, ops := forgeDispatcher(t, opapi.Operator, &forgeReaderProbe{rows: []toolforge.ScriptTool{{}}, found: true}, new(int), &inventoryForbiddenAudit{})
	required := []string{"id", "name", "description", "language", "status", "tested_ok", "created_ms", "updated_ms"}
	for i, operation := range ops {
		var declared map[string]any
		_ = json.Unmarshal(operation.Spec().OutputSchema, &declared)
		props := declared["properties"].(map[string]any)
		var rowSchema map[string]any
		var value map[string]any
		if i == 0 {
			if len(declared["required"].([]any)) != 3 {
				t.Fatal(declared)
			}
			rowSchema = props["tools"].(map[string]any)["items"].(map[string]any)
			value = forgeJSON(t, ForgeListOutput{Tools: []ForgeItem{{}}, Count: 1})
		} else {
			if len(declared["required"].([]any)) != 1 {
				t.Fatal(declared)
			}
			rowSchema = props["tool"].(map[string]any)
			value = forgeJSON(t, ForgeShowOutput{})
		}
		fields := append([]string{}, required...)
		if i == 1 {
			fields = append(fields, "code")
		}
		have := rowSchema["required"].([]any)
		if len(have) != len(fields) {
			t.Fatal(rowSchema)
		}
		rowProps := rowSchema["properties"].(map[string]any)
		if len(rowProps) != 11+i {
			t.Fatal(rowProps)
		}
		for _, field := range fields {
			found := false
			for _, v := range have {
				if v == field {
					found = true
				}
			}
			if !found {
				t.Fatal(field, have)
			}
			encoded, _ := json.Marshal(value)
			if err := schema.ValidateJSON(operation.Spec().OutputSchema, encoded); err != nil {
				t.Fatal(string(encoded), err)
			}
			clone := forgeJSON(t, value)
			var row map[string]any
			if i == 0 {
				row = clone["tools"].([]any)[0].(map[string]any)
			} else {
				row = clone["tool"].(map[string]any)
			}
			delete(row, field)
			encoded, _ = json.Marshal(clone)
			if err := schema.ValidateJSON(operation.Spec().OutputSchema, encoded); err == nil {
				t.Fatal("missing required field accepted", field)
			}
		}
	}
}
