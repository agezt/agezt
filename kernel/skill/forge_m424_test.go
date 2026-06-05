// SPDX-License-Identifier: MIT

package skill

import (
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/journal"
)

// TestRevertDoesNotActivateDraftParent: Revert must respect the state machine — a
// DRAFT lineage parent must NOT be force-activated (that would skip the shadow gate),
// so reverting a child whose only parent is a draft restores nothing (M424).
func TestRevertDoesNotActivateDraftParent(t *testing.T) {
	f, _ := newTestForge(t)

	// v1 stays a DRAFT (never promoted).
	v1, _, _ := f.Create("c", CreateSpec{Name: "deploy", Body: "v1 steps"})
	if got, _, _ := f.Get(v1.ID); got.Status != StatusDraft {
		t.Fatalf("v1 should be a draft, got %s", got.Status)
	}

	// v2: same name → lineage includes the draft v1; promote v2 to active.
	v2, _, _ := f.Create("c", CreateSpec{Name: "deploy", Body: "v2 steps"})
	_, _ = f.Promote("c", v2.ID)
	_, _ = f.Promote("c", v2.ID) // active

	restored, err := f.Revert("c", v2.ID)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}
	if restored != "" {
		t.Errorf("a draft parent must not be restored to active; restored=%q", restored)
	}
	if got, _, _ := f.Get(v1.ID); got.Status != StatusDraft {
		t.Errorf("draft parent must stay draft after revert (shadow gate not skipped), got %s", got.Status)
	}
}

// skillRMWProbe wraps a skill Store and tracks the max number of overlapping
// Get→Put windows (concurrent mutators). With the Forge mutex held across each
// mutator's Get→Put, the max stays 1; without it, concurrent runs overlap (M424).
type skillRMWProbe struct {
	Store
	mu       sync.Mutex
	inFlight int
	maxConc  int
}

func (p *skillRMWProbe) Get(id string) (Skill, bool, error) {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.maxConc {
		p.maxConc = p.inFlight
	}
	p.mu.Unlock()
	time.Sleep(2 * time.Millisecond) // widen the RMW window
	return p.Store.Get(id)
}

func (p *skillRMWProbe) Put(s Skill) error {
	err := p.Store.Put(s)
	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()
	return err
}

func (p *skillRMWProbe) maxConcurrent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxConc
}

// reset zeroes the counters — used after setup (Create does a standalone read-after-
// write whose unpaired Get would otherwise inflate the baseline) so the measurement
// covers only the concurrent phase under test.
func (p *skillRMWProbe) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inFlight = 0
	p.maxConc = 0
}

// TestForge_SerializesConcurrentOutcomes: the Forge must hold a lock across each
// mutator's Get→Put so concurrent runs (each calls RecordOutcome on the shared
// Forge) can't interleave and lose a metric update — or resurrect a quarantined
// skill by writing back a stale snapshot (M424). Verified structurally: no two RMW
// windows overlap with the lock held.
func TestForge_SerializesConcurrentOutcomes(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	b := bus.New(j)
	t.Cleanup(func() { b.Close(); j.Close() })
	base, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("skill.Open: %v", err)
	}
	probe := &skillRMWProbe{Store: base}
	f := NewForge(probe, b)
	f.now = func() time.Time { return fixedNow }

	sk, _, _ := f.Create("c", CreateSpec{Name: "s", Body: "b"})
	probe.reset() // measure only the concurrent RecordOutcome phase

	const N = 8
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.RecordOutcome("c", []string{sk.ID}, true)
		}()
	}
	wg.Wait()

	if got := probe.maxConcurrent(); got != 1 {
		t.Errorf("overlapping read-modify-write windows (maxConcurrent=%d, want 1): the Forge lock must serialize writes", got)
	}
}
