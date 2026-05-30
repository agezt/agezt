// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
)

func mustTime(hour int) time.Time {
	return time.Date(2026, 1, 1, hour, 0, 0, 0, time.UTC)
}

// fakeWarden returns scripted exit codes for the probe observer.
type fakeWarden struct{ exits []int }

func (f *fakeWarden) Run(_ context.Context, _ warden.Spec) (*warden.Result, error) {
	code := 0
	if len(f.exits) > 0 {
		code = f.exits[0]
		f.exits = f.exits[1:]
	}
	return &warden.Result{ExitCode: code}, nil
}
func (f *fakeWarden) EffectiveProfile(p warden.Profile) warden.Profile { return p }
func (f *fakeWarden) SetBus(*bus.Bus)                                  {}

func TestProbeTransitions(t *testing.T) {
	st, _ := state.Open(t.TempDir())
	t.Cleanup(func() { st.Close() })
	// exit sequence: 0 (baseline), 0 (no change), 1 (fail!), 1 (no change), 0 (recover!)
	w := &fakeWarden{exits: []int{0, 0, 1, 1, 0}}
	p := NewProbeObserver("ci", []string{"x"}, w, st)
	ctx := context.Background()

	mustNone := func(label string) {
		d, err := p.Poll(ctx)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if len(d) != 0 {
			t.Fatalf("%s: expected no delta, got %+v", label, d)
		}
	}
	mustKind := func(label, kind string) {
		d, err := p.Poll(ctx)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if len(d) != 1 || d[0].Kind != kind {
			t.Fatalf("%s: expected %s, got %+v", label, kind, d)
		}
	}

	mustNone("baseline")    // first observation establishes baseline
	mustNone("still green") // 0→0
	mustKind("went red", "probe_failed")
	mustNone("still red") // 1→1
	mustKind("recovered", "probe_recovered")
}

func TestProbeFailedSeverityHigh(t *testing.T) {
	st, _ := state.Open(t.TempDir())
	t.Cleanup(func() { st.Close() })
	w := &fakeWarden{exits: []int{0, 1}}
	p := NewProbeObserver("ci", []string{"x"}, w, st)
	_, _ = p.Poll(context.Background()) // baseline
	d, _ := p.Poll(context.Background())
	if len(d) != 1 || d[0].Severity() != SevHigh {
		t.Fatalf("probe failure should be high severity: %+v", d)
	}
}

func TestProbeStatePersistsAcrossInstances(t *testing.T) {
	st, _ := state.Open(t.TempDir())
	t.Cleanup(func() { st.Close() })
	// First instance sees green.
	p1 := NewProbeObserver("ci", []string{"x"}, &fakeWarden{exits: []int{0}}, st)
	_, _ = p1.Poll(context.Background())
	// A fresh instance (simulating restart) sees red → should detect the
	// transition using persisted state, not treat it as a baseline.
	p2 := NewProbeObserver("ci", []string{"x"}, &fakeWarden{exits: []int{1}}, st)
	d, _ := p2.Poll(context.Background())
	if len(d) != 1 || d[0].Kind != "probe_failed" {
		t.Fatalf("probe state should persist across instances: %+v", d)
	}
}

func TestDiskObserverTransitions(t *testing.T) {
	// 50% free, then 5% free (< 10 threshold) → disk_low, then 50% → recovered.
	free := []uint64{50, 5, 50}
	usage := func(string) (uint64, uint64, error) {
		f := free[0]
		free = free[1:]
		return f, 100, nil
	}
	o := NewDiskObserver("/", 10, usage)
	ctx := context.Background()

	if d, _ := o.Poll(ctx); len(d) != 0 {
		t.Fatal("baseline should not emit")
	}
	d, _ := o.Poll(ctx)
	if len(d) != 1 || d[0].Kind != "disk_low" {
		t.Fatalf("crossing below threshold should emit disk_low: %+v", d)
	}
	d, _ = o.Poll(ctx)
	if len(d) != 1 || d[0].Kind != "disk_recovered" {
		t.Fatalf("crossing back above should emit disk_recovered: %+v", d)
	}
}

func TestDiskObserverCriticalWhenVeryLow(t *testing.T) {
	free := []uint64{50, 2} // 2% < 10/2=5 → critical
	usage := func(string) (uint64, uint64, error) {
		f := free[0]
		free = free[1:]
		return f, 100, nil
	}
	o := NewDiskObserver("/", 10, usage)
	_, _ = o.Poll(context.Background()) // baseline
	d, _ := o.Poll(context.Background())
	if len(d) != 1 || d[0].Severity() != SevCritical {
		t.Fatalf("very-low disk should be critical: %+v", d)
	}
}

func TestParseProbeSpec(t *testing.T) {
	name, argv, ok := ParseProbeSpec("name=ci;argv=make test")
	if !ok || name != "ci" || len(argv) != 2 || argv[0] != "make" {
		t.Fatalf("parse failed: name=%q argv=%v ok=%v", name, argv, ok)
	}
	if _, _, ok := ParseProbeSpec("argv=only-no-name"); ok {
		t.Fatal("missing name should fail")
	}
	if _, _, ok := ParseProbeSpec("name=x"); ok {
		t.Fatal("missing argv should fail")
	}
}

func TestParseQuietHours(t *testing.T) {
	q := ParseQuietHours("22-7")
	if !q.Enabled || q.Start != 22 || q.End != 7 {
		t.Fatalf("bad parse: %+v", q)
	}
	if !q.Active(mustTime(2)) || q.Active(mustTime(12)) {
		t.Fatal("wrap-around window 22-7 should include 2am, exclude noon")
	}
	if ParseQuietHours("garbage").Enabled {
		t.Fatal("garbage should disable")
	}
}
