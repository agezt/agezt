// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"encoding/json"
	core "github.com/agezt/agezt/kernel/pulse"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"testing"
	"time"
)

type controlPort struct {
	calls                   []string
	status                  map[string]any
	asks                    []map[string]any
	found, acted            bool
	key                     string
	approve                 bool
	requested, applied      time.Duration
	dial, quiet, removed    string
	flushCount, removeCount int
}

func (p *controlPort) StatusMap() map[string]any {
	p.calls = append(p.calls, "status")
	return p.status
}
func (p *controlPort) Pause()  { p.calls = append(p.calls, "pause") }
func (p *controlPort) Resume() { p.calls = append(p.calls, "resume") }
func (p *controlPort) Beat()   { p.calls = append(p.calls, "beat") }
func (p *controlPort) SetCadence(d time.Duration) time.Duration {
	p.calls = append(p.calls, "cadence")
	p.requested = d
	return p.applied
}
func (p *controlPort) SetDial(v string) string {
	p.calls = append(p.calls, "dial")
	p.dial = v
	return "applied-dial"
}
func (p *controlPort) SetQuietHours(v string) string {
	p.calls = append(p.calls, "quiet")
	p.quiet = v
	return "applied-quiet"
}
func (p *controlPort) FlushDigest() int { p.calls = append(p.calls, "flush"); return p.flushCount }
func (p *controlPort) RemoveObserver(v string) int {
	p.calls = append(p.calls, "unwatch")
	p.removed = v
	return p.removeCount
}
func (p *controlPort) PendingAsks() []map[string]any {
	p.calls = append(p.calls, "asks")
	return p.asks
}
func (p *controlPort) ResolveAsk(key string, approve bool) (bool, bool) {
	p.calls = append(p.calls, "resolve")
	p.key = key
	p.approve = approve
	return p.found, p.acted
}

type observerPort struct {
	calls      []string
	path, name string
	pct        float64
	argv       []string
	ok         bool
}

