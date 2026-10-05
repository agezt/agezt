// SPDX-License-Identifier: MIT

package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"testing"
)

type skillAuth struct{}

func (skillAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type skillRouter struct{}

func (skillRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type skillAudit struct {
	cause error
	calls int
}

func (a *skillAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return skillSpan{}, a.cause
}

type skillSpan struct{}

func (skillSpan) End(context.Context, error) error { return nil }
func TestSkillSpecsRetainCompleteTypedMetadataAndActualSchemas(t *testing.T) {
	ops, err := appskill.Operations(func(context.Context) *appskill.Service { return nil }, func(context.Context) *appskill.Lifecycle { return nil }, func(context.Context) *appskill.Curation { return nil }, func(context.Context) *appskill.Observations { return nil })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"skill_list": true, "skill_get": true, "skill_history": true, "skill_files": true, "skill_read_file": true, "skill_hygiene": true, "skill_promote": false, "skill_quarantine": false, "skill_archive": false, "skill_revert": false, "skill_restore": false, "skill_share": false, "skill_reassign": false, "skill_import": false}
	if len(ops) != len(want) {
		t.Fatalf("skill operation count=%d", len(ops))
	}
	seen := map[string]bool{}
	http := map[string]opapi.HTTP{"skill_list": {Method: "GET", Path: "/api/skills"}, "skill_files": {Method: "GET", Path: "/api/skill/files"}, "skill_hygiene": {Method: "GET", Path: "/api/skills/hygiene"}, "skill_promote": {Method: "POST", Path: "/api/skill/promote"}, "skill_quarantine": {Method: "POST", Path: "/api/skill/quarantine"}, "skill_archive": {Method: "POST", Path: "/api/skill/archive"}, "skill_revert": {Method: "POST", Path: "/api/skill/revert"}, "skill_share": {Method: "POST", Path: "/api/skill/share"}, "skill_import": {Method: "POST", Path: "/api/skill/import"}}
	for _, op := range ops {
		spec := op.Spec()
		read, found := want[spec.Name]
		if !found || seen[spec.Name] || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input == nil || spec.Output == nil || spec.HTTP != http[spec.Name] {
			t.Fatalf("skill metadata=%+v", spec)
		}
		seen[spec.Name] = true
		raw, err := json.Marshal(reflect.Zero(spec.Output).Interface())
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("%s output=%v", spec.Name, err)
		}
		input := json.RawMessage(`{"id":"fixture","path":"ref.md","status":"draft","name":"fixture","body":"body","unused":true}`)
		if err := schema.ValidateJSON(spec.InputSchema, input); err != nil {
			t.Fatalf("%s input=%v", spec.Name, err)
		}
	}
	if _, err := appskill.Operations(nil, nil, nil, nil); err == nil {
		t.Fatal("nil providers admitted")
	}
}
func TestSkillMutationAuditPrecedesAllServiceFactories(t *testing.T) {
	calls := 0
	ops, err := appskill.Operations(func(context.Context) *appskill.Service { calls++; return nil }, func(context.Context) *appskill.Lifecycle { calls++; return nil }, func(context.Context) *appskill.Curation { calls++; return nil }, func(context.Context) *appskill.Observations { calls++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("owned audit unavailable")
	audit := &skillAudit{cause: cause}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: skillAuth{}, Router: skillRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skill_promote", "skill_quarantine", "skill_archive", "skill_revert", "skill_restore", "skill_share", "skill_reassign", "skill_import"} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"id":"fixture","status":"draft","name":"fixture","body":"body"}`), nil); !errors.Is(err, cause) {
			t.Fatalf("%s audit cause=%v", name, err)
		}
	}
	if calls != 0 || audit.calls != 8 {
		t.Fatalf("effects before audit: service=%d audit=%d", calls, audit.calls)
	}
}
func TestSkillTypedImportAndHygieneRetainNativeAdmission(t *testing.T) {
	forge, _ := lifecycleFixture(t)
	store := &observationStore{}
	r := &reader{}
	ops, err := appskill.Operations(func(context.Context) *appskill.Service { return appskill.New(r) }, func(context.Context) *appskill.Lifecycle { return appskill.NewLifecycle(forge) }, func(context.Context) *appskill.Curation { return appskill.NewCuration(forge, nil) }, func(context.Context) *appskill.Observations {
		return appskill.NewObservations(store, observationReader{})
	})
	if err != nil {
		t.Fatal(err)
	}
	audit := &skillAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: skillAuth{}, Router: skillRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	value, err := d.Dispatch(ctx, opapi.Caller{}, "skill_import", json.RawMessage(`{"name":" fixture ","body":" body ","description":" desc ","agent":" writer ","triggers":[" ci ",""],"tools_required":[" shell "],"unused":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	out := value.(appskill.ImportOutput)
	sk, found, err := forge.Get(out.ID)
	if err != nil || !found || sk.Name != "fixture" || sk.Body != "body" || sk.Description != "desc" || sk.Agent != "writer" || !reflect.DeepEqual(sk.Triggers, []string{"ci"}) || !reflect.DeepEqual(sk.ToolsRequired, []string{"shell"}) {
		t.Fatalf("imported=%+v err=%v", sk, err)
	}
	for _, raw := range []string{`{"name":true,"body":"body"}`, `{"name":"x","body":"body","triggers":null}`, `{"name":"x","body":"body","tools_required":null}`, `{"name":"x","body":"body","triggers":[true]}`, `{"name":"x","body":"body","resources":{"ref.md":true}}`} {
		if _, err := d.Dispatch(ctx, opapi.Caller{}, "skill_import", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("bad import accepted: %s", raw)
		}
	}
	before := audit.calls
	for _, tc := range []struct {
		raw  string
		days int
	}{{`{}`, 30}, {`{"idle_days":0.5}`, 30}, {`{"idle_days":-1}`, 30}, {`{"idle_days":"2"}`, 2}, {`{"idle_days":"1.2"}`, 30}, {`{"idle_days":true}`, 30}} {
		value, err := d.Dispatch(ctx, opapi.Caller{}, "skill_hygiene", json.RawMessage(tc.raw), nil)
		if err != nil || value.(appskill.HygieneOutput).IdleDays != tc.days {
			t.Fatalf("hygiene %s out=%+v err=%v", tc.raw, value, err)
		}
	}
	if audit.calls != before {
		t.Fatalf("read audited=%d vs %d", audit.calls, before)
	}
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "skill_get", json.RawMessage(`{"id":" fixture "}`), nil); err != nil || r.got != " fixture " {
		t.Fatalf("native ID bytes changed=%q err=%v", r.got, err)
	}
}
