// SPDX-License-Identifier: MIT

package steer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/intervention"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fakeRuns struct {
	calls  []string
	known  map[string]bool
	result intervention.Result
	err    error
	got    intervention.Request
}

func (f *fakeRuns) act(name, corr string) bool {
	f.calls = append(f.calls, name+":"+corr)
	return f.known[corr]
}
func (f *fakeRuns) PauseRun(corr string) bool  { return f.act("pause", corr) }
func (f *fakeRuns) ResumeRun(corr string) bool { return f.act("resume", corr) }
func (f *fakeRuns) StepRun(corr string) bool   { return f.act("step", corr) }
func (f *fakeRuns) SteerRun(corr, directive string, note bool) bool {
	f.calls = append(f.calls, fmt.Sprintf("steer:%s:%s:%v", corr, directive, note))
	return f.known[corr]
}
func (f *fakeRuns) InterveneRun(req intervention.Request) (intervention.Result, error) {
	f.calls = append(f.calls, "intervene")
	f.got = req
	return f.result, f.err
}

func decode[T any](t *testing.T, raw string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(raw, err)
	}
	return v
}

func TestSteerControlCodecs(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{`{}`: "args.correlation required", `{"correlation":"  "}`: "args.correlation required", `{"correlation":3}`: "args.correlation must be a string", `{"correlation":null}`: "args.correlation must be a string"} {
		runs := &fakeRuns{}
		s := New(runs)
		for _, call := range []func(RunRequest) error{
			func(in RunRequest) error { _, err := s.Pause(ctx, in); return err },
			func(in RunRequest) error { _, err := s.Resume(ctx, in); return err },
			func(in RunRequest) error { _, err := s.Step(ctx, in); return err },
		} {
			if err := call(decode[RunRequest](t, raw)); err == nil || err.Error() != want {
				t.Fatal(raw, err)
			}
		}
		if runs.calls != nil {
			t.Fatal("effect on a rejected request", runs.calls)
		}
	}
	runs := &fakeRuns{known: map[string]bool{" r1 ": true}}
	s := New(runs)
	// The correlation passes through untrimmed, exactly as given.
	if out, err := s.Pause(ctx, decode[RunRequest](t, `{"correlation":" r1 "}`)); err != nil || out != (ControlOutput{Correlation: " r1 ", OK: true}) {
		t.Fatal(out, err)
	}
	if out, _ := s.Resume(ctx, decode[RunRequest](t, `{"correlation":"r2"}`)); out != (ControlOutput{Correlation: "r2"}) {
		t.Fatal(out)
	}
	if out, _ := s.Step(ctx, decode[RunRequest](t, `{"correlation":" r1 "}`)); !out.OK {
		t.Fatal(out)
	}
	if strings.Join(runs.calls, ",") != "pause: r1 ,resume:r2,step: r1 " {
		t.Fatal(runs.calls)
	}
}

func TestSteerDirective(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{
		`{"directive":"x"}`:                            "args.correlation required",
		`{"correlation":"r","mode":3}`:                 "args.directive required",
		`{"correlation":"r","directive":" "}`:          "args.directive required",
		`{"correlation":"r","directive":5}`:            "args.directive must be a string",
		`{"correlation":"r","directive":"d","mode":3}`: "args.mode must be a string",
	} {
		runs := &fakeRuns{}
		if _, err := New(runs).Steer(ctx, decode[SteerRequest](t, raw)); err == nil || err.Error() != want || runs.calls != nil {
			t.Fatal(raw, err)
		}
	}
	for raw, want := range map[string]SteerOutput{
		`{"correlation":"r","directive":" go "}`:              {Correlation: "r", Mode: "steer", Accepted: true},
		`{"correlation":"r","directive":"go","mode":"note"}`:  {Correlation: "r", Mode: "note", Accepted: true},
		`{"correlation":"r","directive":"go","mode":" note"}`: {Correlation: "r", Mode: "steer", Accepted: true},
		`{"correlation":"x","directive":"go","mode":"steer"}`: {Correlation: "x", Mode: "steer"},
		`{"correlation":"r","directive":"go","mode":"NOTE"}`:  {Correlation: "r", Mode: "steer", Accepted: true},
	} {
		runs := &fakeRuns{known: map[string]bool{"r": true}}
		out, err := New(runs).Steer(ctx, decode[SteerRequest](t, raw))
		if err != nil || out != want {
			t.Fatal(raw, out, err)
		}
	}
	runs := &fakeRuns{}
	New(runs).Steer(ctx, decode[SteerRequest](t, `{"correlation":"r","directive":" go ","mode":"note"}`))
	if runs.calls[0] != "steer:r: go :true" {
		t.Fatal("directive passes untrimmed", runs.calls)
	}
}