func (p *observerPort) AddDiskObserver(path string, pct float64) (string, bool) {
	p.calls = append(p.calls, "disk")
	p.path = path
	p.pct = pct
	return "disk-result", p.ok
}
func (p *observerPort) AddProbeObserver(name string, argv []string) (string, bool) {
	p.calls = append(p.calls, "probe")
	p.name = name
	p.argv = argv
	return "probe-result", p.ok
}
func object(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestPulseDisabledAndUnavailableControlsKeepSafeReadAndErrorShapes(t *testing.T) {
	s := NewControls(nil, nil, func(string, string) { t.Fatal("disabled setting write") })
	if s.Available() || s.WatchesAvailable() {
		t.Fatal("disabled available")
	}
	status, err := s.Status(context.Background(), StatusInput{})
	if err != nil || !reflect.DeepEqual(object(t, status), map[string]any{"enabled": false}) {
		t.Fatal(status, err)
	}
	asks, err := s.Asks(context.Background(), AsksInput{})
	if err != nil || asks.Asks == nil || len(asks.Asks) != 0 {
		t.Fatal(asks, err)
	}
	failures := []func() error{func() error { _, err := s.Pause(context.Background(), PauseInput{}); return err }, func() error { _, err := s.Resume(context.Background(), PauseInput{}); return err }, func() error { _, err := s.Beat(context.Background(), BeatInput{}); return err }, func() error { _, err := s.Resolve(context.Background(), ResolveInput{}); return err }, func() error { _, err := s.Cadence(context.Background(), CadenceInput{}); return err }, func() error { _, err := s.Dial(context.Background(), DialInput{}); return err }, func() error { _, err := s.Quiet(context.Background(), QuietInput{}); return err }, func() error { _, err := s.Flush(context.Background(), FlushInput{}); return err }, func() error { _, err := s.Unwatch(context.Background(), UnwatchInput{}); return err }}
	for _, call := range failures {
		if err := call(); err != ErrDisabled {
			t.Fatal(err)
		}
	}
	if _, err := s.Watch(context.Background(), WatchInput{}); err != ErrWatchesUnavailable {
		t.Fatal(err)
	}
	if _, err := s.Probe(context.Background(), ProbeInput{}); err != ErrWatchesUnavailable {
		t.Fatal(err)
	}
}
func TestPulseControlsRetainDynamicStatusAsksAndRequiredFalseZeroResults(t *testing.T) {
	p := &controlPort{status: map[string]any{"enabled": false, "opaque": map[string]any{"raw": true}}, found: true}
	s := NewControls(p, nil, nil)
	status, err := s.Status(context.Background(), StatusInput{})
	if err != nil || status["enabled"] != true || p.status["enabled"] != true || !reflect.DeepEqual(status["opaque"], map[string]any{"raw": true}) {
		t.Fatal(status, err, p.status)
	}
	asks, err := s.Asks(context.Background(), AsksInput{})
	if err != nil || object(t, asks)["asks"] != nil {
		t.Fatal(asks, err)
	}
	p.asks = []map[string]any{{"issue_key": "owned", "raw": false}}
	asks, err = s.Asks(context.Background(), AsksInput{})
	if err != nil || !reflect.DeepEqual(asks.Asks, p.asks) {
		t.Fatal(asks, err)
	}
	pause, err := s.Pause(context.Background(), PauseInput{})
	if err != nil || !pause.Paused {
		t.Fatal(pause, err)
	}
	pause, err = s.Resume(context.Background(), PauseInput{})
	if err != nil || object(t, pause)["paused"] != false {
		t.Fatal(pause, err)
	}
	beat, err := s.Beat(context.Background(), BeatInput{})
	if err != nil || !beat.Triggered {
		t.Fatal(beat, err)
	}
	resolved, err := s.Resolve(context.Background(), ResolveInput{IssueKey: " raw key ", Approve: false})
	if err != nil || !reflect.DeepEqual(object(t, resolved), map[string]any{"resolved": true, "approved": false, "acted": false}) || p.key != " raw key " || p.approve {
		t.Fatal(resolved, err, p)
	}
	p.found = false
	resolved, err = s.Resolve(context.Background(), ResolveInput{IssueKey: "missing", Approve: true})
	if err != ErrPendingAskMissing || !reflect.DeepEqual(resolved, ResolveOutput{}) || !p.approve {
		t.Fatal(resolved, err, p)
	}
	flush, err := s.Flush(context.Background(), FlushInput{})
	if err != nil || object(t, flush)["flushed"] != float64(0) {
		t.Fatal(flush, err)
	}
	removed, err := s.Unwatch(context.Background(), UnwatchInput{Name: " raw name "})
	if err != nil || object(t, removed)["removed"] != float64(0) || p.removed != " raw name " {
		t.Fatal(removed, err, p)
	}
}
func TestPulseSettingsUseAppliedValuesAfterLiveChange(t *testing.T) {
	p := &controlPort{applied: 5 * time.Second}
	settings := [][2]string{}
	s := NewControls(p, nil, func(key, value string) {
		settings = append(settings, [2]string{key, value})
		p.calls = append(p.calls, "persist")
	})
	out, err := s.Cadence(context.Background(), CadenceInput{Seconds: .1234567899})
	if err != nil || out.CadenceMS != 5000 || p.requested != 123456789*time.Nanosecond || !reflect.DeepEqual(p.calls, []string{"cadence", "persist"}) || !reflect.DeepEqual(settings, [][2]string{{"AGEZT_PULSE_CADENCE", "5s"}}) {
		t.Fatal(out, err, p, settings)
	}
	before := len(p.calls)
	for _, secs := range []float64{0, -1} {
		if _, err := s.Cadence(context.Background(), CadenceInput{Seconds: secs}); err == nil || len(p.calls) != before {
			t.Fatal(err, p.calls)
		}
	}
	dial, err := s.Dial(context.Background(), DialInput{Dial: " raw dial "})
	if err != nil || dial.Dial != "applied-dial" || p.dial != " raw dial " || settings[1] != ([2]string{"AGEZT_PULSE_DIAL", "applied-dial"}) {
		t.Fatal(dial, err, p, settings)
	}
	quiet, err := s.Quiet(context.Background(), QuietInput{Hours: " raw hours "})
	if err != nil || quiet.Quiet != "applied-quiet" || p.quiet != " raw hours " || settings[2] != ([2]string{"AGEZT_PULSE_QUIET_HOURS", "applied-quiet"}) {
		t.Fatal(quiet, err, p, settings)
	}
}
func TestPulseObserversRetainAvailabilityBoundsNamesAndFieldSplit(t *testing.T) {
	p := &observerPort{ok: true}
	s := NewControls(nil, p, nil)
	out, err := s.Watch(context.Background(), WatchInput{Path: " raw path ", MinPct: 17.25})
	if err != nil || out.Observer != "disk-result" || !out.Added || p.path != " raw path " || p.pct != 17.25 {
		t.Fatal(out, err, p)
	}
	for _, pct := range []float64{-1, 0, 100, 101} {
		before := len(p.calls)
		if _, err := s.Watch(context.Background(), WatchInput{Path: "owned", MinPct: pct}); err == nil || len(p.calls) != before {
			t.Fatal(err, p)
		}
	}
	out, err = s.Probe(context.Background(), ProbeInput{Name: " raw name ", Command: "  echo  'two words' \t --flag "})
	if err != nil || out.Observer != "probe-result" || !out.Added || p.name != " raw name " || !reflect.DeepEqual(p.argv, []string{"echo", "'two", "words'", "--flag"}) {
		t.Fatal(out, err, p)
	}
	for _, input := range []ProbeInput{{Name: "", Command: "echo"}, {Name: "owned", Command: " "}} {
		before := len(p.calls)
		if _, err := s.Probe(context.Background(), input); err == nil || len(p.calls) != before {
			t.Fatal(err, p)
		}
	}
	p.ok = false
	if out, err := s.Watch(context.Background(), WatchInput{Path: "owned", MinPct: 10}); err == nil || err.Error() != "could not add the watch" || !reflect.DeepEqual(out, ObserverOutput{}) {
		t.Fatal(out, err)
	}
	if out, err := s.Probe(context.Background(), ProbeInput{Name: "owned", Command: "echo"}); err == nil || err.Error() != "could not add the probe" || !reflect.DeepEqual(out, ObserverOutput{}) {
		t.Fatal(out, err)
	}
}
func TestPulseControlsUseOwnedUnstartedActualEngineAndSettingsPort(t *testing.T) {
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	engine := core.New(core.Config{Bus: k.Bus()})
	settings := map[string]string{}
	s := NewControls(engine, nil, func(key, value string) { settings[key] = value })
	if out, err := s.Pause(context.Background(), PauseInput{}); err != nil || !out.Paused || !engine.IsPaused() {
		t.Fatal(out, err)
	}
	if out, err := s.Resume(context.Background(), PauseInput{}); err != nil || out.Paused || engine.IsPaused() {
		t.Fatal(out, err)
	}
	if out, err := s.Cadence(context.Background(), CadenceInput{Seconds: 1}); err != nil || out.CadenceMS != 5000 || settings["AGEZT_PULSE_CADENCE"] != "5s" {
		t.Fatal(out, err, settings)
	}
	if out, err := s.Dial(context.Background(), DialInput{Dial: "unknown"}); err != nil || out.Dial != "balanced" || settings["AGEZT_PULSE_DIAL"] != "balanced" {
		t.Fatal(out, err, settings)
	}
	if out, err := s.Quiet(context.Background(), QuietInput{Hours: "22-7"}); err != nil || out.Quiet != "22-7" || settings["AGEZT_PULSE_QUIET_HOURS"] != "22-7" {
		t.Fatal(out, err, settings)
	}
	if out, err := s.Beat(context.Background(), BeatInput{}); err != nil || !out.Triggered {
		t.Fatal(out, err)
	}
	if provider.CallCount() != 0 {
		t.Fatal(provider.CallCount())
	}
}
