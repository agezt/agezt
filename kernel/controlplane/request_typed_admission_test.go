// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	appboard "github.com/agezt/agezt/kernel/app/board"
	appcouncil "github.com/agezt/agezt/kernel/app/council"
	appresearch "github.com/agezt/agezt/kernel/app/research"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// The door of the request side: a body whose field is not the type the
// request contract names is refused BEFORE the handler runs — and before the
// audit sees it. A mistyped field is a usage error, not a silent default, so
// the operator learns it from the door instead of from a wrong effect.
//
// The three operations below are the ones whose bodies carried an empty
// property schema (`{}` says "anything") until W5.8 named every field.

type typedAdmissionAuth struct{}

func (typedAdmissionAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return opapi.Principal{Kind: opapi.Operator}, nil
}

type typedAdmissionRouter struct{}

func (typedAdmissionRouter) Route(ctx context.Context, _ opapi.Principal, _ opapi.Spec) (context.Context, error) {
	return ctx, nil
}

type typedAdmissionAudit struct{ calls int }

func (a *typedAdmissionAudit) Begin(context.Context, opapi.AuditRecord) (opapi.AuditSpan, error) {
	a.calls++
	return typedAdmissionSpan{}, nil
}

type typedAdmissionSpan struct{}

func (typedAdmissionSpan) End(context.Context, error) error { return nil }

func TestConsoleRequestDoorsRefuseMistypedBodies(t *testing.T) {
	board, err := appboard.Operations(
		func(context.Context) (*appboard.Service, error) { return nil, nil },
		func(context.Context) (*appboard.Service, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	council, err := appcouncil.AskOperations(func(context.Context) appcouncil.Engines { return appcouncil.Engines{} })
	if err != nil {
		t.Fatal(err)
	}
	membership, err := appcouncil.Operations(func(context.Context) *appcouncil.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	research, err := appresearch.AskOperations(func(context.Context) appresearch.Engines { return appresearch.Engines{} })
	if err != nil {
		t.Fatal(err)
	}
	ops := append(append(append(append([]app.Operation{}, board...), council...), membership...), research...)
	audit := &typedAdmissionAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: typedAdmissionAuth{}, Router: typedAdmissionRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, raw, want string
	}{
		{opapi.BoardSend, `{"text":"owned","from":true}`, "$.from has wrong type: got boolean, want string"},
		{opapi.BoardAck, `{"id":7,"by":"reader"}`, "$.id has wrong type: got number, want string"},
		{opapi.CouncilAsk, `{"question":7}`, "$.question has wrong type: got number, want string"},
		{opapi.CouncilSet, `{"members":[{"model":7}]}`, "has wrong type"},
		{opapi.ResearchAsk, `{"question":"q","verify":"yes"}`, "$.verify has wrong type: got string, want boolean"},
		{opapi.ResearchAsk, `{"question":"q","max_sources":true}`, "$.max_sources has wrong type: got boolean, want number"},
	} {
		before := audit.calls
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.name, json.RawMessage(tc.raw), nil)
		if err == nil {
			t.Fatalf("%s accepted %s", tc.name, tc.raw)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s %s: %v, want %q", tc.name, tc.raw, err, tc.want)
		}
		if audit.calls != before {
			t.Fatalf("%s %s: the refused body was audited", tc.name, tc.raw)
		}
	}
}