func TestSteerIntervene(t *testing.T) {
	ctx := context.Background()
	for raw, want := range map[string]string{
		`{"primitive":1}`: "args.correlation required",
		`{"correlation":"r","primitive":1,"directive":2}`:        "args.primitive must be a string",
		`{"correlation":"r","directive":2,"scope":3}`:            "args.directive must be a string",
		`{"correlation":"r","scope":3,"idempotency_key":4}`:      "args.scope must be a string",
		`{"correlation":"r","idempotency_key":4,"lease_ms":"x"}`: "args.idempotency_key must be a string",
		`{"correlation":"r","lease_ms":"5"}`:                     "args.lease_ms must be a number",
		`{"correlation":"r","lease_ms":null}`:                    "args.lease_ms must be a number",
	} {
		runs := &fakeRuns{}
		if _, err := New(runs).Intervene(ctx, decode[InterveneRequest](t, raw)); err == nil || err.Error() != want || runs.calls != nil {
			t.Fatal(raw, err)
		}
	}
	for raw, lease := range map[string]time.Duration{`{"correlation":"r"}`: 0, `{"correlation":"r","lease_ms":0}`: 0, `{"correlation":"r","lease_ms":-5}`: 0, `{"correlation":"r","lease_ms":1500}`: 1500 * time.Millisecond, `{"correlation":"r","lease_ms":1.9}`: time.Millisecond} {
		runs := &fakeRuns{}
		New(runs).Intervene(ctx, decode[InterveneRequest](t, raw))
		if runs.got.Lease != lease {
			t.Fatal(raw, runs.got.Lease)
		}
	}
	expires := time.Unix(1700000000, 999)
	runs := &fakeRuns{result: intervention.Result{Primitive: "pause", CorrelationID: "r2", Accepted: true, Applied: true, State: "paused", Paused: true, Pending: 2, IdempotencyKey: "k2", Reason: "why", LeaseExpires: expires}}
	out, err := New(runs).Intervene(ctx, decode[InterveneRequest](t, `{"correlation":" r ","primitive":" pause ","directive":"d","scope":"s","idempotency_key":"k"}`))
	if err != nil || runs.got != (intervention.Request{Primitive: " pause ", CorrelationID: " r ", Directive: "d", Scope: "s", IdempotencyKey: "k"}) {
		t.Fatal("request passes through for the kernel to normalize", runs.got, err)
	}
	unix := expires.Unix()
	if !reflect.DeepEqual(out, InterveneOutput{Primitive: "pause", Correlation: "r2", Accepted: true, Applied: true, State: "paused", Paused: true, Pending: 2, IdempotencyKey: "k2", Reason: "why", LeaseExpiresUnix: &unix}) {
		t.Fatal(out)
	}
	runs = &fakeRuns{result: intervention.Result{Primitive: "abort", State: "unknown"}}
	out, _ = New(runs).Intervene(ctx, decode[InterveneRequest](t, `{"correlation":"r"}`))
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "lease_expires_unix") || out.LeaseExpiresUnix != nil {
		t.Fatal("no lease, no expiry", string(raw))
	}
	runs = &fakeRuns{err: errors.New("unknown primitive")}
	if _, err := New(runs).Intervene(ctx, decode[InterveneRequest](t, `{"correlation":"r"}`)); err == nil || err.Error() != "unknown primitive" {
		t.Fatal(err)
	}
}

type steerAuth struct{ principal opapi.Principal }

func (a steerAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.principal, nil
}

type steerRoute struct{ routed *[]string }

