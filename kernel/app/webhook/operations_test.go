// SPDX-License-Identifier: MIT
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type fixtureAuth struct{}

func (fixtureAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

func TestWebhookOperationRawNumberCompatibility(t *testing.T) {
	r := &fixtureJournal{events: []*event.Event{delivery(event.KindWebhookDelivered, 900, 1, `{"url":"a"}`), delivery(event.KindWebhookFailed, 800, 2, `{"url":"b"}`)}}
	ops, err := Operations(func(context.Context) *Observability { return NewObservability(r, func() int64 { return 1000 }) })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: fixtureAuth{}, Router: fixtureRoute{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw    string
		window int64
		total  int
	}{
		{`{"since_ms":100.9}`, 100, 1}, {`{"since_ms":-7}`, -7, 2}, {`{"since_ms":"100"}`, 0, 2}, {`{"since_ms":null}`, 0, 2}, {`{"since_ms":true}`, 0, 2},
	} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, "webhook_stats", json.RawMessage(tc.raw), nil)
		if err != nil {
			t.Fatal(err)
		}
		stats := out.(StatsOutput)
		if stats.WindowMS != tc.window || stats.Total != tc.total {
			t.Fatal(tc.raw, stats)
		}
	}
	for _, tc := range []struct {
		raw   string
		count int
	}{
		{`{"limit":0}`, 1}, {`{"limit":1.9}`, 1}, {`{"limit":2.9}`, 2}, {`{"limit":"ignored"}`, 2}, {`{"limit":null}`, 2}, {`{"failed":true}`, 1}, {`{"failed":false}`, 2},
	} {
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, "webhook_log", json.RawMessage(tc.raw), nil)
		if err != nil || out.(LogOutput).Count != tc.count {
			t.Fatal(tc.raw, out, err)
		}
	}
}

func BenchmarkWebhookDispatch(b *testing.B) {
	reader := &fixtureJournal{events: []*event.Event{delivery(event.KindWebhookDelivered, 900, 1, `{"url":"owned","status":200}`)}}
	ops, err := Operations(func(context.Context) *Observability { return NewObservability(reader, func() int64 { return 1000 }) })
	if err != nil {
		b.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: fixtureAuth{}, Router: fixtureRoute{}})
	if err != nil {
		b.Fatal(err)
	}
	for _, name := range []string{"webhook_log", "webhook_stats"} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"limit":20,"since_ms":1000}`), nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type fixtureRoute struct{}
type selectedKey struct{}

func (fixtureRoute) Route(ctx context.Context, _ opapi.Principal, s opapi.Spec) (context.Context, error) {
	if s.Tenancy != opapi.CallerTenant {
		return nil, errors.New("lost tenant scope")
	}
	return context.WithValue(ctx, selectedKey{}, "owned"), nil
}

func TestWebhookOperationsTypedMetadataAndAdmission(t *testing.T) {
	if _, err := Operations(nil); err == nil {
		t.Fatal("nil provider")
	}
	r := &fixtureJournal{}
	calls := 0
	ops, err := Operations(func(ctx context.Context) *Observability {
		calls++
		if ctx.Value(selectedKey{}) != "owned" {
			t.Fatal("lost selected host")
		}
		return NewObservability(r, nil)
	})
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: fixtureAuth{}, Router: fixtureRoute{}})
	if err != nil {
		t.Fatal(err)
	}
	for i, op := range ops {
		s := op.Spec()
		input, output := reflect.TypeFor[LogRequest](), reflect.TypeFor[LogOutput]()
		if i == 1 {
			input, output = reflect.TypeFor[StatsRequest](), reflect.TypeFor[StatsOutput]()
		}
		if !s.ReadOnly || s.Authz != opapi.OwnTenant || s.Tenancy != opapi.CallerTenant || s.Input != input || s.Output != output || s.Stream != opapi.StreamNone || s.Emission != nil || !s.AllowUnknownInput {
			t.Fatal(s)
		}
		if i == 0 && (s.HTTP.Method != "GET" || s.HTTP.Path != "/api/webhook_log") {
			t.Fatal(s.HTTP)
		}
		if i == 1 && (s.HTTP.Method != "" || s.HTTP.Path != "") {
			t.Fatal("invented stats route", s.HTTP)
		}
		out, err := d.Dispatch(context.Background(), opapi.Caller{}, s.Name, json.RawMessage(`{"unknown":true}`), nil)
		if err != nil || out == nil {
			t.Fatal("read unexpectedly required audit", out, err)
		}
		raw, _ := json.Marshal(out)
		if err := schema.ValidateJSON(s.OutputSchema, raw); err != nil {
			t.Fatal(err)
		}
		bad := `{"deliveries":[{"attempts":"wrong","ok":true}],"count":0,"next_cursor":""}`
		if i == 1 {
			bad = `{"by_url":{"a":{"delivered":"wrong","failed":0}},"delivered":0,"failed":0,"failure_rate":0,"total":0,"window_ms":0}`
		}
		if schema.ValidateJSON(s.OutputSchema, json.RawMessage(bad)) == nil {
			t.Fatal("untyped nested result")
		}
		beforeCalls, beforeRange := calls, r.calls
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := d.Dispatch(ctx, opapi.Caller{}, s.Name, json.RawMessage(`{}`), nil); !errors.Is(err, context.Canceled) || calls != beforeCalls || r.calls != beforeRange {
			t.Fatal("canceled entered reader", err, calls, r.calls)
		}
	}
	for _, raw := range []string{`{"failed":null}`, `{"failed":"true"}`, `{"failed":1}`, `{"failed":[]}`} {
		before := calls
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, "webhook_log", json.RawMessage(raw), nil)
		if err == nil || err.Error() != "args.failed must be a boolean" || calls != before {
			t.Fatal(raw, err, calls)
		}
	}
	for _, raw := range []string{`{"failed":false,"limit":"ignored","since_ms":null,"cursor":42}`, `{"failed":true,"limit":1.9,"since_ms":-2}`, `{"limit":0,"unknown":{"nested":true}}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "webhook_log", json.RawMessage(raw), nil); err != nil {
			t.Fatal(raw, err)
		}
	}
}
