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

// TestDiskObserver_ThresholdEdges pins the two inclusive disk thresholds at their exact
// edges, which the other disk tests only cross well clear of (5% vs a 10% floor; 2% vs the
// 5% critical line). Mutation testing (M525) showed `freePct < minPct → <=` and
// `freePct < minPct/2 → <=` both survived: free space sitting *exactly* on the floor is
// still OK (not low), and exactly on the half-floor is High, not Critical.
func TestDiskObserver_ThresholdEdges(t *testing.T) {
	// Exactly at minPct (10% free, floor 10) must NOT be low → no transition from the
	// 50% baseline, so no delta. Under `<=` it would wrongly fire disk_low.
	t.Run("exactly at floor is not low", func(t *testing.T) {
		free := []uint64{50, 10} // 50% baseline, then exactly 10% free
		usage := func(string) (uint64, uint64, error) {
			f := free[0]
			free = free[1:]
			return f, 100, nil
		}
		o := NewDiskObserver("/", 10, usage)
		_, _ = o.Poll(context.Background()) // baseline (not low)
		if d, _ := o.Poll(context.Background()); len(d) != 0 {
			t.Fatalf("free%% exactly at the floor must not be low, got %+v", d)
		}
	})

	// A transition into low at exactly minPct/2 (5% free, floor 10) is High, not Critical.
	// Under `<=` it would escalate to Critical.
	t.Run("exactly at half-floor is high not critical", func(t *testing.T) {
		free := []uint64{50, 5} // 50% baseline, then exactly 5% (== minPct/2)
		usage := func(string) (uint64, uint64, error) {
			f := free[0]
			free = free[1:]
			return f, 100, nil
		}
		o := NewDiskObserver("/", 10, usage)
		_, _ = o.Poll(context.Background()) // baseline
		d, _ := o.Poll(context.Background())
		if len(d) != 1 || d[0].Kind != "disk_low" {
			t.Fatalf("crossing to 5%% free should emit disk_low: %+v", d)
		}
		if got := d[0].Severity(); got != SevHigh {
			t.Errorf("free%% exactly at minPct/2 must be High, not %v", got)
		}
	})
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

// TestQuietHours_Active pins QuietHours.Active at every edge of both branches. The only
// prior coverage is one wrap-window check (2am in, noon out), so the entire normal
// (Start<End) branch and the exact hour edges went unpinned — mutation testing (M526)
// left `h >= Start → h > Start` and `h < End → h <= End` alive on BOTH the normal and
// wrap branches. The window is inclusive of Start and exclusive of End.
func TestQuietHours_Active(t *testing.T) {
	cases := []struct {
		name string
		q    QuietHours
		hour int
		want bool
	}{
		{"disabled is never active", QuietHours{Enabled: false, Start: 22, End: 7}, 2, false},
		// Normal window 9..17 (Start < End): inclusive 9, exclusive 17.
		{"normal: at start (inclusive)", QuietHours{Enabled: true, Start: 9, End: 17}, 9, true},
		{"normal: mid", QuietHours{Enabled: true, Start: 9, End: 17}, 16, true},
		{"normal: at end (exclusive)", QuietHours{Enabled: true, Start: 9, End: 17}, 17, false},
		{"normal: before start", QuietHours{Enabled: true, Start: 9, End: 17}, 8, false},
		{"normal: after end", QuietHours{Enabled: true, Start: 9, End: 17}, 20, false},
		// Wrap window 22..7 (Start > End): inclusive 22, exclusive 7.
		{"wrap: at start (inclusive)", QuietHours{Enabled: true, Start: 22, End: 7}, 22, true},
		{"wrap: just after midnight", QuietHours{Enabled: true, Start: 22, End: 7}, 0, true},
		{"wrap: just before end", QuietHours{Enabled: true, Start: 22, End: 7}, 6, true},
		{"wrap: at end (exclusive)", QuietHours{Enabled: true, Start: 22, End: 7}, 7, false},
		{"wrap: daytime gap", QuietHours{Enabled: true, Start: 22, End: 7}, 12, false},
		// Degenerate Start==End → never active.
		{"start==end is never active", QuietHours{Enabled: true, Start: 9, End: 9}, 9, false},
	}
	for _, c := range cases {
		if got := c.q.Active(mustTime(c.hour)); got != c.want {
			t.Errorf("%s: Active(%02d:00) = %v, want %v", c.name, c.hour, got, c.want)
		}
	}
}
