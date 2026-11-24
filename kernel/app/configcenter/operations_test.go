// SPDX-License-Identifier: MIT
package configcenter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type centerAuth struct{ kind opapi.PrincipalKind }

func (a centerAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: a.kind, Tenant: "owned"}, nil
}

type centerRoute struct{}

func (centerRoute) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type centerAudit struct {
	begin, end int
	err        error
	names      []string
	causes     []error
}

func (a *centerAudit) Begin(_ context.Context, r opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begin++
	a.names = append(a.names, r.Operation)
	if a.err != nil {
		return nil, a.err
	}
	return a, nil
}
func (a *centerAudit) End(_ context.Context, err error) error {
	a.end++
	a.causes = append(a.causes, err)
	return nil
}
func centerDispatcher(t *testing.T, ops []app.Operation, kind opapi.PrincipalKind, audit *centerAudit) *app.Dispatcher {
	t.Helper()
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: centerAuth{kind}, Router: centerRoute{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestConfigCenterTypedNineMetadataContextAndAdmission(t *testing.T) {
	if _, err := Operations(nil, func(context.Context) *Writes { return nil }); err == nil {
		t.Fatal("nil reader accepted")
	}
	if _, err := Operations(func(context.Context) *Reads { return nil }, nil); err == nil {
		t.Fatal("nil writer accepted")
	}
	rp := &configCenterReadProbe{entry: &core.ConfigEntry{Key: "owned", Rating: core.RatingPublic}, stats: map[string]any{"total_entries": 0, "by_rating": map[string]int{}}}
	wp := &centerWriteProbe{entry: &core.ConfigEntry{Key: "owned", Value: "value", Rating: core.RatingPublic}, auto: core.RatingPublic}
	rf, wf := 0, 0
	var readCtx, writeCtx context.Context
	ops, err := Operations(func(ctx context.Context) *Reads { rf++; readCtx = ctx; return NewReads(rp) }, func(ctx context.Context) *Writes { wf++; writeCtx = ctx; return NewWrites(wp) })
	if err != nil || len(ops) != 9 {
		t.Fatal(ops, err)
	}
	audit := &centerAudit{}
	d := centerDispatcher(t, ops, opapi.Operator, audit)
	type marker struct{}
	ctx := context.WithValue(context.Background(), marker{}, "owned")
	cases := []struct {
		name, raw     string
		input, output reflect.Type
		read          bool
	}{
		{"configcenter.get", `{"key":"owned","unknown":true}`, reflect.TypeFor[KeyRequest](), reflect.TypeFor[GetOutput](), true},
		{"configcenter.list", `{"rating":"PUBLIC","unused":true}`, reflect.TypeFor[ListRequest](), reflect.TypeFor[ListOutput](), true},
		{"configcenter.access-log", `{"key":" raw ","agent_id":" Agent ","since":"-1h"}`, reflect.TypeFor[AccessLogRequest](), reflect.TypeFor[AccessLogOutput](), true},
		{"configcenter.audit", `{"since":"0","key":"ignored"}`, reflect.TypeFor[SinceRequest](), reflect.TypeFor[AuditOutput](), true},
		{"configcenter.health", `{"key":"ignored","rating":"ignored"}`, reflect.TypeFor[HealthInput](), reflect.TypeFor[HealthOutput](), true},
		{"configcenter.set", `{"key":" raw ","value":" raw value ","rating":"PUBLIC","description":" raw ","allowed_agents":"First,first; Second","excluded_agents":["denied"]}`, reflect.TypeFor[SetRequest](), reflect.TypeFor[SetOutput](), false},
		{"configcenter.delete", `{"key":" raw ","rating":"ignored"}`, reflect.TypeFor[KeyRequest](), reflect.TypeFor[DeleteOutput](), false},
		{"configcenter.set-rating", `{"key":" raw ","rating":"PUBLIC"}`, reflect.TypeFor[SetRatingRequest](), reflect.TypeFor[SetRatingOutput](), false},
		{"configcenter.access", `{"key":" raw ","allowed_agents":[],"excluded_agents":" Raw "}`, reflect.TypeFor[SetAccessRequest](), reflect.TypeFor[SetAccessOutput](), false},
	}
	for _, tc := range cases {
		found := false
		for _, sp := range d.Specs() {
			if sp.Name == tc.name {
				found = true
				if sp.ReadOnly != tc.read || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != tc.input || sp.Output != tc.output || sp.Emission != nil || len(sp.EmissionSchema) != 0 {
					t.Fatal(sp)
				}
			}
		}
		if !found {
			t.Fatal(tc.name)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if err != nil || reflect.TypeOf(out) != tc.output {
			t.Fatal(tc.name, out, err)
		}
		wire, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if tc.name == "configcenter.list" || tc.name == "configcenter.audit" {
			if !strings.Contains(string(wire), `"entries":[]`) {
				t.Fatal("empty entries changed", string(wire))
			}
		}
		if tc.name == "configcenter.access-log" && !strings.Contains(string(wire), `"logs":[]`) {
			t.Fatal("empty logs changed", string(wire))
		}
		if tc.name == "configcenter.set" && (wp.saved.Key != " raw " || wp.saved.Value != " raw value " || wp.saved.Rating != core.RatingPublic || !reflect.DeepEqual(wp.saved.AllowedAgents, []string{"First", "Second"}) || !reflect.DeepEqual(wp.saved.ExcludedAgents, []string{"denied"})) {
			t.Fatal("set codec changed", wp.saved)
		}
		if tc.name == "configcenter.access" && (wp.saved.AllowedAgents == nil || len(wp.saved.AllowedAgents) != 0 || !reflect.DeepEqual(wp.saved.ExcludedAgents, []string{"Raw"})) {
			t.Fatal("access codec changed", wp.saved)
		}
		if tc.read && readCtx != ctx || !tc.read && writeCtx != ctx {
			t.Fatal("factory context lost")
		}
	}
	if rf != 5 || wf != 4 || audit.begin != 4 || audit.end != 4 || !reflect.DeepEqual(audit.names, []string{opapi.ConfigCenterSet, "configcenter.delete", "configcenter.set-rating", "configcenter.access"}) {
		t.Fatal(rf, wf, audit)
	}
	readCalls, writeCalls := len(rp.calls), len(wp.calls)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	tenant := centerDispatcher(t, ops, opapi.Tenant, audit)
	for _, tc := range cases {
		if _, err := d.Dispatch(canceled, opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); !errors.Is(err, context.Canceled) {
			t.Fatal(tc.name, err)
		}
		if _, err := tenant.Dispatch(ctx, opapi.Caller{Tenant: "owned"}, tc.name, json.RawMessage(tc.raw), nil); err == nil {
			t.Fatal("tenant admitted", tc.name)
		}
	}
	audit.err = errors.New("owned closed journal")
	for _, tc := range cases {
		if !tc.read {
			if _, err := d.Dispatch(ctx, opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); !errors.Is(err, audit.err) {
				t.Fatal(tc.name, err)
			}
		}
	}
	if rf != 5 || wf != 4 || len(rp.calls) != readCalls || len(wp.calls) != writeCalls || audit.end != 4 {
		t.Fatal("rejected admission reached factory/effects", rf, wf, audit)
	}
}

func TestConfigCenterTypedCodecsStrictTextOrderAndPermissiveACL(t *testing.T) {
	rp := &configCenterReadProbe{entry: &core.ConfigEntry{}}
	wp := &centerWriteProbe{entry: &core.ConfigEntry{}}
	ops, err := Operations(func(context.Context) *Reads { return NewReads(rp) }, func(context.Context) *Writes { return NewWrites(wp) })
	if err != nil {
		t.Fatal(err)
	}
	d := centerDispatcher(t, ops, opapi.Operator, &centerAudit{})
	// A body whose TYPE is wrong is refused at the door: it never reaches the
	// audit and never reaches the handler. A body that is type-correct but
	// semantically wrong still reaches the codec, and the codec's own message
	// is what the daemon answers.
	for _, tc := range []struct {
		name, raw, want string
		door            bool
		read            bool
	}{
		{"configcenter.set", `{}`, "args.key required", false, false},
		{"configcenter.set", `{"key":null}`, "$.key has wrong type: got null, want string", true, false},
		{"configcenter.set", `{"value":false}`, "$.value has wrong type: got boolean, want string", true, false},
		{"configcenter.set", `{"key":" ","value":"v"}`, "args.key required", false, false},
		{"configcenter.set", `{"key":"k"}`, "args.value required", false, false},
		{"configcenter.set", `{"key":"k","value":null}`, "$.value has wrong type: got null, want string", true, false},
		{"configcenter.set", `{"key":"k","value":" "}`, "args.value required", false, false},
		{"configcenter.set", `{"key":"k","value":"v","rating":false}`, "$.rating has wrong type: got boolean, want string", true, false},
		{"configcenter.set", `{"key":"k","value":"v","description":null}`, "$.description has wrong type: got null, want string", true, false},
		{"configcenter.set", `{"key":"k","value":"v","rating":"invalid"}`, "invalid rating: invalid", false, false},
		{"configcenter.set", `{"key":"k","value":"v","rating":" PUBLIC "}`, "invalid rating:  PUBLIC ", false, false},
		{"configcenter.get", `{"key":false}`, "$.key has wrong type: got boolean, want string", true, true},
		{"configcenter.get", `{}`, "args.key required", false, true},
		{"configcenter.get", `{"key":" "}`, "args.key required", false, true},
		{"configcenter.delete", `{}`, "args.key required", false, false},
		{"configcenter.delete", `{"key":null}`, "$.key has wrong type: got null, want string", true, false},
		{"configcenter.set-rating", `{"key":"k"}`, "args.rating required", false, false},
		{"configcenter.set-rating", `{"key":"k","rating":null}`, "$.rating has wrong type: got null, want string", true, false},
		{"configcenter.set-rating", `{"key":"k","rating":" PUBLIC "}`, "invalid rating:  PUBLIC  (expected: public, internal, restricted, secret)", false, false},
		{"configcenter.access", `{}`, "args.key required", false, false},
		{"configcenter.list", `{"rating":null}`, "$.rating has wrong type: got null, want string", true, true},
		{"configcenter.access-log", `{"key":null}`, "$.key has wrong type: got null, want string", true, true},
		{"configcenter.access-log", `{"agent_id":false}`, "$.agent_id has wrong type: got boolean, want string", true, true},
		{"configcenter.access-log", `{"since":null}`, "$.since has wrong type: got null, want string", true, true},
		{"configcenter.audit", `{"since":false}`, "$.since has wrong type: got boolean, want string", true, true},
	} {
		before := len(rp.calls) + len(wp.calls)
		audit := &centerAudit{}
		one := centerDispatcher(t, ops, opapi.Operator, audit)
		_, err := one.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if err == nil {
			t.Fatal(tc.name, tc.raw, err)
		}
		if len(rp.calls)+len(wp.calls) != before {
			t.Fatal("invalid codec touched manager")
		}
		if tc.door {
			// Refused at admission: nothing was audited and the codec never
			// ran.
			if !strings.Contains(err.Error(), tc.want) || audit.begin != 0 || audit.end != 0 {
				t.Fatal(tc.name, tc.raw, err, audit)
			}
			continue
		}
		if err.Error() != tc.want {
			t.Fatal(tc.name, tc.raw, err)
		}
		if tc.read {
			// A read is never audited; the codec's own error is the answer.
			if audit.begin != 0 || audit.end != 0 {
				t.Fatal(tc.name, tc.raw, err, audit)
			}
			continue
		}
		if audit.begin != 1 || audit.end != 1 || audit.causes[0] == nil {
			t.Fatal(tc.name, tc.raw, err, audit)
		}
	}
	for _, tc := range []struct {
		raw  string
		want []string
	}{
		{"null", nil}, {"false", nil}, {`{}`, nil}, {`" "`, nil}, {`[]`, []string{}}, {`[null,false,1,{}]`, []string{}},
		{`" First,first;Second\nthird\t Fourth "`, []string{"First", "Second", "third", "Fourth"}},
		{`[" Raw ","raw","SECOND","",false]`, []string{"Raw", "SECOND"}},
		{`"one\rtwo"`, []string{"one\rtwo"}},
	} {
		if got := stringList(json.RawMessage(tc.raw)); !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc.raw, got, tc.want)
		}
	}
	if stringList(nil) != nil {
		t.Fatal("missing ACL changed")
	}
	_, err = d.Dispatch(context.Background(), opapi.Caller{}, "configcenter.set", json.RawMessage(`{"key":" raw ","value":" value ","description":" desc ","allowed_agents":["First","first"]}`), nil)
	if err != nil || wp.saved.Key != " raw " || wp.saved.Value != " value " || wp.saved.Description != " desc " || !reflect.DeepEqual(wp.saved.AllowedAgents, []string{"First"}) || wp.saved.ExcludedAgents != nil {
		t.Fatal(wp, err)
	}
	// A mistyped access list is a usage error: the door refuses the body, the
	// codec never runs, nothing is audited and the manager is never touched.
	for _, tc := range []struct{ raw, want string }{
		{`{"key":" raw ","value":" value ","excluded_agents":true}`, "$.excluded_agents has wrong type: got boolean, want string|array"},
		{`{"key":" raw ","value":" value ","allowed_agents":["First",false]}`, "$.allowed_agents[1] has wrong type: got boolean, want string"},
	} {
		before := len(wp.calls)
		audit := &centerAudit{}
		refused := centerDispatcher(t, ops, opapi.Operator, audit)
		_, err = refused.Dispatch(context.Background(), opapi.Caller{}, "configcenter.set", json.RawMessage(tc.raw), nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) || len(wp.calls) != before || audit.begin != 0 || audit.end != 0 {
			t.Fatal(tc.raw, err, wp, audit)
		}
	}
}

func TestConfigCenterTypedDTOSchemasRequiredOptionalEmptyAndLargeIntegers(t *testing.T) {
	ops, err := Operations(func(context.Context) *Reads { return NewReads(nil) }, func(context.Context) *Writes { return NewWrites(nil) })
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range ops {
		var shape struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		if err := json.Unmarshal(op.Spec().OutputSchema, &shape); err != nil {
			t.Fatal(err)
		}
		wantProps, wantRequired := 2, 2
		switch op.Spec().Name {
		case "configcenter.get", "configcenter.set", "configcenter.access", "configcenter.delete", "configcenter.set-rating":
			wantProps, wantRequired = 1, 1
		case "configcenter.health":
			wantProps, wantRequired = 3, 2
		}
		if len(shape.Properties) != wantProps || len(shape.Required) != wantRequired {
			t.Fatal(op.Spec().Name, shape)
		}
		if raw, ok := shape.Properties["entry"]; ok {
			var entry struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			}
			if json.Unmarshal(raw, &entry) != nil || len(entry.Properties) != 12 || len(entry.Required) != 6 {
				t.Fatal(entry)
			}
		}
	}
	e := &core.ConfigEntry{Key: "raw", Value: "owned-sensitive-middle-value", Rating: core.RatingSecret, CreatedAt: 9007199254740993, UpdatedAt: 0, Version: -2}
	row := entryRow(e)
	raw, err := json.Marshal(GetOutput{Entry: row})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Entry map[string]json.RawMessage `json:"entry"`
	}
	json.Unmarshal(raw, &wire)
	if string(wire.Entry["created_at"]) != "9007199254740993" || string(wire.Entry["updated_at"]) != "0" || string(wire.Entry["version"]) != "-2" || len(wire.Entry) != 7 || string(wire.Entry["masked"]) != "true" {
		t.Fatal(string(raw))
	}
	for _, value := range []any{ListOutput{Entries: []EntryRow{}}, AccessLogOutput{Logs: []AccessLogRow{}}, AuditOutput{Entries: []AuditRow{}}} {
		raw, _ := json.Marshal(value)
		var wire map[string]json.RawMessage
		json.Unmarshal(raw, &wire)
		if len(wire) != 2 || string(wire["count"]) != "0" {
			t.Fatal(string(raw))
		}
		delete(wire, "count")
		for _, collection := range wire {
			if string(collection) != "[]" {
				t.Fatal(string(raw))
			}
		}
	}
	for _, value := range []any{DeleteOutput{Deleted: false}, SetRatingOutput{Override: false}} {
		raw, _ := json.Marshal(value)
		var wire map[string]json.RawMessage
		json.Unmarshal(raw, &wire)
		if len(wire) != 1 {
			t.Fatal(string(raw))
		}
		for _, flag := range wire {
			if string(flag) != "false" {
				t.Fatal(string(raw))
			}
		}
	}
	for _, stats := range []map[string]any{nil, {}, {"total_entries": 0, "by_rating": map[string]int{}}} {
		out, err := NewReads(&configCenterReadProbe{stats: stats}).Health(context.Background(), HealthInput{})
		raw, _ := json.Marshal(out)
		var wire map[string]json.RawMessage
		json.Unmarshal(raw, &wire)
		if err != nil || len(wire) != 3 || wire["stats"] == nil {
			t.Fatal(string(raw), err)
		}
	}
}
