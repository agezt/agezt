// SPDX-License-Identifier: MIT

package pulse

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/warden"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type errorWarden struct{}

func (errorWarden) Run(context.Context, warden.Spec) (*warden.Result, error) {
	return nil, errors.New("runner failed")
}
func (errorWarden) EffectiveProfile(p warden.Profile) warden.Profile { return p }
func (errorWarden) SetBus(*bus.Bus)                                  {}

func TestLogSinkAndIndentCoverage(t *testing.T) {
	if err := (LogSink{}).Deliver(Brief{Title: "ignored"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	sink := LogSink{W: &out}
	if err := sink.Deliver(Brief{Title: "One", Disposition: DispNotify}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Deliver(Brief{Title: "Two", Body: "line 1\nline 2\n", Disposition: DispAlert}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "    line 1\n    line 2") {
		t.Fatalf("body was not indented: %q", out.String())
	}
	if err := (LogSink{W: failingWriter{}}).Deliver(Brief{Title: "fail"}); err == nil {
		t.Fatal("writer failure must surface")
	}
	if err := (LogSink{W: failingWriter{}}).Deliver(Brief{Title: "fail", Body: "body"}); err == nil {
		t.Fatal("body writer failure must surface")
	}
}

func TestObserverNamesAndNoopBranches(t *testing.T) {
	probe := NewProbeObserver("ci", nil, nil, nil)
	if probe.Name() != "probe:ci" {
		t.Fatalf("probe name = %q", probe.Name())
	}
	if deltas, err := probe.Poll(context.Background()); err != nil || deltas != nil {
		t.Fatalf("empty probe: deltas=%v err=%v", deltas, err)
	}
	probe = NewProbeObserver("ci", []string{"test"}, errorWarden{}, nil)
	if _, err := probe.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "probe ci") {
		t.Fatalf("probe runner error = %v", err)
	}

	disk := NewDiskObserver("/", 10, nil)
	if disk.Name() != "system:disk" {
		t.Fatalf("disk name = %q", disk.Name())
	}
	if deltas, err := disk.Poll(context.Background()); err != nil || deltas != nil {
		t.Fatalf("nil disk source: deltas=%v err=%v", deltas, err)
	}
	disk.usage = func(string) (uint64, uint64, error) { return 0, 0, errors.New("disk failed") }
	if _, err := disk.Poll(context.Background()); err == nil {
		t.Fatal("disk source failure must surface")
	}
	disk.usage = func(string) (uint64, uint64, error) { return 0, 0, nil }
	if deltas, err := disk.Poll(context.Background()); err != nil || deltas != nil {
		t.Fatalf("zero-sized disk: deltas=%v err=%v", deltas, err)
	}
}

func TestHealthErrorAndDegradedRunRate(t *testing.T) {
	o := NewHealthObserver(func(context.Context) (HealthStat, error) {
		return HealthStat{}, errors.New("stats unavailable")
	}, 0.3, 5)
	if _, err := o.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "health") {
		t.Fatalf("health source error = %v", err)
	}
	if level := o.assess(HealthStat{Runs: 8, FailedRuns: 2}); level != healthDegraded {
		t.Fatalf("25%% failed runs = %v, want degraded", level)
	}
}

func TestStatusMapPauseResumeAndNilPublish(t *testing.T) {
	now := time.Unix(123, 0)
	e := New(Config{Now: func() time.Time { return now }, Cadence: 10 * time.Second})
	status := e.StatusMap()
	if status["running"] != true || status["paused"] != false || status["cadence_ms"] != int64(10_000) {
		t.Fatalf("unexpected status map: %#v", status)
	}
	if ev, err := e.publish("", "", "", "", nil); err != nil || ev != nil {
		t.Fatalf("nil-bus publish: event=%v err=%v", ev, err)
	}

	e.Pause()
	e.Pause()
	if !e.IsPaused() {
		t.Fatal("engine should be paused")
	}
	e.Resume()
	e.Resume()
	if e.IsPaused() {
		t.Fatal("engine should be resumed")
	}
}

func TestAskQueueTrimsOldestAndSortsNewest(t *testing.T) {
	e := New(Config{})
	for i := 0; i <= maxPendingAsks; i++ {
		e.queueAsk(&pendingAsk{
			IssueKey: string(rune('a' + i)),
			TS:       int64(i),
		})
	}
	asks := e.PendingAsks()
	if len(asks) != maxPendingAsks {
		t.Fatalf("ask count = %d", len(asks))
	}
	if asks[0]["ts_unix_ms"] != int64(maxPendingAsks) {
		t.Fatalf("newest ask not first: %#v", asks[0])
	}
	for _, ask := range asks {
		if ask["ts_unix_ms"] == int64(0) {
			t.Fatal("oldest ask was not trimmed")
		}
	}
}

func TestParseProbeSpecSkipsMalformedSegmentsAndAcceptsCmd(t *testing.T) {
	name, argv, ok := ParseProbeSpec("junk; name = deploy ; cmd = echo ready")
	if !ok || name != "deploy" || strings.Join(argv, " ") != "echo ready" {
		t.Fatalf("parsed name=%q argv=%v ok=%v", name, argv, ok)
	}
}
