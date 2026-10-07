// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestChannelInboxOperationSpecNestedSchemaAndPresence(t *testing.T) {
	if _, err := InboxOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	ops, err := InboxOperations(func(context.Context) *Inbox { return nil })
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	s := ops[0].Spec()
	if s.Name != "inbox" || !s.ReadOnly || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.HTTP.Method != "GET" || s.HTTP.Path != "/api/inbox" || s.Input != reflect.TypeFor[InboxRequest]() || s.Output != reflect.TypeFor[InboxOutput]() {
		t.Fatal(s)
	}
	var node map[string]any
	json.Unmarshal(s.OutputSchema, &node)
	props := node["properties"].(map[string]any)
	threads := props["threads"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	messages := threads["messages"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if len(props) != 5 || len(threads) != 5 || len(messages) != 5 {
		t.Fatal(string(s.OutputSchema))
	}
	empty, _ := NewInbox(&ownedInboxJournal{}).List(context.Background(), InboxInput{})
	raw, _ := json.Marshal(empty)
	if string(raw) != `{"threads":[],"count":0,"total":0}` || schema.ValidateJSON(s.OutputSchema, raw) != nil {
		t.Fatal(string(raw))
	}
	j := &ownedInboxJournal{events: []*event.Event{ownedInboxEvent("owned", "owned", 9223372036854775807, event.KindChannelInbound, `{"text":"original"}`)}}
	out, _ := NewInbox(j).List(context.Background(), InboxInput{Channel: "ignored"})
	if out.Threads == nil || out.Channel != "ignored" || out.NextCursor != "" {
		t.Fatal(out)
	}
	out, _ = NewInbox(j).List(context.Background(), InboxInput{})
	raw, _ = json.Marshal(out)
	if !strings.Contains(string(raw), `"ts_unix_ms":9223372036854775807`) || schema.ValidateJSON(s.OutputSchema, raw) != nil {
		t.Fatal(string(raw))
	}
	out.Threads[0].Messages[0].Text = "mutated"
	out.Threads[0].ChannelID = "mutated"
	fresh, _ := NewInbox(j).List(context.Background(), InboxInput{})
	if fresh.Threads[0].Messages[0].Text != "original" || fresh.Threads[0].ChannelID != "" {
		t.Fatal("borrowed journal/output")
	}
	bad := inboxJSONView(t, fresh)
	bad["threads"].([]any)[0].(map[string]any)["last_ts_unix_ms"] = "wrong"
	raw, _ = json.Marshal(bad)
	if schema.ValidateJSON(s.OutputSchema, raw) == nil {
		t.Fatal("untyped nested timestamp")
	}
}

func TestChannelInboxRequestWireNumericLimitsAndDelayedCursorError(t *testing.T) {
	for _, tc := range []struct {
		raw             string
		limit           *int
		channel, cursor string
		cursorError     bool
	}{
		{`{}`, nil, "", "", false},
		{`{"limit":5.9,"channel":" Raw Channel ","cursor":" 100:z "}`, inboxLimit(5), " Raw Channel ", " 100:z ", false},
		{`{"limit":5}`, inboxLimit(5), "", "", false},
		{`{"limit":null,"channel":false,"cursor":null}`, nil, "", "", true},
		{`{"limit":"5","channel":5,"cursor":false}`, nil, "", "", true},
		{`{"limit":false,"cursor":5}`, nil, "", "", true},
		{`{"limit":{},"channel":[],"unknown":true}`, nil, "", "", false},
	} {
		var r InboxRequest
		if err := json.Unmarshal([]byte(tc.raw), &r); err != nil {
			t.Fatal(err)
		}
		in := r.input()
		if !reflect.DeepEqual(in.Limit, tc.limit) || in.Channel != tc.channel || in.Cursor != tc.cursor || (in.CursorError != nil) != tc.cursorError {
			t.Fatal(tc, in)
		}
		if in.CursorError != nil && in.CursorError.Error() != "args.cursor must be a string" {
			t.Fatal(in.CursorError)
		}
	}
	// At the actual JSON boundary Go integer/json.Number arguments both become
	// numbers, just as the former native Request decoder produced float64 values.
	for _, value := range []any{5, json.Number("5"), 5.9} {
		raw, _ := json.Marshal(map[string]any{"limit": value})
		var r InboxRequest
		json.Unmarshal(raw, &r)
		if r.input().Limit == nil || *r.input().Limit != 5 {
			t.Fatal(string(raw))
		}
	}
}

func TestChannelInboxOperationAdmissionAndRangeBeforeCursorError(t *testing.T) {
	for _, mode := range []string{"success", "canceled", "tenant", "agent", "range-error", "cursor-error"} {
		j := &ownedInboxJournal{}
		providers := 0
		sentinel := errors.New("owned range failure")
		if mode == "range-error" {
			j.err = sentinel
		}
		ops, _ := InboxOperations(func(ctx context.Context) *Inbox {
			providers++
			if ctx.Value(ownedACPRouteKey{}) != "owned-route" {
				t.Fatal("route lost")
			}
			return NewInbox(j)
		})
		principal := opapi.Principal{Kind: opapi.Operator}
		if mode == "tenant" {
			principal = opapi.Principal{Kind: opapi.Tenant, Tenant: "owned"}
		}
		if mode == "agent" {
			principal.Kind = opapi.Agent
		}
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}})
		ctx, cancel := context.WithCancel(context.Background())
		if mode == "canceled" {
			cancel()
		}
		raw := json.RawMessage(`{"unknown":true}`)
		if mode == "range-error" || mode == "cursor-error" {
			raw = json.RawMessage(`{"cursor":false}`)
		}
		out, err := d.Dispatch(ctx, opapi.Caller{Tenant: "ignored"}, "inbox", raw, nil)
		cancel()
		if mode == "canceled" || mode == "tenant" || mode == "agent" {
			if err == nil || out != nil || providers != 0 || j.calls != 0 {
				t.Fatal(mode, out, err, providers, j.calls)
			}
			if mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			continue
		}
		if providers != 1 || j.calls != 1 {
			t.Fatal(mode, providers, j.calls)
		}
		if mode == "range-error" {
			if !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			continue
		}
		if mode == "cursor-error" {
			if err == nil || err.Error() != "args.cursor must be a string" {
				t.Fatal(err)
			}
			continue
		}
		if err != nil || out.(InboxOutput).Threads == nil || out.(InboxOutput).Count != 0 {
			t.Fatal(out, err)
		}
	}
}
