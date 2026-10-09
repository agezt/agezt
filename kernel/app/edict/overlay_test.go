// SPDX-License-Identifier: MIT

package edict

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
)

type overlayJournal struct {
	fakeJournal
	head int64
}

func (o overlayJournal) Head() (int64, string) { return o.head, "hash" }

func policyJournal() overlayJournal {
	ev := func(seq int64, kind event.Kind, payload string) *event.Event {
		return &event.Event{Seq: seq, Kind: kind, Payload: json.RawMessage(payload)}
	}
	return overlayJournal{head: 10, fakeJournal: fakeJournal{events: []*event.Event{
		ev(1, event.KindPolicyChanged, `{"action":"level.set","capability":"shell","from":"unset","to":"L1"}`),
		ev(2, event.KindPolicyChanged, `{"action":"deny.add","name":"runtime[1]","substring":"curl evil","applies_to":["shell"],"count":16}`),
		ev(3, event.KindPolicyDecision, `{"action":"mode.set","from":"allow","to":"deny"}`),
		ev(4, event.KindPolicyChanged, `{"action":"deny.add","name":"runtime[2]","substring":"exfil","applies_to":[],"count":17}`),
		ev(5, event.KindPolicyChanged, `not json`),
		ev(6, event.KindPolicyChanged, `{"action":"mode.set","from":"allow","to":"prompt"}`),
		ev(7, event.KindPolicyChanged, `{"action":"level.set","capability":"shell","from":"L1","to":"L3"}`),
		ev(8, event.KindPolicyChanged, `{"action":"deny.rm","name":"runtime[1]","count":16}`),
		ev(9, event.KindPolicyChanged, `{"action":"deny.add","name":"runtime[3]","substring":"/srv","applies_to":["file.delete","file.write"],"count":17}`),
	}}}
}

