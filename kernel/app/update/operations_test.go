// SPDX-License-Identifier: MIT
package update

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/update"
)

type updateAuth struct{ principal opapi.Principal }

func (a updateAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.principal, nil
}

type updateRoute struct{}
type routeKey struct{}

func (updateRoute) Route(ctx context.Context, _ opapi.Principal, s opapi.Spec) (context.Context, error) {
	if s.Tenancy != opapi.Primary {
		return nil, errors.New("not primary")
	}
	return context.WithValue(ctx, routeKey{}, "selected"), nil
}

type updateAudit struct {
	begin, end int
	cause      error
	endCause   error
}

func (a *updateAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.begin++
	if a.cause != nil {
		return nil, a.cause
	}
	return a, nil
}
func (a *updateAudit) End(context.Context, error) error { a.end++; return a.endCause }

func TestUpdateOperationCompletionAuditFailureRetainsAppliedEffects(t *testing.T) {
	var call context.Context
	var events []string
	backend := fixtureBackend{apply: func(ctx context.Context, _ *core.UpdateInfo, _ func(context.Context, time.Duration) core.DrainResult) error {
		call = ctx
		events = append(events, "apply")
		return nil
	}}
	ops, _ := Operations(func(context.Context) *Service {
		return New(backend, "c", nil, func() { events = append(events, "sentinel") }, func(time.Duration) { events = append(events, "restart") })
	})
	cause := errors.New("owned terminal audit failure")
	audit := &updateAudit{endCause: cause}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: updateAuth{opapi.Principal{Kind: opapi.Operator}}, Router: updateRoute{}, Audit: audit})
	p := &ownershipProbe{accept: true}
	ctx := opapi.WithTerminalWrite(opapi.WithTerminalCleanup(context.Background(), p), p)
	out, err := d.Dispatch(ctx, opapi.Caller{}, "update_apply", json.RawMessage(`{"version":"v","sha256":"s","url":"u"}`), nil)
	if out == nil || !out.(ApplyOutput).Applied || !errors.Is(err, cause) || !reflect.DeepEqual(events, []string{"apply", "sentinel"}) || call.Err() != nil || len(p.write) != 1 || len(p.cleanup) != 1 || audit.begin != 1 || audit.end != 1 {
		t.Fatal(out, err, events)
	}
	p.write[0]()
	p.cleanup[0]()
	if !reflect.DeepEqual(events, []string{"apply", "sentinel", "restart"}) || call.Err() != context.Canceled {
		t.Fatal("effects rolled back or lost restart", events)
	}
}

type ownershipProbe struct {
	accept         bool
	cleanup, write []func()
}

func (p *ownershipProbe) Defer(f func()) bool {
	if !p.accept {
		return false
	}
	p.cleanup = append(p.cleanup, f)
	return true
}
func (p *ownershipProbe) After(f func()) bool {
	if !p.accept {
		return false
	}
	p.write = append(p.write, f)
	return true
}

func TestUpdateOperationsSchemasAndPrimaryMetadata(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := Operations(func(context.Context) *Service { return New(nil, "current", nil, nil, nil) })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	for i, op := range ops {
		s := op.Spec()
		input, output := reflect.TypeFor[CheckRequest](), reflect.TypeFor[CheckOutput]()
		if i == 1 {
			input, output = reflect.TypeFor[ApplyRequest](), reflect.TypeFor[ApplyOutput]()
		}
		if s.Input != input || s.Output != output || s.ReadOnly != (i == 0) || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || s.Emission != nil || len(s.EmissionSchema) != 0 || !s.AllowUnknownInput || s.HTTP.Method != "" || s.HTTP.Path != "" {
			t.Fatal(s)
		}
		valid := []string{`{"current":"c","update":null,"up_to_date":true}`, `{"current":"c","update":{"version":"v","sha256":"s","url":"u","notes":""},"up_to_date":false}`, `{"current":"c","update":null,"up_to_date":true,"status":""}`}
		bad := []string{`{"current":"c","up_to_date":true}`, `{"current":"c","update":{"version":"v","sha256":"s","url":"u"},"up_to_date":false}`, `{"current":"c","update":null,"up_to_date":"wrong"}`}
		if i == 1 {
			valid = []string{`{"applied":false,"error":""}`, `{"applied":true,"version":" "}`}
			bad = []string{`{"applied":"true"}`, `{"error":"e"}`, `{"applied":false,"error":7}`}
		}
		for _, raw := range valid {
			if err := schema.ValidateJSON(s.OutputSchema, json.RawMessage(raw)); err != nil {
				t.Fatal(raw, err)
			}
		}
		for _, raw := range bad {
			if schema.ValidateJSON(s.OutputSchema, json.RawMessage(raw)) == nil {
				t.Fatal("untyped/missing field", raw)
			}
		}
	}
}

