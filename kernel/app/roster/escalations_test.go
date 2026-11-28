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
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
	"github.com/agezt/agezt/kernel/modules/board/board"
)

type fakeEscalationBoard struct {
	help    []board.Message
	replies map[string][]board.Message
}

func (b fakeEscalationBoard) Help() []board.Message             { return b.help }
func (b fakeEscalationBoard) Replies(id string) []board.Message { return b.replies[id] }

func escalationFixture(b fakeEscalationBoard, events []*event.Event, boardErr error) (*EscalationService, *int, *int) {
	boards, reads := 0, 0
	return NewEscalations(func(ref string) (core.Profile, bool) {
		if ref == "a" {
			return core.Profile{Slug: "a"}, true
		}
		return core.Profile{}, false
	}, func() (EscalationBoard, error) {
		boards++
		if boardErr != nil {
			return nil, boardErr
		}
		return b, nil
	}, func(fn func(*event.Event) error) error {
		reads++
		for _, e := range events {
			_ = fn(e)
		}
		return nil
	}), &boards, &reads
}

func escalationRun(t *testing.T, s *EscalationService, raw string) (EscalationsOutput, error) {
	t.Helper()
	var in opapi.AgentPageRequest
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		// A mistyped field is refused by the door before the service sees
		// the body; here it is the decoder that refuses it.
		return EscalationsOutput{}, err
	}
	return s.Escalations(context.Background(), in)
}

func doctorEvent(seq int64, pl map[string]any) *event.Event {
	raw, _ := json.Marshal(pl)
	return &event.Event{Seq: seq, Kind: event.KindInfo, Subject: "doctor.auto_repair", Payload: raw}
}

