// SPDX-License-Identifier: MIT

package skill_test

import (
	"context"
	"encoding/json"
	"errors"
	appskill "github.com/agezt/agezt/kernel/app/skill"
	"github.com/agezt/agezt/kernel/event"
	curated "github.com/agezt/agezt/kernel/skill"
	"reflect"
	"testing"
	"time"
)

type observationReader struct {
	events []event.Event
	cause  error
}

func (r observationReader) Range(fn func(*event.Event) error) error {
	for i := range r.events {
		if err := fn(&r.events[i]); err != nil {
			return err
		}
	}
	return r.cause
}

type observationStore struct {
	skill   curated.Skill
	found   bool
	cause   error
	bundles *curated.BundleStore
	report  curated.HygieneReport
	cutoff  int64
	id      string
}

func (s *observationStore) Get(id string) (curated.Skill, bool, error) {
	s.id = id
	return s.skill, s.found, s.cause
}
func (s *observationStore) Bundles() *curated.BundleStore { return s.bundles }
func (s *observationStore) Hygiene(cutoff int64) (curated.HygieneReport, error) {
	s.cutoff = cutoff
	return s.report, s.cause
}
func TestSkillHistoryPreservesChronologyFilteringAndMalformedRows(t *testing.T) {
	rows := []event.Event{{Seq: 1, ID: "one", Kind: event.KindSkillCreated, TSUnixMS: 100, Payload: json.RawMessage(`{"id":"owned","name":"fixture"}`)}, {Seq: 2, ID: "two", Kind: event.KindSkillReverted, CorrelationID: "corr", TSUnixMS: 101, Payload: json.RawMessage(`{"id":"child","restored":"owned"}`)}, {Seq: 3, Kind: event.KindSkillPromoted, Payload: json.RawMessage(`{"id":"other"}`)}, {Seq: 4, Kind: event.KindSkillActivated, Payload: json.RawMessage(`{broken`)}, {Seq: 5, Kind: event.KindOpCompleted, Payload: json.RawMessage(`{"id":"owned"}`)}, {Seq: 6, Kind: event.KindSkillShared, Payload: json.RawMessage(`{"id":"owned"}`)}, {Seq: 7, Kind: event.KindSkillReassigned, Payload: json.RawMessage(`{"id":"owned"}`)}}
	s := appskill.NewObservations(nil, observationReader{events: rows})
	out, err := s.History(context.Background(), appskill.GetInput{ID: "owned"})
	if err != nil || out.ID != "owned" || out.Count != 4 || len(out.Events) != 4 || out.Events[0].Seq != 1 || out.Events[0].ID != "one" || out.Events[1].Seq != 2 || out.Events[1].CorrelationID != "corr" || out.Events[1].Payload["restored"] != "owned" || out.Events[2].Seq != 6 || out.Events[2].Kind != event.KindSkillShared || out.Events[3].Seq != 7 || out.Events[3].Kind != event.KindSkillReassigned {
		t.Fatalf("history=%+v err=%v", out, err)
	}
	raw, err := json.Marshal(out.Events[0])
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if corr, ok := fields["correlation_id"]; !ok || corr != "" {
		t.Fatalf("empty correlation=%s", raw)
	}
	s = appskill.NewObservations(nil, observationReader{})
	empty, err := s.History(context.Background(), appskill.GetInput{ID: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"id":"missing","events":null,"count":0}` {
		t.Fatalf("empty history=%s", raw)
	}
}
func TestSkillFilesRetainLiveManifestFallbackAndByteContent(t *testing.T) {
	ctx := context.Background()
	store := &observationStore{found: true, skill: curated.Skill{ID: "owned", Name: "fixture", Resources: []string{"stale.md"}}}
	s := appskill.NewObservations(store, nil)
	fallback, err := s.Files(ctx, appskill.GetInput{ID: "owned"})
	if err != nil || fallback.ID != "owned" || fallback.Name != "fixture" || fallback.Dir != "" || fallback.Count != 1 || !reflect.DeepEqual(fallback.Files, []string{"stale.md"}) {
		t.Fatalf("fallback=%+v err=%v", fallback, err)
	}
	if _, err := s.ReadFile(ctx, appskill.ReadFileInput{ID: "owned", Path: "ref.md"}); err == nil || err.Error() != "skill bundles are not available on this daemon" {
		t.Fatalf("unavailable bundles=%v", err)
	}
	bundles, err := curated.OpenBundles(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.bundles = bundles
	files, err := s.Files(ctx, appskill.GetInput{ID: "owned"})
	if err != nil || files.Count != 1 || files.Dir != bundles.Dir("fixture") || !reflect.DeepEqual(files.Files, []string{"stale.md"}) {
		t.Fatalf("missing disk fallback=%+v err=%v", files, err)
	}
	content := "owned örnek 🧪"
	if _, err := bundles.Write("fixture", map[string][]byte{"refs/guide.md": []byte(content)}); err != nil {
		t.Fatal(err)
	}
	files, err = s.Files(ctx, appskill.GetInput{ID: "owned"})
	if err != nil || files.Count != 1 || !reflect.DeepEqual(files.Files, []string{"refs/guide.md"}) {
		t.Fatalf("disk truth=%+v err=%v", files, err)
	}
	read, err := s.ReadFile(ctx, appskill.ReadFileInput{ID: "owned", Path: "refs/guide.md"})
	if err != nil || read.ID != "owned" || read.Name != "fixture" || read.Path != "refs/guide.md" || read.Content != content || read.Bytes != len([]byte(content)) {
		t.Fatalf("read=%+v err=%v", read, err)
	}
	if _, err := s.ReadFile(ctx, appskill.ReadFileInput{ID: "owned", Path: "absent.md"}); err == nil {
		t.Fatal("missing resource error swallowed")
	}
	store.found = false
	if _, err := s.Files(ctx, appskill.GetInput{ID: "missing"}); err == nil || err.Error() != "no skill with id missing" {
		t.Fatalf("missing skill=%v", err)
	}
	if _, err := s.ReadFile(ctx, appskill.ReadFileInput{ID: "missing", Path: "ref.md"}); err == nil || err.Error() != "no skill with id missing" {
		t.Fatalf("missing read skill=%v", err)
	}
}
func TestSkillHygieneRetainsDefaultsProjectionAndStoreCauses(t *testing.T) {
	store := &observationStore{report: curated.HygieneReport{Total: 3, Active: 2, Idle: []curated.Skill{{ID: "idle", Name: "fixture", Status: curated.StatusActive, Body: "body", Metrics: curated.Metrics{Uses: 4, LastUsedMS: 123}}}}}
	s := appskill.NewObservations(store, nil)
	ctx := context.Background()
	for _, days := range []int{0, -1, 1, 30} {
		before := time.Now()
		out, err := s.Hygiene(ctx, appskill.HygieneInput{IdleDays: days})
		after := time.Now()
		want := days
		if want <= 0 {
			want = 30
		}
		low := before.Add(-time.Duration(want) * 24 * time.Hour).UnixMilli()
		high := after.Add(-time.Duration(want) * 24 * time.Hour).UnixMilli()
		if err != nil || out.IdleDays != want || out.Total != 3 || out.Active != 2 || out.IdleCount != 1 || len(out.Idle) != 1 || out.Idle[0].ID != "idle" || out.Idle[0].Uses != 4 || out.Idle[0].LastUsedMS != 123 || out.Idle[0].Body != "body" || store.cutoff < low || store.cutoff > high {
			t.Fatalf("hygiene=%+v cutoff=%d want %d..%d err=%v", out, store.cutoff, low, high, err)
		}
	}
	store.report = curated.HygieneReport{}
	out, err := s.Hygiene(ctx, appskill.HygieneInput{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"idle_days":30,"total":0,"active":0,"idle":[],"idle_count":0}` {
		t.Fatalf("empty hygiene=%s", raw)
	}
	cause := errors.New("owned observation failure")
	store.cause = cause
	for _, run := range []func() error{func() error { _, err := s.Files(ctx, appskill.GetInput{ID: "fixture"}); return err }, func() error {
		_, err := s.ReadFile(ctx, appskill.ReadFileInput{ID: "fixture", Path: "ref.md"})
		return err
	}, func() error { _, err := s.Hygiene(ctx, appskill.HygieneInput{}); return err }} {
		if err := run(); !errors.Is(err, cause) {
			t.Fatalf("store cause=%v", err)
		}
	}
}