func (r steerRoute) Route(ctx context.Context, p opapi.Principal, spec opapi.Spec) (context.Context, error) {
	*r.routed = append(*r.routed, spec.Name+"@"+p.Tenant)
	return ctx, nil
}

type steerAudit struct {
	begins []opapi.AuditRecord
	fail   error
}

func (a *steerAudit) Begin(_ context.Context, record opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begins = append(a.begins, record)
	if a.fail != nil {
		return nil, a.fail
	}
	return steerSpan{}, nil
}

type steerSpan struct{}

func (steerSpan) End(context.Context, error) error { return nil }

func TestSteerOperations(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	runs := &fakeRuns{known: map[string]bool{"r": true}}
	ops, err := Operations(func(context.Context) *Service { return New(runs) })
	if err != nil || len(ops) != 5 {
		t.Fatal(ops, err)
	}
	want := map[string]struct {
		path string
		in   reflect.Type
		out  reflect.Type
	}{
		"run_pause":     {"/api/run/pause", reflect.TypeFor[RunRequest](), reflect.TypeFor[ControlOutput]()},
		"run_resume":    {"/api/run/resume", reflect.TypeFor[RunRequest](), reflect.TypeFor[ControlOutput]()},
		"run_step":      {"/api/run/step", reflect.TypeFor[RunRequest](), reflect.TypeFor[ControlOutput]()},
		"run_steer":     {"/api/run/steer", reflect.TypeFor[SteerRequest](), reflect.TypeFor[SteerOutput]()},
		"run_intervene": {"", reflect.TypeFor[InterveneRequest](), reflect.TypeFor[InterveneOutput]()},
	}
	for _, op := range ops {
		spec := op.Spec()
		w, ok := want[spec.Name]
		http := opapi.HTTP{}
		if w.path != "" {
			http = opapi.HTTP{Method: "POST", Path: w.path}
		}
		if !ok || spec.ReadOnly || spec.Authz != opapi.OwnTenant || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.Input != w.in || spec.Output != w.out || spec.HTTP != http {
			t.Fatal(spec)
		}
		delete(want, spec.Name)
	}
	if schema.ValidateJSON(ops[4].Spec().OutputSchema, json.RawMessage(`{"primitive":"p","correlation":"c","accepted":true,"applied":false,"state":"s","paused":false,"pending":0,"idempotency_key":"","reason":""}`)) != nil {
		t.Fatal("intervene schema without lease")
	}
	var routed []string
	audit := &steerAudit{fail: errors.New("audit unavailable")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: steerAuth{opapi.Principal{Kind: opapi.Tenant, Tenant: "acme"}}, Router: steerRoute{&routed}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "run_pause", json.RawMessage(`{"correlation":"r"}`), nil); err == nil || err.Error() != "audit unavailable" || runs.calls != nil {
		t.Fatal("failed audit admission must block the steer", err, runs.calls)
	}
	audit = &steerAudit{}
	d, _ = app.NewDispatcher(ops, app.Dependencies{Auth: steerAuth{opapi.Principal{Kind: opapi.Tenant, Tenant: "acme"}}, Router: steerRoute{&routed}, Audit: audit})
	out, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "run_steer", json.RawMessage(`{"correlation":"r","directive":"go"}`), nil)
	if err != nil || out != (SteerOutput{Correlation: "r", Mode: "steer", Accepted: true}) || len(audit.begins) != 1 || audit.begins[0].Operation != "run_steer" || routed[len(routed)-1] != "run_steer@acme" {
		t.Fatal("a tenant steers its own runs", out, err, routed, audit.begins)
	}
	runs.calls = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{Tenant: "acme"}, "run_step", json.RawMessage(`{"correlation":"r"}`), nil); !errors.Is(err, context.Canceled) || runs.calls != nil || len(audit.begins) != 1 {
		t.Fatal("canceled steer", err, runs.calls)
	}
	denied, _ := app.NewDispatcher(ops, app.Dependencies{Auth: steerAuth{opapi.Principal{Kind: opapi.Agent}}, Router: steerRoute{&routed}, Audit: &steerAudit{}})
	if _, err := denied.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "run_pause", json.RawMessage(`{"correlation":"r"}`), nil); err == nil || runs.calls != nil {
		t.Fatal("agent principal", err)
	}
}
