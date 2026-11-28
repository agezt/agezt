// SPDX-License-Identifier: MIT

package workboard_test

// The workboard family's request rows live in the contract, and the family's
// own rules live in the family: a count that is not a count is the family's
// default, a sweep is capped at the family's cap, and a list of tags is a list
// of texts — or the ONE comma-separated string a caller sends instead of a
// list.
//
// The rows are read through the daemon's door, so a mistyped field is refused
// by the door with its own wording, before any effect — and a field that was
// not sent is not the same answer as a field that was sent as zero.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	appwork "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/contract/opapi"
	tasks "github.com/agezt/agezt/kernel/modules/work/workboard"
)

// workboardAuth admits the operator the family's operations are served to.
type workboardAuth struct{}

func (workboardAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

// workboardRouter routes every operation to its caller.
type workboardRouter struct{}

func (workboardRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

// workboardAudit counts the audits the family caused.
type workboardAudit struct{ calls int }

func (a *workboardAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return workboardSpan{}, nil
}

type workboardSpan struct{}

func (workboardSpan) End(context.Context, error) error { return nil }

// TestWorkboardTypedAdmissionRetainsLimitsAndRefusesMistypedText says the
// family's counts are the family's own, and that a mistyped field is refused
// by the door before any effect.
func TestWorkboardTypedAdmissionRetainsLimitsAndRefusesMistypedText(t *testing.T) {
	reader := &workReader{rows: []tasks.Task{{ID: "one", Status: tasks.StatusTriage}}}
	lifecycle := &lifecycleHost{}
	relations := &relationHost{}
	services := appwork.NativeServices{
		Reads:     appwork.New(reader),
		Lifecycle: appwork.NewLifecycle(lifecycle, lifecycle),
		Relations: appwork.NewRelations(relations),
		ValidSeat: func(string) bool { return true },
	}
	ops, err := appwork.Operations(func(context.Context) appwork.NativeServices { return services })
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 21 {
		t.Fatalf("operations=%d", len(ops))
	}
	audit := &workboardAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: workboardAuth{}, Router: workboardRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	// The listing's page size: a count that is not a count is no count the
	// caller asked for, so the family's default stands — and a fraction
	// truncates to no count at all.
	for _, tc := range []struct {
		raw   string
		limit int
	}{
		{`{}`, 100},
		{`{"limit":0}`, 100},
		{`{"limit":-1}`, 100},
		{`{"limit":0.5}`, 100},
		{`{"limit":1.9}`, 1},
		{`{"limit":9999}`, 9999},
	} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_list", json.RawMessage(tc.raw), nil); err != nil {
			t.Fatalf("list %s: %v", tc.raw, err)
		}
		if reader.filter.Limit != tc.limit {
			t.Fatalf("list %s limit=%d, want %d", tc.raw, reader.filter.Limit, tc.limit)
		}
	}
	// The lanes read more rows than the listing does — the family's own page.
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_lanes", json.RawMessage(`{}`), nil); err != nil {
		t.Fatal(err)
	}
	if reader.filter.Limit != 500 {
		t.Fatalf("lanes limit=%d, want 500", reader.filter.Limit)
	}
	// The sweep is capped: a caller cannot ask the board to sweep more than
	// the family sweeps, and a stale claim is the family's own age.
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_sweep", json.RawMessage(`{"limit":9999}`), nil); err != nil {
		t.Fatal(err)
	}
	if relations.limit != 1000 {
		t.Fatalf("sweep limit=%d, want 1000", relations.limit)
	}
	if relations.stale != 10*60*1000*time.Millisecond {
		t.Fatalf("sweep stale=%v, want the family's ten minutes", relations.stale)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_reclaim", json.RawMessage(`{"id":"owned"}`), nil); err != nil {
		t.Fatal(err)
	}
	if relations.stale != 10*60*1000*time.Millisecond {
		t.Fatalf("reclaim stale=%v, want the family's ten minutes", relations.stale)
	}
	// The retry policy: a policy that asks for no attempts is not a policy,
	// and a policy that asks for zero attempts is refused — the two answers
	// stay apart.
	for _, raw := range []string{`{"id":"owned"}`, `{"id":"owned","max_attempts":0}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_policy", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("policy %s was accepted", raw)
		}
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_policy", json.RawMessage(`{"id":"owned","max_attempts":3}`), nil); err != nil {
		t.Fatal(err)
	}
	if relations.policy == nil || relations.policy.MaxAttempts != 3 {
		t.Fatalf("policy=%+v", relations.policy)
	}
	// A mistyped field is a usage error the door names — not a value the
	// family reads as something else.
	audited := audit.calls
	for _, raw := range []string{`{"limit":"10"}`, `{"limit":true}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_list", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("a mistyped count was accepted %s", raw)
		} else if !strings.Contains(err.Error(), "has wrong type") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	for _, raw := range []string{`{"title":7}`, `{"tags":7}`, `{"max_attempts":"3"}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_create", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("a mistyped body was accepted %s", raw)
		} else if !strings.Contains(err.Error(), "has wrong type") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if audit.calls != audited {
		t.Fatalf("a refused body was audited %d times", audit.calls-audited)
	}
}

// TestWorkboardRowsCarryTheFamiliesOwnVocabulary says the tag lists are read
// as lists of texts — or as the one comma-separated string a caller sends —
// and that the family's own texts are trimmed.
func TestWorkboardRowsCarryTheFamiliesOwnVocabulary(t *testing.T) {
	reader := &workReader{}
	lifecycle := &lifecycleHost{}
	services := appwork.NativeServices{
		Reads:     appwork.New(reader),
		Lifecycle: appwork.NewLifecycle(lifecycle, lifecycle),
		ValidSeat: func(string) bool { return true },
	}
	ops, err := appwork.Operations(func(context.Context) appwork.NativeServices { return services })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: workboardAuth{}, Router: workboardRouter{}, Audit: &workboardAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	// A list is trimmed, and the members that say nothing are dropped — the
	// family filters what it cannot use rather than refusing the body.
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_create", json.RawMessage(`{"title":" t ","tags":[" a ","","b"],"artifacts":[" c "]}`), nil); err != nil {
		t.Fatal(err)
	}
	if len(lifecycle.spec.Tags) != 2 || lifecycle.spec.Tags[0] != "a" || lifecycle.spec.Tags[1] != "b" {
		t.Fatalf("tags=%v", lifecycle.spec.Tags)
	}
	if len(lifecycle.spec.Artifacts) != 1 || lifecycle.spec.Artifacts[0] != "c" {
		t.Fatalf("artifacts=%v", lifecycle.spec.Artifacts)
	}
	// A member that is not a text is a member the family cannot use, not a
	// body it refuses.
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_create", json.RawMessage(`{"title":"t","tags":["a",7]}`), nil); err != nil {
		t.Fatal(err)
	}
	if len(lifecycle.spec.Tags) != 1 || lifecycle.spec.Tags[0] != "a" {
		t.Fatalf("tags=%v", lifecycle.spec.Tags)
	}
	if lifecycle.spec.Title != "t" {
		t.Fatalf("title=%q", lifecycle.spec.Title)
	}
	// The ONE comma-separated string is split as it came — the family's own
	// reading of a list sent as a string.
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_create", json.RawMessage(`{"title":"t","criteria":"one, two"}`), nil); err != nil {
		t.Fatal(err)
	}
	if len(lifecycle.spec.AcceptanceCriteria) != 2 || lifecycle.spec.AcceptanceCriteria[0] != "one" || lifecycle.spec.AcceptanceCriteria[1] != " two" {
		t.Fatalf("criteria=%v", lifecycle.spec.AcceptanceCriteria)
	}
	// A policy asked for without attempts is no policy at all; an escalation
	// on its own is a policy with no attempts.
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_create", json.RawMessage(`{"title":"t"}`), nil); err != nil {
		t.Fatal(err)
	}
	if lifecycle.spec.RetryPolicy != nil {
		t.Fatalf("policy=%+v, want none", lifecycle.spec.RetryPolicy)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "workboard_create", json.RawMessage(`{"title":"t","escalate_to":" lead "}`), nil); err != nil {
		t.Fatal(err)
	}
	if lifecycle.spec.RetryPolicy == nil || lifecycle.spec.RetryPolicy.EscalateTo != "lead" || lifecycle.spec.RetryPolicy.MaxAttempts != 0 {
		t.Fatalf("policy=%+v", lifecycle.spec.RetryPolicy)
	}
}
