// SPDX-License-Identifier: MIT

package board_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	appboard "github.com/agezt/agezt/kernel/app/board"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"reflect"
	"testing"
)

type boardAuth struct{}

func (boardAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type boardRouter struct{}

func (boardRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type boardAudit struct {
	cause error
	calls int
}

func (a *boardAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return boardSpan{}, a.cause
}

type boardSpan struct{}

func (boardSpan) End(context.Context, error) error { return nil }
func TestBoardSpecsRetainNativePolicyAndActualSchemas(t *testing.T) {
	ops, err := appboard.Operations(func(context.Context) (*appboard.Service, error) { return nil, nil }, func(context.Context) (*appboard.Service, error) { return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"board_read": true, "board_help": true, "board_inbox": true, "board_get": true, "board_replies": true, "board_send": false, "board_ack": false}
	http := map[string]opapi.HTTP{"board_read": {Method: "GET", Path: "/api/board"}, "board_help": {Method: "GET", Path: "/api/board/help"}, "board_send": {Method: "POST", Path: "/api/board/send"}, "board_ack": {Method: "POST", Path: "/api/board/ack"}}
	seen := map[string]bool{}
	if len(ops) != 7 {
		t.Fatalf("operations=%d", len(ops))
	}
	for _, op := range ops {
		spec := op.Spec()
		read, found := want[spec.Name]
		if !found || seen[spec.Name] || spec.ReadOnly != read || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.HTTP != http[spec.Name] {
			t.Fatalf("metadata=%+v", spec)
		}
		seen[spec.Name] = true
		raw, err := json.Marshal(reflect.Zero(spec.Output).Interface())
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("%s output=%v", spec.Name, err)
		}
		if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(`{"id":"fixture","text":"owned","topic":"status","unused":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := appboard.Operations(nil, nil); err == nil {
		t.Fatal("nil providers admitted")
	}
}
func TestBoardMutationAuditPrecedesAllFactoryEffects(t *testing.T) {
	calls := 0
	cause := errors.New("owned audit unavailable")
	ops, err := appboard.Operations(func(context.Context) (*appboard.Service, error) { calls++; return nil, nil }, func(context.Context) (*appboard.Service, error) { calls++; return nil, nil })
	if err != nil {
		t.Fatal(err)
	}
	audit := &boardAudit{cause: cause}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: boardAuth{}, Router: boardRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"board_send", "board_ack"} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, name, json.RawMessage(`{"id":"fixture","by":"reader","text":"owned","topic":"status"}`), nil); !errors.Is(err, cause) {
			t.Fatalf("%s audit cause=%v", name, err)
		}
	}
	if calls != 0 || audit.calls != 2 {
		t.Fatalf("factory=%d audit=%d", calls, audit.calls)
	}
}
func TestBoardTypedAdmissionRetainsLimitsAndLenientText(t *testing.T) {
	store, s := boardFixture(t)
	for i := 0; i < 505; i++ {
		if _, err := store.Post("topic", "writer", "owned", 100); err != nil {
			t.Fatal(err)
		}
	}
	ops, err := appboard.Operations(func(context.Context) (*appboard.Service, error) { return s, nil }, func(context.Context) (*appboard.Service, error) { return s, nil })
	if err != nil {
		t.Fatal(err)
	}
	audit := &boardAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: boardAuth{}, Router: boardRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw   string
		count int
	}{{`{}`, 50}, {`{"limit":-1}`, 50}, {`{"limit":"ignored"}`, 50}, {`{"limit":0.5}`, 505}, {`{"limit":1.9}`, 1}, {`{"limit":9999}`, 500}} {
		value, err := d.Dispatch(context.Background(), opapi.Caller{}, "board_read", json.RawMessage(tc.raw), nil)
		if err != nil || value.(appboard.ReadOutput).Count != tc.count {
			t.Fatalf("read %s out=%+v err=%v", tc.raw, value, err)
		}
	}
	if audit.calls != 0 {
		t.Fatal("read audited")
	}
	value, err := d.Dispatch(context.Background(), opapi.Caller{}, "board_send", json.RawMessage(`{"text":" owned ","topic":" status ","from":true,"correlation_id":" inbound ","unused":true}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	sent := value.(appboard.SendOutput)
	if sent.Sent.Text != "owned" || sent.Sent.Topic != "status" || sent.Sent.From != "" || sent.CorrelationID != "inbound" {
		t.Fatalf("send=%+v", sent)
	}
	for _, raw := range []string{`{"text":"owned","topic":"status","help":null}`, `{"text":"owned","topic":"status","help":"bad"}`} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "board_send", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("invalid help accepted %s", raw)
		}
	}
}
