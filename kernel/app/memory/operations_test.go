// SPDX-License-Identifier: MIT

package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	appmemory "github.com/agezt/agezt/kernel/app/memory"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type memoryAuth struct{}

func (memoryAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type memoryRouter struct{}

func (memoryRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type memoryAudit struct {
	cause error
	calls int
}

func (a *memoryAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	if a.cause != nil {
		return nil, a.cause
	}
	return memorySpan{}, nil
}

type memorySpan struct{}

func (memorySpan) End(context.Context, error) error { return nil }

func TestMemorySpecsRetainCompleteTypedMetadataAndSchemas(t *testing.T) {
	operations, err := appmemory.Operations(func(context.Context) *appmemory.Service { return nil }, func(context.Context) *appmemory.Distillation { return nil }, func(context.Context) *appmemory.LogService { return nil })
	if err != nil {
		t.Fatal(err)
	}
	readOnly := map[string]bool{"memory_get": true, "memory_list": true, "memory_search": true, "memory_find_related": true, "memory_audit": true, "memory_log": true}
	tenant := map[string]bool{"memory_audit": true, "memory_clean": true, "memory_log": true}
	if len(operations) != 16 {
		t.Fatalf("operation count=%d", len(operations))
	}
	seen := map[string]bool{}
	want := map[string]bool{"memory_get": true, "memory_list": true, "memory_search": true, "memory_find_related": true,
		"memory_add": true, "memory_supersede": true, "memory_forget": true, "memory_promote": true, "memory_bulk_forget": true,
		"memory_prune": true, "memory_tidy": true, "memory_audit": true, "memory_clean": true, "memory_log": true, "memory_consolidate": true, "profile_rebuild": true}
	for _, operation := range operations {
		spec := operation.Spec()
		if !want[spec.Name] || seen[spec.Name] || spec.ReadOnly != readOnly[spec.Name] || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input == nil || spec.Output == nil {
			t.Fatalf("operation metadata=%+v", spec)
		}
		seen[spec.Name] = true
		if tenant[spec.Name] {
			if spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant {
				t.Fatalf("tenant scope=%+v", spec)
			}
		} else if spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary {
			t.Fatalf("primary scope=%+v", spec)
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
		case "memory_get", "memory_forget", "memory_promote", "memory_find_related":
			input = json.RawMessage(`{"id":"fixture","ignored":true}`)
		case "memory_search":
			input = json.RawMessage(`{"query":"fixture","ignored":true}`)
		case "memory_add":
			input = json.RawMessage(`{"content":"fixture","ignored":true}`)
		case "memory_supersede":
			input = json.RawMessage(`{"old_id":"fixture","content":"fixture","ignored":true}`)
		case "memory_bulk_forget":
			input = json.RawMessage(`{"ids":[],"ignored":true}`)
		default:
			input = json.RawMessage(`{"ignored":true}`)
		}
		if err := schema.ValidateJSON(spec.InputSchema, input); err != nil {
			t.Fatalf("%s compatible input=%v", spec.Name, err)
		}
		if spec.Name == "memory_add" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"content":"fixture","tags":null}`)) == nil {
			t.Fatal("null tags admitted")
		}
		if spec.Name == "memory_bulk_forget" && schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"ids":null}`)) == nil {
			t.Fatal("null ids admitted")
		}
	}
	if _, err := appmemory.Operations(nil, nil, nil); err == nil {
		t.Fatal("nil service providers admitted")
	}
}

