// SPDX-License-Identifier: MIT

package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

type authFunc func(context.Context, opapi.Caller) (opapi.Principal, error)

func (f authFunc) Authenticate(ctx context.Context, c opapi.Caller) (opapi.Principal, error) {
	return f(ctx, c)
}

type routeFunc func(context.Context, opapi.Principal, opapi.Spec) (context.Context, error)

func (f routeFunc) Route(ctx context.Context, p opapi.Principal, s opapi.Spec) (context.Context, error) {
	return f(ctx, p, s)
}

type auditFunc func(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error)

func (f auditFunc) Begin(ctx context.Context, r opapi.AuditRecord) (opapi.AuditSpan, error) {
	return f(ctx, r)
}

type spanFunc func(context.Context, error) error

func (f spanFunc) End(ctx context.Context, err error) error { return f(ctx, err) }

type emitFunc func(context.Context, any) error

func (f emitFunc) Emit(ctx context.Context, v any) error { return f(ctx, v) }

type routedKey struct{}
type input struct {
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
}
type output struct {
	Value string `json:"value"`
}

func TestDispatchOrdersAdmissionBeforeEffects(t *testing.T) {
	for _, mode := range []string{"allow", "auth-error", "forbidden", "route-error", "bad-input", "missing-required", "audit-error", "no-audit", "cancel-at-audit", "handler-error", "panic", "end-error"} {
		t.Run(mode, func(t *testing.T) {
			var trace []string
			cause := errors.New("typed failure")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			op, err := app.NewOperation(opapi.Spec{Name: "mutate"}, func(ctx context.Context, in input) (output, error) {
				trace = append(trace, "effect")
				if ctx.Value(routedKey{}) != "primary" {
					t.Error("routing context lost")
				}
				if mode == "panic" {
					panic("handler fault")
				}
				if mode == "handler-error" {
					return output{}, cause
				}
				return output{in.Name}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			deps := app.Dependencies{
				Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
					trace = append(trace, "auth")
					if mode == "auth-error" {
						return opapi.Principal{}, cause
					}
					if mode == "forbidden" {
						return opapi.Principal{Kind: opapi.Tenant, Tenant: "a"}, nil
					}
					return opapi.Principal{Kind: opapi.Operator}, nil
				}),
				Router: routeFunc(func(ctx context.Context, p opapi.Principal, s opapi.Spec) (context.Context, error) {
					trace = append(trace, "route")
					if mode == "route-error" {
						return nil, cause
					}
					return context.WithValue(ctx, routedKey{}, "primary"), nil
				}),
				Audit: auditFunc(func(ctx context.Context, record opapi.AuditRecord) (opapi.AuditSpan, error) {
					trace = append(trace, "begin")
					if record.Operation != "mutate" || record.Principal.Kind != opapi.Operator {
						t.Errorf("audit=%+v", record)
					}
					if mode == "audit-error" {
						return nil, cause
					}
					if mode == "cancel-at-audit" {
						cancel()
					}
					return spanFunc(func(_ context.Context, outcome error) error {
						trace = append(trace, "end")
						if mode == "handler-error" && !errors.Is(outcome, cause) {
							t.Error("terminal cause lost")
						}
						if mode == "panic" && (outcome == nil || !strings.Contains(outcome.Error(), "panicked")) {
							t.Error("panic missing from outcome")
						}
						if mode == "end-error" {
							return cause
						}
						return nil
					}), nil
				}),
			}
			if mode == "no-audit" {
				deps.Audit = nil
			}
			d, err := app.NewDispatcher([]app.Operation{op}, deps)
			if err != nil {
				t.Fatal(err)
			}
			raw := json.RawMessage(`{"name":"value"}`)
			if mode == "missing-required" {
				raw = json.RawMessage(`{}`)
			}
			if mode == "bad-input" {
				raw = json.RawMessage(`{"extra":"bad"}`)
			}
			result, err := d.Dispatch(ctx, opapi.Caller{Credential: "opaque", Tenant: "a"}, "mutate", raw, nil)
			want := []string{"auth", "route", "begin", "effect", "end"}
			switch mode {
			case "auth-error", "forbidden":
				want = []string{"auth"}
			case "route-error", "bad-input", "missing-required", "no-audit":
				want = []string{"auth", "route"}
			case "audit-error":
				want = []string{"auth", "route", "begin"}
			case "cancel-at-audit":
				want = []string{"auth", "route", "begin", "end"}
			}
			if !reflect.DeepEqual(trace, want) {
				t.Fatalf("trace=%v want=%v", trace, want)
			}
			if mode == "allow" {
				if err != nil || result.(output).Value != "value" {
					t.Fatalf("result=%v error=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("failure accepted")
			}
			if mode == "auth-error" || mode == "route-error" || mode == "audit-error" || mode == "handler-error" || mode == "end-error" {
				if !errors.Is(err, cause) {
					t.Fatalf("typed cause lost: %v", err)
				}
			}
		})
	}
}

func TestReadOnlyTenantAndStreamingContracts(t *testing.T) {
	var calls int
	op, err := app.NewStreamingOperation(opapi.Spec{Name: "stream", ReadOnly: true, Authz: opapi.OwnTenant, Tenancy: opapi.CallerTenant, Stream: opapi.StreamEvents}, func(ctx context.Context, in input, emit func(output) error) (output, error) {
		calls++
		if ctx.Value(routedKey{}) != "tenant-a" {
			t.Error("wrong tenant host")
		}
		value := output{in.Name}
		return value, emit(value)
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := app.Dependencies{Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
		return opapi.Principal{Kind: opapi.Tenant, Tenant: "a"}, nil
	}), Router: routeFunc(func(ctx context.Context, p opapi.Principal, s opapi.Spec) (context.Context, error) {
		if p.Tenant != "a" || s.Tenancy != opapi.CallerTenant {
			t.Error("routing metadata lost")
		}
		return context.WithValue(ctx, routedKey{}, "tenant-a"), nil
	})}
	d, err := app.NewDispatcher([]app.Operation{op}, deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"missing"} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "a"}, name, nil, nil); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatalf("unknown tenant op=%v", err)
		}
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "b"}, "stream", json.RawMessage(`{"name":"x"}`), nil); err == nil {
		t.Fatal("foreign tenant accepted")
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "a"}, "stream", json.RawMessage(`{"name":"x"}`), nil); err == nil || calls != 0 {
		t.Fatal("missing emitter admitted effects")
	}
	cause := errors.New("emitter failure")
	_, err = d.Dispatch(context.Background(), opapi.Caller{Tenant: "a"}, "stream", json.RawMessage(`{"name":"x"}`), emitFunc(func(_ context.Context, v any) error {
		if v.(output).Value != "x" {
			t.Error("typed emitted value changed")
		}
		return cause
	}))
	if !errors.Is(err, cause) || calls != 1 {
		t.Fatalf("stream error=%v calls=%d", err, calls)
	}
	specs := d.Specs()
	specs[0].InputSchema[0] = '!'
	if !json.Valid(d.Specs()[0].InputSchema) {
		t.Fatal("caller changed registry schema")
	}
	if _, err := app.NewDispatcher([]app.Operation{op, op}, deps); err == nil {
		t.Fatal("duplicate operation accepted")
	}
	if _, err := app.NewOperation(opapi.Spec{Name: "unsafe", Authz: opapi.OwnTenant}, func(context.Context, input) (output, error) { return output{}, nil }); err == nil {
		t.Fatal("unrouted tenant operation registered")
	}
}

func BenchmarkDispatchWithoutAuditIO(b *testing.B) {
	op, err := app.NewOperation(opapi.Spec{Name: "read", ReadOnly: true}, func(context.Context, input) (output, error) { return output{"ok"}, nil })
	if err != nil {
		b.Fatal(err)
	}
	d, err := app.NewDispatcher([]app.Operation{op}, app.Dependencies{Auth: authFunc(func(context.Context, opapi.Caller) (opapi.Principal, error) {
		return opapi.Principal{Kind: opapi.Operator}, nil
	}), Router: routeFunc(func(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) { return ctx, nil })})
	if err != nil {
		b.Fatal(err)
	}
	raw := json.RawMessage(`{"name":"value"}`)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.Dispatch(ctx, opapi.Caller{}, "read", raw, nil); err != nil {
			b.Fatal(err)
		}
	}
}