func TestRosterEscalationsFoldStatusesAndMetadata(t *testing.T) {
	b := fakeEscalationBoard{help: []board.Message{
		{ID: "m1", From: "doc", To: "a", Text: "Doctor flagged a problem for agent broken. Fix it", TSMS: 10, Help: true},
		{ID: "m2", From: "doc", To: board.Everyone, Text: "help all", TSMS: 30, Help: true, AckedBy: []string{" A "}},
		{ID: "m3", From: "x", To: "a", Text: "answered", TSMS: 20, Help: true},
		{ID: "m4", From: "x", To: "b", Text: "not mine", TSMS: 40, Help: true},
		{ID: "m5", From: "x", To: "a", Text: "plain post", TSMS: 50},
		{ID: "m6", From: "boss", To: "a", Text: "Doctor x", TSMS: 30, Help: true},
	}, replies: map[string][]board.Message{"m3": {{ID: "r1"}, {ID: "r2"}}}}
	events := []*event.Event{
		doctorEvent(1, map[string]any{"target_agent": "a", "mailbox_message_id": "m1", "agent": "old", "phase": "escalation_woke"}),
		doctorEvent(2, map[string]any{"target_agent": "a", "mailbox_message_id": "m1", "agent": "src", "phase": "escalation_woke", "mode": "degraded", "reason": "why", "error": "e", "target_correlation": "tc", "fingerprint": "fp", "resolution": "force_chain", "resolution_summary": "rs", "delegate_to": "d", "root_agent": "root", "chain_depth": float64(2), "incident_id": "i", "root_incident_id": "ri", "parent_incident_id": "pi"}),
		doctorEvent(3, map[string]any{"target_agent": "a", "mailbox_message_id": "m6", "agent": "src", "phase": "delegation_woke", "delegated_by": "lead"}),
		doctorEvent(4, map[string]any{"target_agent": "b", "mailbox_message_id": "m2", "agent": "foreign"}),
		doctorEvent(5, map[string]any{"target_agent": "a", "mailbox_message_id": " ", "agent": "blank"}),
		{Seq: 6, Kind: event.KindInfo, Subject: "doctor.auto_repair", Payload: json.RawMessage(`{"target_agent":"a"`)},
	}
	s, _, reads := escalationFixture(b, events, nil)
	out, err := escalationRun(t, s, `{"ref":"a"}`)
	if err != nil || *reads != 1 {
		t.Fatal(err)
	}
	ids := func(rows []EscalationOutputRow) (got []string) {
		for _, r := range rows {
			got = append(got, r.MessageID)
		}
		return got
	}
	if !reflect.DeepEqual(ids(out.Escalations), []string{"m2", "m6", "m3", "m1"}) || out.Count != 4 || out.OpenCount != 2 || out.Slug != "a" || out.NextCursor != "" {
		t.Fatal("rows", ids(out.Escalations), out.OpenCount)
	}
	byID := map[string]EscalationOutputRow{}
	for _, r := range out.Escalations {
		byID[r.MessageID] = r
	}
	if r := byID["m2"]; r.Status != "acked" || !r.Acked || r.OriginKind != "doctor" || r.OriginAgent != "doc" || r.SourceAgent != "" || r.RootAgent != "" {
		t.Fatal("acked broadcast", r)
	}
	if r := byID["m3"]; r.Status != "answered" || r.ReplyCount != 2 {
		t.Fatal("answered", r)
	}
	want := EscalationOutputRow{MessageID: "m1", From: "doc", To: "a", Text: "Doctor flagged a problem for agent broken. Fix it", TSUnixMS: 10, Status: "open", SourceAgent: "src", Mode: "degraded", WakePhase: "escalation_woke", WakeReason: "why", WakeError: "e", WakeCorrelationID: "tc", Fingerprint: "fp", Resolution: "force_chain", ResolutionSummary: "rs", DelegateTo: "d", OriginKind: "doctor", OriginAgent: "doc", RootAgent: "root", ChainDepth: 2, IncidentID: "i", RootIncidentID: "ri", ParentIncidentID: "pi"}
	if byID["m1"] != want {
		t.Fatalf("latest metadata wins\n got %+v\nwant %+v", byID["m1"], want)
	}
	if r := byID["m6"]; r.OriginKind != "delegated" || r.OriginAgent != "lead" || r.SourceAgent != "src" || r.RootAgent != "src" {
		t.Fatal("delegated origin", r)
	}
	textOnly, _, _ := escalationFixture(fakeEscalationBoard{help: []board.Message{{ID: "t", From: "doc", To: "a", Text: "Doctor says: escalation for agent  broken-one . more", TSMS: 1, Help: true}}}, nil, nil)
	got, _ := escalationRun(t, textOnly, `{"ref":"a"}`)
	if r := got.Escalations[0]; r.SourceAgent != "broken-one" || r.RootAgent != "broken-one" || r.OriginKind != "doctor" || r.OriginAgent != "doc" {
		t.Fatal("source parsed from text", r)
	}
	for text, want := range map[string]string{"Doctors for agent y.": "", "Doctor x for agent y": "", "Nurse for agent y.": "", "Doctor for agent y.z": "y", " Doctor a for agent b. ": "b"} {
		if got := escalationSourceFromText(text); got != want {
			t.Error(text, got, want)
		}
	}
	empty, _, _ := escalationFixture(fakeEscalationBoard{}, nil, nil)
	got, _ = escalationRun(t, empty, `{"ref":"a"}`)
	raw, _ := json.Marshal(got)
	if string(raw) != `{"slug":"a","escalations":[],"count":0,"open_count":0}` {
		t.Fatal("empty shape", string(raw))
	}
}