func TestMemoryMutationAuditPrecedesEveryServiceEffect(t *testing.T) {
	s, service := readFixture(t)
	calls := 0
	target := &fakeDistiller{}
	operations, err := appmemory.Operations(func(context.Context) *appmemory.Service { calls++; return service }, func(context.Context) *appmemory.Distillation { calls++; return appmemory.NewDistillation(target) }, func(context.Context) *appmemory.LogService { calls++; return appmemory.NewLog(logReader{}) })
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("fixture audit unavailable")
	audit := &memoryAudit{cause: cause}
	dispatcher, err := app.NewDispatcher(operations, app.Dependencies{Auth: memoryAuth{}, Router: memoryRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, raw string }{
		{"memory_add", `{"content":"fixture"}`}, {"memory_supersede", `{"old_id":"missing","content":"fixture"}`},
		{"memory_forget", `{"id":"missing"}`}, {"memory_promote", `{"id":"missing"}`}, {"memory_bulk_forget", `{"ids":[]}`},
		{"memory_prune", `{}`}, {"memory_tidy", `{}`}, {"memory_clean", `{}`}, {"memory_consolidate", `{}`}, {"profile_rebuild", `{}`},
	} {
		_, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if !errors.Is(err, cause) {
			t.Errorf("%s audit cause=%v", tc.name, err)
		}
	}
	if audit.calls != 10 || calls != 0 || s.Count() != 0 || target.next != 0 {
		t.Fatalf("audit=%d service=%d records=%d distill=%d", audit.calls, calls, s.Count(), target.next)
	}
}

func TestMemoryTypedAdmissionCompatibilityAndReadOnlyAudit(t *testing.T) {
	s, service := readFixture(t)
	calls := 0
	operations, err := appmemory.Operations(func(context.Context) *appmemory.Service { calls++; return service }, func(context.Context) *appmemory.Distillation {
		calls++
		return appmemory.NewDistillation(&fakeDistiller{})
	}, func(context.Context) *appmemory.LogService { calls++; return appmemory.NewLog(logReader{}) })
	if err != nil {
		t.Fatal(err)
	}
	audit := &memoryAudit{}
	dispatcher, err := app.NewDispatcher(operations, app.Dependencies{Auth: memoryAuth{}, Router: memoryRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, raw string }{{"memory_add", `{"content":"fixture","tags":null}`}, {"memory_add", `{"content":true}`}, {"memory_bulk_forget", `{"ids":null}`}, {"memory_bulk_forget", `{"ids":[true]}`}, {"memory_tidy", `{"dry_run":null}`}, {"memory_clean", `{"dry_run":1}`}, {"memory_get", `{"id":1}`}} {
		if _, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil); err == nil {
			t.Errorf("%s invalid input admitted", tc.name)
		}
	}
	if audit.calls != 0 || calls != 0 || s.Count() != 0 {
		t.Fatalf("invalid input reached effects: audit=%d calls=%d count=%d", audit.calls, calls, s.Count())
	}
	for _, tc := range []struct{ name, raw string }{{"memory_get", `{"id":"missing"}`}, {"memory_list", `{}`}, {"memory_search", `{"query":"fixture"}`}, {"memory_find_related", `{"id":"missing"}`}, {"memory_audit", `{}`}, {"memory_log", `{"limit":"ignored","since_ms":"ignored"}`}} {
		_, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if tc.name != "memory_find_related" && err != nil {
			t.Errorf("%s read=%v", tc.name, err)
		}
	}
	if audit.calls != 0 {
		t.Fatalf("read-only requests audited=%d", audit.calls)
	}
	for _, command := range []string{"memory_prune", "memory_tidy", "memory_clean"} {
		for _, tc := range []struct {
			raw string
			dry bool
		}{{`{}`, true}, {`{"dry_run":false}`, false}, {`{"dry_run":"false"}`, false}, {`{"dry_run":"0"}`, false}, {`{"dry_run":"False"}`, true}} {
			out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, command, json.RawMessage(tc.raw), nil)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if wire["dry_run"] != tc.dry {
				t.Errorf("%s %s dry=%v", command, tc.raw, wire["dry_run"])
			}
		}
	}
	for _, tc := range []struct {
		value any
		days  int
	}{{"7", 7}, {float64(7.9), 7}, {"bad", 30}, {true, 30}, {float64(-1), 30}} {
		raw, err := json.Marshal(map[string]any{"older_than_days": tc.value})
		if err != nil {
			t.Fatal(err)
		}
		out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "memory_prune", raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := out.(appmemory.PruneOutput).OlderThanDays; got != tc.days {
			t.Errorf("legacy day input=%v got=%d want=%d", tc.value, got, tc.days)
		}
	}
	added, err := service.Remember(context.Background(), appmemory.RememberInput{Content: "trimmed bulk fixture"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"ids": []string{" ", "  " + added.ID + "  ", "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := dispatcher.Dispatch(context.Background(), opapi.Caller{}, "memory_bulk_forget", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	bulk := out.(appmemory.BulkForgetOutput)
	if bulk.Forgotten != 1 || bulk.NotFound != 1 {
		t.Fatalf("bulk normalization=%+v", bulk)
	}
}
