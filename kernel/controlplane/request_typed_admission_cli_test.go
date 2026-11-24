// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	appboard "github.com/agezt/agezt/kernel/app/board"
	appedict "github.com/agezt/agezt/kernel/app/edict"
	appjournal "github.com/agezt/agezt/kernel/app/journal"
	appokr "github.com/agezt/agezt/kernel/app/okr"
	appproviders "github.com/agezt/agezt/kernel/app/providers"
	apppulse "github.com/agezt/agezt/kernel/app/pulse"
	appseats "github.com/agezt/agezt/kernel/app/seats"
	apptaste "github.com/agezt/agezt/kernel/app/taste"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	appupdate "github.com/agezt/agezt/kernel/app/update"
	appwebhook "github.com/agezt/agezt/kernel/app/webhook"
	appworkboard "github.com/agezt/agezt/kernel/app/workboard"
	"github.com/agezt/agezt/kernel/contract/opapi"
)

// The door of the CLI/SDK request side (W5.9).
//
// W5.8 typed the bodies the console's route catalogue carries. The registry's
// own surface — the operations a CLI or an SDK calls, the ones no console
// route names — was the last surface still reading "anything goes", so a
// mistyped field reached those handlers as a silent default.
//
// The door is the same door: a body whose field is not the type the request
// contract names is refused BEFORE the handler runs and BEFORE the audit sees
// it. A mistyped field is a usage error, not a silent default, so the
// operator learns it from the door instead of from a wrong effect.

