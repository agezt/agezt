// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/roster"
)

func activityFixture(events []*event.Event) (*ActivityService, *int) {
	reads := 0
	get := func(ref string) (core.Profile, bool) {
		if ref == "a" || ref == "id-a" {
			return core.Profile{ID: "id-a", Slug: "a"}, true
		}
		return core.Profile{}, false
	}
	return NewActivity(get, func(fn func(*event.Event) error) error {
		reads++
		for _, e := range events {
			if err := fn(e); err != nil {
				return err
			}
		}
		return nil
	}), &reads
}

func activityEvent(seq int64, kind event.Kind, corr string, pl map[string]any) *event.Event {
	raw, _ := json.Marshal(pl)
	return &event.Event{Seq: seq, Kind: kind, CorrelationID: corr, TSUnixMS: seq * 10, Payload: raw}
}

func activityRun(t *testing.T, s *ActivityService, raw string) (ActivityOutput, error) {
	t.Helper()
	var in ActivityRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(raw, err)
	}
	return s.Activity(context.Background(), in)
}

func TestRosterActivityArgumentOrderMatchesLegacy(t *testing.T) {
	s, reads := activityFixture(nil)
	for raw, want := range map[string]string{
		`{}`: "args.ref required", `{"ref":"  "}`: "args.ref required", `{"ref":7}`: "args.ref must be a string", `{"ref":null}`: "args.ref must be a string",
		`{"ref":"ghost","limit":"x"}`: "unknown agent: ghost", `{"ref":" a"}`: "unknown agent:  a", `{"ref":"a","limit":"x"}`: "args.limit must be a number", `{"ref":"a","limit":null}`: "args.limit must be a number",
	} {
		if _, err := activityRun(t, s, raw); err == nil || err.Error() != want {
			t.Fatal(raw, err, want)
		}
	}
	if *reads != 0 {
		t.Fatal("journal read before argument admission", *reads)
	}
	out, err := activityRun(t, s, `{"ref":"id-a","cursor":17,"unknown":true}`)
	raw, _ := json.Marshal(out)
	if err != nil || string(raw) != `{"slug":"a","activity":null,"count":0,"total":0}` || *reads != 1 {
		t.Fatal("empty timeline shape", string(raw), err)
	}
}

