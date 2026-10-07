// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"reflect"
	"testing"
	"time"
)

type replayJournal struct {
	events []*event.Event
	cause  error
	reads  int
}

func (p *replayJournal) Range(fn func(*event.Event) error) error {
	p.reads++
	for _, ev := range p.events {
		if err := fn(ev); err != nil {
			return err
		}
	}
	return p.cause
}
func unboundedReplay() ReplayInput {
	return ReplayInput{Pattern: ">", Since: -1, SinceTSMS: -1, Until: -1, UntilTSMS: -1}
}
func TestPulseReplayANDHalfOpenSubjectKindCorrelationFilters(t *testing.T) {
	evs := []*event.Event{{Seq: 0, TSUnixMS: 10, Subject: "owned.one", Kind: event.KindWorkflowStarted, CorrelationID: "owned"}, {Seq: 1, TSUnixMS: 20, Subject: "owned.two", Kind: event.KindWorkflowCompleted, CorrelationID: "owned"}, {Seq: 2, TSUnixMS: 30, Subject: "owned.three", Kind: event.KindWorkflowStarted, CorrelationID: "other"}, {Seq: 3, TSUnixMS: 40, Subject: "foreign.one", Kind: event.KindWorkflowStarted, CorrelationID: "owned"}, {Seq: 4, TSUnixMS: 50, Subject: "owned.four", Kind: event.KindWorkflowStarted, CorrelationID: "owned"}}
	cases := []struct {
		name string
		edit func(*ReplayInput)
		want []int64
	}{{"all", func(*ReplayInput) {}, []int64{0, 1, 2, 3, 4}}, {"subject", func(in *ReplayInput) { in.Pattern = "owned.*" }, []int64{0, 1, 2, 4}}, {"exact", func(in *ReplayInput) { in.Pattern = "owned.one" }, []int64{0}}, {"kind", func(in *ReplayInput) { in.Kinds = map[event.Kind]struct{}{event.KindWorkflowStarted: {}} }, []int64{0, 2, 3, 4}}, {"empty kind", func(in *ReplayInput) { in.Kinds = map[event.Kind]struct{}{} }, nil}, {"correlation", func(in *ReplayInput) { in.Correlation = "owned" }, []int64{0, 1, 3, 4}}, {"lower seq", func(in *ReplayInput) { in.Since = 1 }, []int64{1, 2, 3, 4}}, {"lower timestamp", func(in *ReplayInput) { in.SinceTSMS = 20 }, []int64{1, 2, 3, 4}}, {"upper seq", func(in *ReplayInput) { in.Until = 3 }, []int64{0, 1, 2}}, {"upper timestamp", func(in *ReplayInput) { in.UntilTSMS = 40 }, []int64{0, 1, 2}}, {"AND", func(in *ReplayInput) {
		in.Since = 1
		in.SinceTSMS = 30
		in.Until = 5
		in.UntilTSMS = 51
		in.Pattern = "owned.*"
		in.Kinds = map[event.Kind]struct{}{event.KindWorkflowStarted: {}}
		in.Correlation = "owned"
	}, []int64{4}}, {"past head", func(in *ReplayInput) { in.Since = 100 }, nil}, {"empty window", func(in *ReplayInput) { in.Since = 3; in.Until = 3 }, nil}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			j := &replayJournal{events: evs}
			in := unboundedReplay()
			test.edit(&in)
			var got []int64
			out, err := NewReplay(j).Replay(context.Background(), in, func(ev *event.Event) error {
				if ev != evs[ev.Seq] {
					t.Fatal("event copied/replaced", ev)
				}
				got = append(got, ev.Seq)
				return nil
			})
			wantLast := int64(-1)
			if len(test.want) > 0 {
				wantLast = test.want[len(test.want)-1]
			}
			if err != nil || out.LastWritten != wantLast || !reflect.DeepEqual(got, test.want) || j.reads != 1 {
				t.Fatal(out, err, got, test.want, j.reads)
			}
		})
	}
}
func TestPulseReplayRetainsJournalOrderLastSuccessfulSeqAndOriginalCauses(t *testing.T) {
	cause := errors.New("owned write cause")
	rangeCause := errors.New("owned journal cause")
	j := &replayJournal{events: []*event.Event{{Seq: 5, Subject: "owned.one"}, {Seq: 2, Subject: "owned.two"}}}
	seen := []int64{}
	out, err := NewReplay(j).Replay(context.Background(), unboundedReplay(), func(ev *event.Event) error { seen = append(seen, ev.Seq); return nil })
	if err != nil || out.LastWritten != 2 || !reflect.DeepEqual(seen, []int64{5, 2}) {
		t.Fatal(out, err, seen)
	}
	writes := 0
	out, err = NewReplay(j).Replay(context.Background(), unboundedReplay(), func(*event.Event) error {
		writes++
		if writes == 2 {
			return cause
		}
		return nil
	})
	if err != cause || out.LastWritten != 5 || writes != 2 {
		t.Fatal(out, err, writes)
	}
	out, err = NewReplay(j).Replay(context.Background(), unboundedReplay(), func(*event.Event) error { return cause })
	if err != cause || out.LastWritten != -1 {
		t.Fatal(out, err)
	}
	j.cause = rangeCause
	out, err = NewReplay(j).Replay(context.Background(), unboundedReplay(), func(*event.Event) error { return nil })
	if err != rangeCause || out.LastWritten != 2 {
		t.Fatal(out, err)
	}
	j.events = nil
	out, err = NewReplay(j).Replay(context.Background(), unboundedReplay(), func(*event.Event) error { t.Fatal("empty emitted"); return nil })
	if err != rangeCause || out.LastWritten != -1 {
		t.Fatal(out, err)
	}
}
func TestPulseReplayRateWaitHonorsCancellationAndFirstEventIsImmediate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	in := unboundedReplay()
	in.RateEPS = .01
	j := &replayJournal{events: []*event.Event{{Seq: 7, Subject: "owned.one"}, {Seq: 8, Subject: "owned.two"}}}
	writes := 0
	started := time.Now()
	out, err := NewReplay(j).Replay(ctx, in, func(*event.Event) error { writes++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || out.LastWritten != 7 || writes != 1 || time.Since(started) > time.Second {
		t.Fatal(out, err, writes, time.Since(started))
	}
	// Legacy unlimited replay delegates cancellation to the emitter rather than
	// adding a new scan gate. The stream service will own that lifetime policy.
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	writes = 0
	out, err = NewReplay(j).Replay(canceled, unboundedReplay(), func(*event.Event) error { writes++; return nil })
	if err != nil || out.LastWritten != 8 || writes != 2 {
		t.Fatal(out, err, writes)
	}
	in.RateEPS = 1000
	writes = 0
	started = time.Now()
	out, err = NewReplay(j).Replay(context.Background(), in, func(*event.Event) error { writes++; return nil })
	if err != nil || writes != 2 || out.LastWritten != 8 || time.Since(started) < time.Millisecond {
		t.Fatal(out, err, writes, time.Since(started))
	}
}
func TestPulseReplayUsesOwnedActualJournalAndZeroSequence(t *testing.T) {
	now := int64(10)
	j, err := journal.Open(t.TempDir(), journal.Options{Now: func() time.Time { return time.UnixMilli(now) }})
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	for i := 0; i < 3; i++ {
		now = int64(10 * (i + 1))
		if _, err := j.Append(event.Spec{Subject: "owned.event", Kind: event.KindWorkflowStarted, Actor: "fixture", CorrelationID: "owned", Payload: map[string]any{"raw": false}}); err != nil {
			t.Fatal(err)
		}
	}
	in := unboundedReplay()
	in.Since = 0
	in.Until = 2
	in.Correlation = "owned"
	var seen []int64
	out, err := NewReplay(j).Replay(context.Background(), in, func(ev *event.Event) error {
		seen = append(seen, ev.Seq)
		if ev.IsEphemeral() {
			t.Fatal(ev)
		}
		return nil
	})
	if err != nil || out.LastWritten != 1 || !reflect.DeepEqual(seen, []int64{0, 1}) {
		t.Fatal(out, err, seen)
	}
}
