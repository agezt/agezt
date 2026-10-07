// SPDX-License-Identifier: MIT

package standing_test

import (
	"context"
	"encoding/json"
	"errors"
	appstanding "github.com/agezt/agezt/kernel/app/standing"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/roster"
	orders "github.com/agezt/agezt/kernel/standing"
	"reflect"
	"testing"
	"time"
)

type historyPort struct {
	events []*event.Event
	cause  error
	reads  int
}

func (p *historyPort) Range(fn func(*event.Event) error) error {
	p.reads++
	for _, e := range p.events {
		if err := fn(e); err != nil {
			return err
		}
	}
	return p.cause
}
func TestStandingWhyRetainsPrefixIDPayloadOrderRequiredFieldsAndBestEffortPartialHistory(t *testing.T) {
	valid := func(id string, seq int64, payload json.RawMessage) *event.Event {
		return &event.Event{ID: id, Seq: seq, Kind: event.KindStandingUpdated, CorrelationID: "owned-corr", TSUnixMS: seq, Payload: payload}
	}
	events := []*event.Event{
		{ID: "other-kind", Kind: event.KindTaskCompleted, Payload: json.RawMessage(`{"id":"owned"}`)},
		valid("first", 0, json.RawMessage(`{"id":"owned","enabled":false,"nested":{"value":0}}`)),
		valid("foreign", 1, json.RawMessage(`{"id":"other"}`)),
		valid("malformed", 2, json.RawMessage(`{`)),
		valid("numeric-id", 3, json.RawMessage(`{"id":7}`)),
		{ID: "custom", Seq: 4, Kind: event.Kind("standing.custom"), Payload: json.RawMessage(`{"id":"owned","action":"custom"}`)},
		valid("last", 5, json.RawMessage(`{"id":"owned","action":"edited"}`)),
	}
	cause := errors.New("owned partial range cause")
	for _, withError := range []bool{false, true} {
		p := &historyPort{events: events}
		if withError {
			p.cause = cause
		}
		out, err := appstanding.NewObservations(p).Why(context.Background(), appstanding.WhyInput{ID: "owned"})
		if err != nil || out.ID != "owned" || out.Count != 3 || len(out.Events) != 3 || out.Events[0].ID != "first" || out.Events[1].ID != "custom" || out.Events[2].ID != "last" || out.Events[0].Payload["enabled"] != false || out.Events[0].Seq != 0 || out.Events[0].CorrelationID != "owned-corr" {
			t.Fatal(out, err)
		}
		raw, _ := json.Marshal(out.Events[0])
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		for _, key := range []string{"seq", "id", "kind", "correlation_id", "ts_unix_ms", "payload"} {
			if _, ok := fields[key]; !ok {
				t.Fatal(key, string(raw))
			}
		}
		none, err := appstanding.NewObservations(p).Why(context.Background(), appstanding.WhyInput{ID: "missing"})
		raw, _ = json.Marshal(none)
		if err != nil || string(raw) != `{"id":"missing","events":null,"count":0}` {
			t.Fatal(none, err, string(raw))
		}
	}
	empty, err := appstanding.NewObservations(&historyPort{cause: cause}).Why(context.Background(), appstanding.WhyInput{ID: "owned"})
	if err != nil || empty.Count != 0 || empty.Events != nil {
		t.Fatal(empty, err)
	}
}
func TestStandingWhyUsesOwnedActualLifecycleJournal(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{Now: func() time.Time { return time.UnixMilli(0) }})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for _, kind := range []event.Kind{event.KindStandingCreated, event.KindStandingUpdated, event.KindStandingRemoved} {
		if _, err := j.Append(event.Spec{Kind: kind, Subject: "standing.owned", Actor: "fixture", CorrelationID: "owned", Payload: map[string]any{"id": "owned"}}); err != nil {
			t.Fatal(err)
		}
	}
	out, err := appstanding.NewObservations(j).Why(context.Background(), appstanding.WhyInput{ID: "owned"})
	if err != nil || out.Count != 3 || out.Events[0].Seq != 0 || out.Events[2].Seq != 2 || out.Events[0].TSUnixMS != 0 {
		t.Fatal(out, err)
	}
}
func TestStandingFireRetainsAvailabilityLookupValidationCallbackOrderAndFalseResult(t *testing.T) {
	no := false
	for _, mode := range []string{"unavailable", "missing", "unknown-agent", "retired", "paused", "managed", "accepted", "declined", "blank-agent"} {
		t.Run(mode, func(t *testing.T) {
			p := &standingPort{found: true, current: orders.Order{ID: "canonical-order", Agent: " alias "}}
			profile := roster.Profile{Slug: "canonical", Enabled: true}
			switch mode {
			case "retired":
				profile.Retired = true
				profile.Enabled = false
			case "paused":
				profile.Enabled = false
			case "managed":
				profile.DirectCallable = &no
			case "missing":
				p.found = false
			case "blank-agent":
				p.current.Agent = " "
			}
			service := appstanding.New(p, p, appstanding.Host{Agent: func(ref string) (roster.Profile, bool) {
				p.calls = append(p.calls, "validate")
				if ref != "alias" {
					t.Fatal(ref)
				}
				return profile, mode != "unknown-agent"
			}, ManagedDirectError: func(profile roster.Profile, action string) string {
				if profile.Slug != "canonical" || action != "called" {
					t.Fatal(profile, action)
				}
				return "selected managed refusal"
			}})
			callback := func(id string) bool {
				p.calls = append(p.calls, "fire")
				if id != "request-id" {
					t.Fatal(id)
				}
				return mode != "declined"
			}
			if mode == "unavailable" {
				callback = nil
			}
			out, err := appstanding.NewFiring(service, callback).Fire(context.Background(), appstanding.FireInput{ID: "request-id"})
			expected := []string{"get", "validate"}
			wantError := ""
			switch mode {
			case "unavailable":
				expected = nil
				wantError = "standing-order firing is not available on this daemon"
			case "missing":
				expected = []string{"get"}
			case "unknown-agent":
				wantError = "unknown standing agent: alias"
			case "retired":
				wantError = "standing agent canonical is retired"
			case "paused":
				wantError = "standing agent canonical is paused"
			case "managed":
				wantError = "standing selected managed refusal"
			case "accepted", "declined":
				expected = append(expected, "fire")
			case "blank-agent":
				expected = []string{"get", "fire"}
			}
			if !reflect.DeepEqual(p.calls, expected) {
				t.Fatal(p.calls, expected)
			}
			if wantError != "" {
				if err == nil || err.Error() != wantError || !reflect.DeepEqual(out, appstanding.FireOutput{}) {
					t.Fatal(out, err, wantError)
				}
			} else {
				if err != nil || out.ID != "request-id" || out.Fired != (mode == "accepted" || mode == "blank-agent") {
					t.Fatal(out, err)
				}
				raw, _ := json.Marshal(out)
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				if fields["fired"] == nil || fields["id"] == nil {
					t.Fatal(string(raw))
				}
			}
		})
	}
}