func TestUpdateOperationsTerminalOwnershipAndCauses(t *testing.T) {
	for _, name := range []string{"update_check", "update_apply"} {
		for _, mode := range []string{"accepted", "rejected", "absent", "error", "panic", "nil-result"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				p := &ownershipProbe{accept: mode != "rejected"}
				var call context.Context
				var events []string
				cause := errors.New("owned backend cause")
				backend := fixtureBackend{check: func(ctx context.Context) (*core.CheckResult, error) {
					call = ctx
					if mode == "panic" {
						panic("private backend panic")
					}
					if mode == "nil-result" {
						return nil, nil
					}
					if mode == "error" {
						return nil, cause
					}
					return &core.CheckResult{Current: "current"}, nil
				}, apply: func(ctx context.Context, info *core.UpdateInfo, _ func(context.Context, time.Duration) core.DrainResult) error {
					call = ctx
					if info.Version != " raw " || info.Provenance != core.ProvenanceUnverified || info.Signature != "" {
						t.Fatal(info)
					}
					if mode == "panic" {
						panic("private backend panic")
					}
					if mode == "error" {
						return cause
					}
					return nil
				}}
				ops, err := Operations(func(ctx context.Context) *Service {
					if ctx.Value(routeKey{}) != "selected" {
						t.Fatal("lost route")
					}
					return New(backend, "current", nil, func() { events = append(events, "sentinel") }, func(delay time.Duration) {
						if delay != 100*time.Millisecond {
							t.Fatal(delay)
						}
						events = append(events, "restart")
					})
				})
				if err != nil {
					t.Fatal(err)
				}
				audit := &updateAudit{}
				d, err := app.NewDispatcher(ops, app.Dependencies{Auth: updateAuth{opapi.Principal{Kind: opapi.Operator}}, Router: updateRoute{}, Audit: audit})
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if mode != "absent" {
					ctx = opapi.WithTerminalWrite(opapi.WithTerminalCleanup(ctx, p), p)
				}
				out, err := d.Dispatch(ctx, opapi.Caller{}, name, json.RawMessage(`{"version":" raw ","sha256":"s","url":"u","unknown":true}`), nil)
				panics := mode == "panic" || name == "update_check" && mode == "nil-result"
				if panics {
					if err == nil || err.Error() != "internal error" || out != nil || call.Err() != context.Canceled || len(p.cleanup) != 0 || len(p.write) != 0 {
						t.Fatal("panic opacity/cleanup", out, err, call.Err())
					}
					return
				}
				if name == "update_check" && mode == "error" {
					if !errors.Is(err, cause) || out != nil {
						t.Fatal(out, err)
					}
				} else if err != nil {
					t.Fatal(err)
				}
				if call == nil {
					t.Fatal("backend not entered")
				}
				if mode == "absent" || mode == "rejected" {
					if call.Err() != context.Canceled {
						t.Fatal("fallback leaked context")
					}
				} else {
					if call.Err() != nil || len(p.cleanup) != 1 {
						t.Fatal("early cancel/missing cleanup")
					}
				}
				if name == "update_apply" && mode != "error" {
					if mode == "absent" || mode == "rejected" {
						if !reflect.DeepEqual(events, []string{"sentinel", "restart"}) {
							t.Fatal(events)
						}
					} else {
						if !reflect.DeepEqual(events, []string{"sentinel"}) || len(p.write) != 1 {
							t.Fatal("early/missing restart", events, p.write)
						}
						p.write[0]()
						if !reflect.DeepEqual(events, []string{"sentinel", "restart"}) {
							t.Fatal(events)
						}
					}
				} else if len(events) != 0 || len(p.write) != 0 {
					t.Fatal("error/check scheduled restart", events)
				}
				for _, f := range p.cleanup {
					f()
				}
				if call.Err() != context.Canceled {
					t.Fatal("cleanup not transferred")
				}
				if name == "update_check" {
					if audit.begin != 0 || audit.end != 0 {
						t.Fatal("read audited")
					}
				} else if audit.begin != 1 || audit.end != 1 {
					t.Fatal("writer audit", audit)
				}
			})
		}
	}
}