func TestRosterActivityScopesOrdersAndPages(t *testing.T) {
	events := []*event.Event{
		activityEvent(1, event.KindTaskReceived, "run-a", map[string]any{"agent": "a", "intent": "first"}),
		activityEvent(2, event.KindTaskReceived, "run-b", map[string]any{"agent": "b", "intent": "other"}),
		activityEvent(3, event.KindCouncilConvened, "run-a", map[string]any{"question": "mine"}),
		activityEvent(4, event.KindCouncilConvened, "run-b", map[string]any{"question": "theirs"}),
		activityEvent(5, event.KindTaskCompleted, "run-a", nil),
		activityEvent(6, event.KindTaskReceived, "", map[string]any{"agent": "a", "intent": "no correlation"}),
		activityEvent(7, event.KindTaskCompleted, "", nil),
		{Seq: 8, Kind: event.KindTaskReceived, CorrelationID: "run-c", Payload: json.RawMessage(`{"agent":"a"`)},
		activityEvent(9, event.KindTaskCompleted, "run-c", nil),
	}
	s, _ := activityFixture(events)
	out, err := activityRun(t, s, `{"ref":"a"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []ActivityItem{
		{Seq: 6, Kind: string(event.KindTaskReceived), TSUnixMS: 60, Summary: "started a run: no correlation"},
		{Seq: 5, Kind: string(event.KindTaskCompleted), TSUnixMS: 50, CorrelationID: "run-a", Summary: "completed a run"},
		{Seq: 3, Kind: string(event.KindCouncilConvened), TSUnixMS: 30, CorrelationID: "run-a", Summary: "consulted the council: mine"},
		{Seq: 1, Kind: string(event.KindTaskReceived), TSUnixMS: 10, CorrelationID: "run-a", Summary: "started a run: first"},
	}
	if !reflect.DeepEqual(out.Activity, want) || out.Count != 4 || out.Total != 4 || out.NextCursor != "" || out.Slug != "a" {
		t.Fatal("timeline", out)
	}
	page, _ := activityRun(t, s, `{"ref":"a","limit":2}`)
	if page.Count != 2 || page.Total != 4 || page.NextCursor != "5" || page.Activity[1].Seq != 5 {
		t.Fatal("first page", page)
	}
	for raw, seqs := range map[string][]int64{
		`{"ref":"a","limit":2,"cursor":"5"}`: {3, 1}, `{"ref":"a","cursor":" 4 "}`: {3, 1}, `{"ref":"a","cursor":"0"}`: {6, 5, 3, 1},
		`{"ref":"a","cursor":"-3"}`: {6, 5, 3, 1}, `{"ref":"a","cursor":"x"}`: {6, 5, 3, 1}, `{"ref":"a","cursor":5}`: {6, 5, 3, 1},
		`{"ref":"a","limit":1.9}`: {6}, `{"ref":"a","limit":1}`: {6}, `{"ref":"a","limit":0.5}`: {6, 5, 3, 1}, `{"ref":"a","limit":-1}`: {6, 5, 3, 1},
	} {
		out, err := activityRun(t, s, raw)
		var got []int64
		for _, it := range out.Activity {
			got = append(got, it.Seq)
		}
		if err != nil || !reflect.DeepEqual(got, seqs) || out.Total != 4 {
			t.Fatal(raw, got, out.Total, err)
		}
	}
	if out, _ := activityRun(t, s, `{"ref":"a","cursor":"1"}`); out.Activity == nil || len(out.Activity) != 0 || out.Count != 0 {
		t.Fatal("cursor-filtered page is an empty array, not null", out)
	}
}

func TestRosterActivityLimitDefaultAndCap(t *testing.T) {
	var events []*event.Event
	for i := int64(1); i <= 600; i++ {
		events = append(events, activityEvent(i, event.KindTaskReceived, "r"+strconv.FormatInt(i, 10), map[string]any{"agent": "a"}))
	}
	s, _ := activityFixture(events)
	for raw, want := range map[string]int{`{"ref":"a"}`: 50, `{"ref":"a","limit":500}`: 500, `{"ref":"a","limit":9999}`: 500, `{"ref":"a","limit":800}`: 500, `{"ref":"a","limit":0}`: 50} {
		out, err := activityRun(t, s, raw)
		if err != nil || out.Count != want || out.Total != 600 || out.NextCursor != strconv.Itoa(600-want+1) {
			t.Fatal(raw, out.Count, out.NextCursor, err)
		}
	}
}

func TestRosterActivityOperationSpecAndAdmission(t *testing.T) {
	if _, err := ActivityOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, reads := activityFixture([]*event.Event{activityEvent(1, event.KindTaskReceived, "r", map[string]any{"agent": "a", "intent": "x"})})
	calls := 0
	ops, err := ActivityOperations(func(ctx context.Context) *ActivityService {
		calls++
		if ctx.Value(listRouteKey{}) != "selected" {
			t.Fatal("route lost")
		}
		return s
	})
	if err != nil || len(ops) != 1 {
		t.Fatal(ops, err)
	}
	spec := ops[0].Spec()
	if spec.Name != "agent_activity" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[ActivityRequest]() || spec.Output != reflect.TypeFor[ActivityOutput]() || spec.Stream != opapi.StreamNone || spec.HTTP != (opapi.HTTP{Method: "GET", Path: "/api/agents/activity"}) {
		t.Fatal(spec)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(`{"ref":"a"}`), nil); err == nil || calls != 0 || *reads != 0 {
			t.Fatal("non-primary effects", err)
		}
	}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_activity", json.RawMessage(`{"ref":"a"}`), nil); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled effects", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(`{"ref":"a","limit":"1"}`), nil)
	if err == nil || err.Error() != "args.limit must be a number" || out != nil {
		t.Fatal("codec error through dispatch", out, err)
	}
	out, err = d.Dispatch(context.Background(), opapi.Caller{}, "agent_activity", json.RawMessage(`{"ref":"a"}`), nil)
	page, ok := out.(ActivityOutput)
	if err != nil || !ok || page.Count != 1 || page.Activity[0].Summary != "started a run: x" || *reads != 1 {
		t.Fatal(out, err)
	}
}
