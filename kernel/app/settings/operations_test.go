// SPDX-License-Identifier: MIT
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/settings"
	"reflect"
	"strings"
	"testing"
)

type settingsAuth struct{ kind opapi.PrincipalKind }

func (a settingsAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: a.kind, Tenant: "acme"}, nil
}

type settingsRoute struct{}

func (settingsRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type settingsAudit struct {
	begin, end int
	err        error
	operations []string
	causes     []error
}

func (a *settingsAudit) Begin(_ context.Context, record opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begin++
	a.operations = append(a.operations, record.Operation)
	if a.err != nil {
		return nil, a.err
	}
	return a, nil
}
func (a *settingsAudit) End(_ context.Context, cause error) error {
	a.end++
	a.causes = append(a.causes, cause)
	return nil
}
func settingsOpMap(t *testing.T, reads func(context.Context) *Reads, writes func(context.Context) *Writes) map[string]app.Operation {
	t.Helper()
	ops, err := Operations(reads, writes)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]app.Operation{}
	for _, op := range ops {
		out[op.Spec().Name] = op
	}
	return out
}
func TestSettingsTypedFiveOperationsMetadataAdmissionContextAndAudit(t *testing.T) {
	if _, err := Operations(nil, func(context.Context) *Writes { return nil }); err == nil {
		t.Fatal("nil reads")
	}
	if _, err := Operations(func(context.Context) *Reads { return nil }, nil); err == nil {
		t.Fatal("nil writes")
	}
	rp := &settingsReadProbe{}
	wp := &settingsWriteProbe{field: coreFieldForSettingsOps(), found: true}
	rf, wf := 0, 0
	var seenRead, seenWrite context.Context
	ops, err := Operations(func(ctx context.Context) *Reads { rf++; seenRead = ctx; return NewReads(rp) }, func(ctx context.Context) *Writes { wf++; seenWrite = ctx; return NewWrites(wp) })
	if err != nil || len(ops) != 5 {
		t.Fatal(err, ops)
	}
	audit := &settingsAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: settingsAuth{opapi.Operator}, Router: settingsRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	type settingsCallerKey struct{}
	ctx := context.WithValue(context.Background(), settingsCallerKey{}, "owned")
	cases := []struct {
		name, raw     string
		input, output reflect.Type
		read          bool
	}{{"config_schema", `{"unknown":true,"name":"ignored"}`, reflect.TypeFor[SchemaInput](), reflect.TypeFor[SchemaOutput](), true}, {"config_values", `{"unknown":true,"value":"ignored"}`, reflect.TypeFor[ValuesInput](), reflect.TypeFor[ValuesOutput](), true}, {"config_set", `{"name":" owned ","value":" raw ","unknown":true}`, reflect.TypeFor[SetRequest](), reflect.TypeFor[SetOutput](), false}, {"config_schema_register", `{"section":{"id":" raw id ","name":" raw name ","fields":[]},"unknown":true}`, reflect.TypeFor[RegisterRequest](), reflect.TypeFor[RegisterOutput](), false}, {"config_schema_unregister", `{"id":" raw-id ","force":true,"unknown":true}`, reflect.TypeFor[UnregisterRequest](), reflect.TypeFor[UnregisterOutput](), false}}
	for _, tc := range cases {
		sp := d.Specs()
		found := false
		for _, s := range sp {
			if s.Name == tc.name {
				found = true
				if s.ReadOnly != tc.read || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.Input != tc.input || s.Output != tc.output || s.Emission != nil || len(s.EmissionSchema) != 0 {
					t.Fatal(s)
				}
			}
		}
		if !found {
			t.Fatal(tc.name)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		raw, _ := json.Marshal(out)
		for _, s := range sp {
			if s.Name == tc.name && schema.ValidateJSON(s.OutputSchema, raw) != nil {
				t.Fatal(string(raw))
			}
		}
		if err != nil {
			t.Fatal(tc.name, err)
		}
		if tc.read && seenRead != ctx || !tc.read && seenWrite != ctx {
			t.Fatal("operation factory lost caller context", tc.name)
		}
		if tc.name == "config_schema_unregister" && (wp.id != "raw-id" || !wp.force) {
			t.Fatal("unregister codec changed id/force", wp.id, wp.force)
		}
		if tc.name == "config_schema_register" && (wp.section.ID != " raw id " || wp.section.Name != " raw name " || wp.section.Fields == nil) {
			t.Fatal("register codec changed raw section", wp.section)
		}
	}
	if rf != 2 || wf != 3 || seenRead != ctx || seenWrite != ctx || audit.begin != 3 || audit.end != 3 || !reflect.DeepEqual(audit.operations, []string{opapi.ConfigSet, "config_schema_register", "config_schema_unregister"}) {
		t.Fatal(rf, wf, audit)
	}
	beforeCalls := len(wp.calls)
	beforeReads := len(rp.calls)
	ctxCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	tenant, _ := app.NewDispatcher(ops, app.Dependencies{Auth: settingsAuth{opapi.Tenant}, Router: settingsRoute{}, Audit: audit})
	for _, tc := range cases {
		if _, err := d.Dispatch(ctxCanceled, opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); err != context.Canceled {
			t.Fatal(tc.name, err)
		}
		if _, err := tenant.Dispatch(ctx, opapi.Caller{Tenant: "acme"}, tc.name, json.RawMessage(tc.raw), nil); err == nil {
			t.Fatal("tenant admitted", tc.name)
		}
	}
	audit.err = errors.New("closed journal")
	for _, tc := range cases[2:] {
		if _, err := d.Dispatch(ctx, opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); !errors.Is(err, audit.err) {
			t.Fatal(tc.name, err)
		}
	}
	if rf != 2 || wf != 3 || len(wp.calls) != beforeCalls || len(rp.calls) != beforeReads || audit.end != 3 {
		t.Fatal("admission reached factory/effects", rf, wf, audit)
	}
}
func coreFieldForSettingsOps() core.Field {
	return core.Field{Type: core.TypeText, Apply: core.ApplyRestart}
}
func TestSettingsTypedCodecsPreserveStrictErrorsAndOrdering(t *testing.T) {
	// A body whose TYPE is wrong is refused at the door: it never reaches the
	// audit and never reaches the handler. A body that is type-correct but
	// semantically wrong still reaches both, and the codec's own message is
	// what the daemon answers.
	for _, tc := range []struct {
		op, raw, want string
		door          bool
	}{
		{"config_set", `{}`, "args.name required", false},
		{"config_set", `{"name":null}`, "$.name has wrong type: got null, want string", true},
		{"config_set", `{"name":false}`, "$.name has wrong type: got boolean, want string", true},
		{"config_set", `{"name":" "}`, "args.name required", false},
		{"config_set", `{"name":"owned","value":null}`, "$.value has wrong type: got null, want string", true},
		{"config_set", `{"name":"owned","value":false}`, "$.value has wrong type: got boolean, want string", true},
		{"config_schema_register", `{}`, "args.section required", false},
		{"config_schema_register", `{"section":false}`, "$.section has wrong type: got boolean, want object", true},
		{"config_schema_unregister", `{}`, "args.id required", false},
		{"config_schema_unregister", `{"id":null}`, "$.id has wrong type: got null, want string", true},
		{"config_schema_unregister", `{"id":" "}`, "args.id required", false},
		{"config_schema_unregister", `{"id":"owned","force":null}`, "$.force has wrong type: got null, want boolean", true},
		{"config_schema_unregister", `{"id":"owned","force":"true"}`, "$.force has wrong type: got string, want boolean", true},
	} {
		t.Run(tc.op+tc.raw, func(t *testing.T) {
			wf := 0
			ops, _ := Operations(func(context.Context) *Reads { return NewReads(&settingsReadProbe{}) }, func(context.Context) *Writes { wf++; return NewWrites(&settingsWriteProbe{}) })
			audit := &settingsAudit{}
			d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: settingsAuth{opapi.Operator}, Router: settingsRoute{}, Audit: audit})
			_, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.op, json.RawMessage(tc.raw), nil)
			if err == nil || wf != 0 {
				t.Fatal(err, wf, audit)
			}
			if tc.door {
				// Refused at admission: nothing was audited and the codec
				// never ran.
				if !strings.Contains(err.Error(), tc.want) || audit.begin != 0 || audit.end != 0 {
					t.Fatal(err, wf, audit)
				}
				return
			}
			if err.Error() != tc.want || audit.begin != 1 || audit.end != 1 || audit.causes[0] == nil {
				t.Fatal(err, wf, audit)
			}
		})
	}
	for _, raw := range []string{`{"name":"owned"}`, `{"name":"owned","value":""}`} {
		p := &settingsWriteProbe{field: coreFieldForSettingsOps(), found: true}
		ops, _ := Operations(func(context.Context) *Reads { return nil }, func(context.Context) *Writes { return NewWrites(p) })
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: settingsAuth{opapi.Operator}, Router: settingsRoute{}, Audit: &settingsAudit{}})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "config_set", json.RawMessage(raw), nil); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p.calls, []string{"field:owned", "config", "load", "remove:owned", "save", "pin:owned"}) {
			t.Fatal(p.calls)
		}
	}
}
func TestSettingsTypedSchemasRequiredOptionalAndNullableShapes(t *testing.T) {
	ops := settingsOpMap(t, func(context.Context) *Reads { return nil }, func(context.Context) *Writes { return nil })
	decl := func(name string) map[string]any {
		return settingsWriterJSON(t, json.RawMessage(ops[name].Spec().OutputSchema))
	}
	check := func(value map[string]any, properties, required int) {
		t.Helper()
		props := value["properties"].(map[string]any)
		req, _ := value["required"].([]any)
		if len(props) != properties || len(req) != required {
			t.Fatal(value)
		}
	}
	schemaDecl := decl("config_schema")
	check(schemaDecl, 2, 2)
	section := schemaDecl["properties"].(map[string]any)["sections"].(map[string]any)["items"].(map[string]any)
	check(section, 6, 3)
	field := section["properties"].(map[string]any)["fields"].(map[string]any)["items"].(map[string]any)
	check(field, 10, 6)
	boundary := schemaDecl["properties"].(map[string]any)["reload_boundaries"].(map[string]any)["items"].(map[string]any)
	check(boundary, 2, 2)
	valuesDecl := decl("config_values")
	check(valuesDecl, 1, 1)
	row := valuesDecl["properties"].(map[string]any)["fields"].(map[string]any)["items"].(map[string]any)
	check(row, 5, 4)
	check(decl("config_set"), 5, 3)
	check(decl("config_schema_register"), 3, 3)
	check(decl("config_schema_unregister"), 2, 2)
	cases := []struct {
		name  string
		value any
	}{{"config_schema", SchemaOutput{Sections: nil, ReloadBoundaries: []core.ReloadBoundary{}}}, {"config_schema", SchemaOutput{Sections: []Section{{Fields: nil}}, ReloadBoundaries: []core.ReloadBoundary{}}}, {"config_values", ValuesOutput{Fields: []ValueRow{}}}, {"config_values", ValuesOutput{Fields: []ValueRow{{Secret: true, Set: false}, {Value: ptrSettingsString("")}}}}, {"config_set", SetOutput{Saved: true, ReloadError: ptrSettingsString("")}}, {"config_schema_register", RegisterOutput{Registered: true}}, {"config_schema_unregister", UnregisterOutput{Removed: false}}}
	for _, tc := range cases {
		raw, _ := json.Marshal(tc.value)
		sp := ops[tc.name].Spec()
		if err := schema.ValidateJSON(sp.OutputSchema, raw); err != nil {
			t.Fatal(tc.name, string(raw), err)
		}
		root := settingsWriterJSON(t, tc.value)
		declaration := decl(tc.name)
		required := declaration["required"].([]any)
		for _, key := range required {
			copy := settingsWriterJSON(t, root)
			delete(copy, key.(string))
			raw, _ := json.Marshal(copy)
			if schema.ValidateJSON(sp.OutputSchema, raw) == nil {
				t.Fatal("required root admitted missing", tc.name, key)
			}
		}
	}
	rowSchema, _ := json.Marshal(row)
	for _, key := range []string{"env", "secret", "env_pinned", "set"} {
		value := settingsWriterJSON(t, ValueRow{})
		delete(value, key)
		raw, _ := json.Marshal(value)
		if schema.ValidateJSON(rowSchema, raw) == nil {
			t.Fatal("required row admitted missing", key)
		}
	}
	value := settingsWriterJSON(t, ValuesOutput{Fields: []ValueRow{{Secret: true}, {Value: ptrSettingsString("")}}})
	rows := value["fields"].([]any)
	if _, ok := rows[0].(map[string]any)["value"]; ok {
		t.Fatal("secret value key present")
	}
	if rows[1].(map[string]any)["value"] != "" {
		t.Fatal("empty non-secret value omitted")
	}
	set := settingsWriterJSON(t, SetOutput{ReloadError: ptrSettingsString("")})
	if set["reload_error"] != "" {
		t.Fatal("empty error omitted")
	}
	if _, ok := set["env_pinned"]; ok {
		t.Fatal("false pin emitted")
	}
}
func ptrSettingsString(value string) *string { return &value }