func TestRosterEscalationsArgumentOrderAndCursor(t *testing.T) {
	var help []board.Message
	for i := range 130 {
		help = append(help, board.Message{ID: "m" + strconv.Itoa(1000+i), To: "a", Help: true, TSMS: int64(i / 2)})
	}
	s, boards, reads := escalationFixture(fakeEscalationBoard{help: help}, nil, nil)
	for raw, want := range map[string]string{
		`{}`: "args.ref required", `{"ref":"  "}`: "args.ref required", `{"ref":null}`: "args.ref required",
		`{"ref":"ghost"}`: "unknown agent: ghost",
	} {
		if _, err := escalationRun(t, s, raw); err == nil || err.Error() != want {
			t.Fatal(raw, err)
		}
	}
	// A mistyped field never reaches the service: the door refuses it in the
	// daemon, and the decoder refuses it here.
	for _, raw := range []string{`{"ref":1}`, `{"ref":"a","limit":"x"}`, `{"ref":"a","cursor":5}`} {
		if _, err := escalationRun(t, s, raw); err == nil {
			t.Fatal(raw, "a mistyped field was accepted")
		}
	}
	if *boards != 0 || *reads != 0 {
		t.Fatal("board/journal before argument admission")
	}
	// A null cursor is no cursor: the page starts at the beginning.
	if _, err := escalationRun(t, s, `{"ref":"a","cursor":null}`); err != nil {
		t.Fatal("null cursor", err)
	}
	failing, _, failReads := escalationFixture(fakeEscalationBoard{}, nil, errors.New("board offline"))
	if _, err := escalationRun(t, failing, `{"ref":"a"}`); err == nil || err.Error() != "board offline" || *failReads != 0 {
		t.Fatal("board error precedes the page", err)
	}
	for raw, want := range map[string]int{`{"ref":"a"}`: 20, `{"ref":"a","limit":100}`: 100, `{"ref":"a","limit":120}`: 100, `{"ref":"a","limit":1}`: 1, `{"ref":"a","limit":0.5}`: 130} {
		out, err := escalationRun(t, s, raw)
		if err != nil || out.Count != want {
			t.Fatal(raw, out.Count, err)
		}
	}
	page, _ := escalationRun(t, s, `{"ref":"a","limit":3}`)
	if page.NextCursor != "63:m1126" || !reflect.DeepEqual([]string{page.Escalations[0].MessageID, page.Escalations[1].MessageID, page.Escalations[2].MessageID}, []string{"m1128", "m1129", "m1126"}) {
		t.Fatal("first page keeps roster order within equal timestamps", page.NextCursor, page.Escalations)
	}
	next, _ := escalationRun(t, s, `{"ref":"a","limit":2,"cursor":"63:m1126"}`)
	if next.Escalations[0].MessageID != "m1124" || next.Escalations[1].MessageID != "m1125" {
		t.Fatal("next page", next.Escalations[0].MessageID, next.Escalations[1].MessageID)
	}
	tie, _ := escalationRun(t, s, `{"ref":"a","limit":1,"cursor":"63:m1127"}`)
	if tie.Escalations[0].MessageID != "m1126" {
		t.Fatal("same-ts tie skips ids at or above the cursor id only", tie.Escalations[0].MessageID)
	}
	for raw, first := range map[string]string{`{"ref":"a","cursor":"x:m"}`: "m1128", `{"ref":"a","cursor":""}`: "m1128", `{"ref":"a","cursor":"0:"}`: "m1128", `{"ref":"a","cursor":"0:zzz"}`: "m1000"} {
		out, err := escalationRun(t, s, raw)
		if err != nil || out.Count == 0 || out.Escalations[0].MessageID != first {
			t.Fatal(raw, out.Count, err)
		}
	}
}

func TestRosterEscalationsOperationSpecAndAdmission(t *testing.T) {
	if _, err := EscalationOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	s, boards, _ := escalationFixture(fakeEscalationBoard{help: []board.Message{{ID: "m", To: "a", Help: true}}}, nil, nil)
	calls := 0
	ops, err := EscalationOperations(func(ctx context.Context) *EscalationService {
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
	if spec.Name != "agent_escalations" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput || spec.Input != reflect.TypeFor[opapi.AgentPageRequest]() || spec.Output != reflect.TypeFor[EscalationsOutput]() {
		t.Fatal(spec)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}} {
		d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{principal}, Router: listRoute{}})
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_escalations", json.RawMessage(`{"ref":"a"}`), nil); err == nil || calls != 0 || *boards != 0 {
			t.Fatal("non-primary effects", err)
		}
	}
	d, _ := app.NewDispatcher(ops, app.Dependencies{Auth: listAuth{opapi.Principal{Kind: opapi.Operator}}, Router: listRoute{}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := d.Dispatch(ctx, opapi.Caller{}, "agent_escalations", json.RawMessage(`{"ref":"a"}`), nil); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled effects", err)
	}
	out, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_escalations", json.RawMessage(`{"ref":"a","x":1}`), nil)
	page, ok := out.(EscalationsOutput)
	if err != nil || !ok || page.Count != 1 || page.OpenCount != 1 || *boards != 1 {
		t.Fatal(out, err)
	}
}