func TestOverlayShow(t *testing.T) {
	out, err := NewOverlay(policyJournal(), nil, nil).Show(context.Background(), OverlayRequest{})
	var changes []core.PolicyChange
	for _, e := range policyJournal().events {
		var ch core.PolicyChange
		if e.Kind == event.KindPolicyChanged && json.Unmarshal(e.Payload, &ch) == nil {
			changes = append(changes, ch)
		}
	}
	want := core.ProjectPolicyChanges(changes)
	if err != nil || out.ChangesFolded != 7 || out.Empty || out.Mode != want.Mode.String() || len(out.Levels) != len(want.Levels) || out.Levels["shell"] != want.Levels["shell"].String() || len(out.DenyRules) != len(want.DenyRules) {
		t.Fatalf("%+v %v", out, err)
	}
	for i, r := range want.DenyRules {
		applies := []string{}
		for _, c := range r.AppliesTo {
			applies = append(applies, string(c))
		}
		if !reflect.DeepEqual(out.DenyRules[i], OverlayDenyRow{Name: r.Name, Substring: r.Substring, AppliesTo: applies}) {
			t.Fatal("deny rules keep the fold order and their capabilities", out.DenyRules, want.DenyRules)
		}
	}
	if !reflect.DeepEqual(out.DenyRules, []OverlayDenyRow{{Name: "runtime[2]", Substring: "exfil", AppliesTo: []string{}}, {Name: "runtime[3]", Substring: "/srv", AppliesTo: []string{"file.delete", "file.write"}}}) {
		t.Fatal("the removed rule is gone and the survivor keeps its scope", out.DenyRules)
	}
	empty, _ := NewOverlay(overlayJournal{}, nil, nil).Show(context.Background(), OverlayRequest{})
	if raw, _ := json.Marshal(empty); string(raw) != `{"levels":{},"deny_rules":[],"mode":"","empty":true,"changes_folded":0}` {
		t.Fatal(string(raw))
	}
	global := overlayJournal{fakeJournal: fakeJournal{events: []*event.Event{{Kind: event.KindPolicyChanged, Payload: json.RawMessage(`{"action":"deny.add","name":"runtime[1]","substring":"x"}`)}}}}
	g, _ := NewOverlay(global, nil, nil).Show(context.Background(), OverlayRequest{})
	if raw, _ := json.Marshal(g.DenyRules); string(raw) != `[{"name":"runtime[1]","substring":"x","applies_to":[]}]` {
		t.Fatal("a global rule reports an empty applies_to list", string(raw))
	}
	boom := errors.New("journal unreadable")
	if _, err := NewOverlay(overlayJournal{fakeJournal: fakeJournal{err: boom}}, nil, nil).Show(context.Background(), OverlayRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
}

func TestOverlayCompact(t *testing.T) {
	var saved []*core.OverlaySnapshot
	rec := &recorder{}
	o := NewOverlay(policyJournal(), func(s *core.OverlaySnapshot) error { saved = append(saved, s); return nil }, rec.publish)
	out, err := o.Compact(context.Background(), OverlayRequest{})
	if err != nil || len(saved) != 1 || saved[0].ThroughSeq != 10 || out != (CompactOutput{Folded: 7, Compacted: len(saved[0].Changes), ThroughSeq: 10}) || out.Compacted == 0 {
		t.Fatalf("%+v %v %v", out, err, saved)
	}
	var changes []core.PolicyChange
	for _, e := range policyJournal().events {
		var ch core.PolicyChange
		if e.Kind == event.KindPolicyChanged && json.Unmarshal(e.Payload, &ch) == nil {
			changes = append(changes, ch)
		}
	}
	if minimal := core.ProjectPolicyChanges(changes).ToChanges(); !reflect.DeepEqual(saved[0].Changes, minimal) || out.Compacted != 4 {
		t.Fatal("the snapshot keeps the minimal change list, not the history", saved[0].Changes, minimal)
	}
	if len(rec.specs) != 1 || rec.specs[0].Subject != "policy.compacted" || rec.specs[0].Kind != event.KindPolicyCompacted || rec.specs[0].Actor != "controlplane" || rec.specs[0].CorrelationID != "" {
		t.Fatalf("%+v", rec.specs)
	}
	if raw, _ := json.Marshal(rec.specs[0].Payload); string(raw) != `{"content_hash":"`+saved[0].ContentHash()+`","through_seq":10}` {
		t.Fatal("the journal vouches for the snapshot's content hash", string(raw))
	}
	boom := errors.New("disk full")
	rec = &recorder{}
	if _, err := NewOverlay(policyJournal(), func(*core.OverlaySnapshot) error { return boom }, rec.publish).Compact(context.Background(), OverlayRequest{}); !errors.Is(err, boom) || rec.specs != nil {
		t.Fatal("a failed snapshot journals nothing", err)
	}
	if _, err := NewOverlay(overlayJournal{fakeJournal: fakeJournal{err: boom}}, func(*core.OverlaySnapshot) error { t.Fatal("no snapshot after a read error"); return nil }, rec.publish).Compact(context.Background(), OverlayRequest{}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	empty, _ := NewOverlay(overlayJournal{head: 3}, func(*core.OverlaySnapshot) error { return nil }, (&recorder{}).publish).Compact(context.Background(), OverlayRequest{})
	if empty != (CompactOutput{ThroughSeq: 3, Empty: true}) {
		t.Fatal(empty)
	}
}

func TestOverlayOperations(t *testing.T) {
	if _, err := OverlayOperations(nil); err == nil {
		t.Fatal("nil provider")
	}
	o := NewOverlay(policyJournal(), func(*core.OverlaySnapshot) error { return nil }, (&recorder{}).publish)
	ops, err := OverlayOperations(func(context.Context) *Overlay { return o })
	if err != nil || len(ops) != 2 {
		t.Fatal(ops, err)
	}
	show, _ := o.Show(context.Background(), OverlayRequest{})
	compact, _ := o.Compact(context.Background(), OverlayRequest{})
	for i, w := range []struct {
		name  string
		authz opapi.Authz
		out   any
	}{
		{"edict_overlay", opapi.OwnTenant, show},
		{"edict_compact", opapi.PrimaryOnly, compact},
	} {
		spec := ops[i].Spec()
		if spec.Name != w.name || spec.ReadOnly || spec.Authz != w.authz || spec.Tenancy != opapi.CallerTenant || !spec.AllowUnknownInput || spec.HTTP != (opapi.HTTP{}) || spec.Output != reflect.TypeOf(w.out) {
			t.Fatal(spec)
		}
		raw, _ := json.Marshal(w.out)
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatal(w.name, err, string(raw))
		}
	}
}