func TestCLISurfaceRequestDoorsRefuseMistypedBodies(t *testing.T) {
	workboardOps, err := appworkboard.Operations(func(context.Context) appworkboard.NativeServices { return appworkboard.NativeServices{} })
	if err != nil {
		t.Fatal(err)
	}
	pulseOps, err := apppulse.ControlOperations(func(context.Context) *apppulse.Controls { return &apppulse.Controls{} })
	if err != nil {
		t.Fatal(err)
	}
	boardOps, err := appboard.Operations(
		func(context.Context) (*appboard.Service, error) { return nil, nil },
		func(context.Context) (*appboard.Service, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	okrOps, err := appokr.Operations(
		func(context.Context) *appokr.Service { return nil },
		func(context.Context) *appokr.Lifecycle { return nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	seatOps, err := appseats.Operations(func(context.Context) *appseats.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	tasteOps, err := apptaste.Operations(func(context.Context) *apptaste.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	edictOps, err := appedict.Operations(func(context.Context) *appedict.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	edictDecisionOps, err := appedict.DecisionOperations(func(context.Context) *appedict.Decisions { return nil })
	if err != nil {
		t.Fatal(err)
	}
	journalOps, err := appjournal.Operations(func(context.Context) *appjournal.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	journalTraceOps, err := appjournal.TraceOperations(func(context.Context) *appjournal.Trace { return nil })
	if err != nil {
		t.Fatal(err)
	}
	providerOps, err := appproviders.ObservationOperations(func(context.Context) *appproviders.Observations { return nil })
	if err != nil {
		t.Fatal(err)
	}
	toolOps, err := apptools.ObservationOperations(func(context.Context) *apptools.Observations { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	updateOps, err := appupdate.Operations(func(context.Context) *appupdate.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	webhookOps, err := appwebhook.Operations(func(context.Context) *appwebhook.Observability { return nil })
	if err != nil {
		t.Fatal(err)
	}
	ops := []app.Operation{}
	ops = append(ops, workboardOps...)
	ops = append(ops, pulseOps...)
	ops = append(ops, boardOps...)
	ops = append(ops, okrOps...)
	ops = append(ops, seatOps...)
	ops = append(ops, tasteOps...)
	ops = append(ops, edictOps...)
	ops = append(ops, edictDecisionOps...)
	ops = append(ops, journalOps...)
	ops = append(ops, journalTraceOps...)
	ops = append(ops, providerOps...)
	ops = append(ops, toolOps...)
	ops = append(ops, updateOps...)
	ops = append(ops, webhookOps...)
	audit := &typedAdmissionAudit{}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: typedAdmissionAuth{}, Router: typedAdmissionRouter{}, Audit: audit})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, op, raw, want string
	}{
		{"workboard title", opapi.WorkboardCreate, `{"title":7}`, "$.title has wrong type: got number, want string"},
		{"workboard maximum", opapi.WorkboardCreate, `{"title":"owned","max_attempts":"3"}`, "$.max_attempts has wrong type: got string, want number"},
		{"workboard page", opapi.WorkboardSweep, `{"limit":"10"}`, "$.limit has wrong type: got string, want number"},
		{"workboard seat", opapi.WorkboardSeat, `{"id":"t","seat":7}`, "$.seat has wrong type: got number, want string"},
		{"pulse issue key", opapi.PulseAskResolve, `{"issue_key":7}`, "$.issue_key has wrong type: got number, want string"},
		{"pulse approval", opapi.PulseAskResolve, `{"issue_key":"owned","approve":1}`, "$.approve has wrong type: got number, want boolean|string"},
		{"pulse dial", opapi.PulseDial, `{"dial":null}`, "$.dial has wrong type: got null, want string"},
		{"board page", opapi.BoardRead, `{"limit":"ten"}`, "$.limit has wrong type: got string, want number"},
		{"board inbox flag", opapi.BoardInbox, `{"to":"me","all":"true"}`, "$.all has wrong type: got string, want boolean"},
		{"board topic", opapi.BoardRead, `{"topic":7}`, "$.topic has wrong type: got number, want string"},
		{"okr title", opapi.OKRCreate, `{"title":7}`, "$.title has wrong type: got number, want string"},
		{"okr target", opapi.OKRKeyResult, `{"id":"o","title":"t","target":"2"}`, "$.target has wrong type: got string, want number"},
		{"seat id", opapi.SeatCreate, `{"id":7}`, "$.id has wrong type: got number, want string"},
		{"seat flag", opapi.SeatCreate, `{"id":"s","restrict_tools":"yes"}`, "$.restrict_tools has wrong type: got string, want boolean"},
		{"taste title", opapi.TasteCreate, `{"title":7}`, "$.title has wrong type: got number, want string"},
		{"edict denial", opapi.EdictLog, `{"denied":"yes"}`, "$.denied has wrong type: got string, want boolean"},
		{"edict tenant", opapi.EdictShow, `{"tenant":7}`, "$.tenant has wrong type: got number, want string"},
		{"journal tail", opapi.JournalTail, `{"n":"20"}`, "$.n has wrong type: got string, want number"},
		{"journal grep", opapi.JournalGrep, `{"pattern":7}`, "$.pattern has wrong type: got number, want string"},
		{"provider page", opapi.ProviderLog, `{"limit":"20"}`, "$.limit has wrong type: got string, want number"},
		{"tool failures", opapi.ToolLog, `{"errors":"yes"}`, "$.errors has wrong type: got string, want boolean"},
		{"update version", opapi.UpdateApply, `{"version":7}`, "$.version has wrong type: got number, want string"},
		{"webhook failure flag", opapi.WebhookLog, `{"failed":"yes"}`, "$.failed has wrong type: got string, want boolean"},
	} {
		before := audit.calls
		_, err := d.Dispatch(context.Background(), opapi.Caller{}, tc.op, json.RawMessage(tc.raw), nil)
		if err == nil {
			t.Fatalf("%s: %s accepted %s", tc.name, tc.op, tc.raw)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %s %s: %v, want %q", tc.name, tc.op, tc.raw, err, tc.want)
		}
		if audit.calls != before {
			t.Fatalf("%s: %s %s: the refused body was audited", tc.name, tc.op, tc.raw)
		}
	}
}