func TestUpdateOperationsAdmissionAndCodecPriority(t *testing.T) {
	providers, backendCalls := 0, 0
	service := New(fixtureBackend{check: func(context.Context) (*core.CheckResult, error) { backendCalls++; return &core.CheckResult{}, nil }, apply: func(context.Context, *core.UpdateInfo, func(context.Context, time.Duration) core.DrainResult) error {
		backendCalls++
		return nil
	}}, "c", nil, func() {}, func(time.Duration) {})
	ops, _ := Operations(func(context.Context) *Service { providers++; return service })
	for _, name := range []string{"update_check", "update_apply"} {
		for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}, {Kind: opapi.System}} {
			d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: updateAuth{principal}, Router: updateRoute{}, Audit: &updateAudit{}})
			if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{}`), nil); err == nil || providers != 0 || backendCalls != 0 {
				t.Fatal("non-primary effect", name, err)
			}
		}
	}
	audit := &updateAudit{cause: errors.New("owned admission failure")}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: updateAuth{opapi.Principal{Kind: opapi.Operator}}, Router: updateRoute{}, Audit: audit})
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "update_apply", json.RawMessage(`{"version":"v","sha256":"s","url":"u"}`), nil); !errors.Is(err, audit.cause) || providers != 0 || backendCalls != 0 {
		t.Fatal("audit effect", err)
	}
	audit.cause = nil
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, name := range []string{"update_check", "update_apply"} {
		if _, err := d.Dispatch(ctx, opapi.Caller{}, name, json.RawMessage(`{}`), nil); !errors.Is(err, context.Canceled) || providers != 0 || backendCalls != 0 {
			t.Fatal("canceled effect", name, err)
		}
	}
	for _, tc := range []struct{ raw, key string }{{`{"version":null}`, "version"}, {`{"version":false,"sha256":false}`, "version"}, {`{"version":"v","sha256":3}`, "sha256"}, {`{"version":"v","sha256":"s","url":[]}`, "url"}, {`{"version":"v","sha256":"s","url":"u","notes":null}`, "notes"}} {
		raw := tc.raw
		before := backendCalls
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, "update_apply", json.RawMessage(raw), nil)
		if err == nil || err.Error() != "args."+tc.key+" must be a string" || backendCalls != before {
			t.Fatal(raw, err)
		}
	}
	service.backend = nil
	_, err := d.Dispatch(context.Background(), opapi.Caller{}, "update_apply", json.RawMessage(`{"version":null}`), nil)
	if err == nil || err.Error() != "update is disabled" {
		t.Fatal("disabled priority", err)
	}
}

func BenchmarkUpdateDispatch(b *testing.B) {
	backend := fixtureBackend{check: func(context.Context) (*core.CheckResult, error) {
		return &core.CheckResult{Current: "owned", Update: &core.UpdateInfo{Version: "v", SHA256: "s", URL: "u", Notes: ""}}, nil
	}, apply: func(context.Context, *core.UpdateInfo, func(context.Context, time.Duration) core.DrainResult) error {
		return nil
	}}
	ops, err := Operations(func(context.Context) *Service { return New(backend, "c", nil, func() {}, func(time.Duration) {}) })
	if err != nil {
		b.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: updateAuth{opapi.Principal{Kind: opapi.Operator}}, Router: updateRoute{}, Audit: &updateAudit{}})
	if err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"update_check", "update_apply"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"version":"v","sha256":"s","url":"u"}`), nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
