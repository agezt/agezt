// SPDX-License-Identifier: MIT

package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
)

// fakeHost is the minimum Host implementation the Manager's unit tests
// need: just the two side effects the Manager cannot perform itself, plus
// a recorder for the bus.Publish calls so the Halt/Resume-with-reason paths
// can be asserted.
type fakeHost struct {
	bus      *fakeBus
	suspendN int
}

type fakeBus struct {
	mu        sync.Mutex
	published []event.Spec
}

func (b *fakeBus) Publish(spec event.Spec) (*event.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, spec)
	return nil, nil
}

func (h *fakeHost) Suspend(reason string) int { h.suspendN++; return 0 }

func (h *fakeHost) PublishBus(spec event.Spec) (*event.Event, error) {
	return h.bus.Publish(spec)
}

// newFixture builds a Manager over a REAL registry and a fake host: the
// registry is the module's own store, so the tests exercise it rather than
// a stand-in for it.
func newFixture() (*Manager, *registry.Registry, *fakeHost) {
	reg := registry.New()
	host := &fakeHost{bus: &fakeBus{}}
	return New(reg, host), reg, host
}

func TestHalt_TogglesAndCancels(t *testing.T) {
	m, reg, host := newFixture()
	// Register a fake run with a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())
	reg.Lock()
	reg.Register("c1", cancel)
	reg.Unlock()
	if m.IsHalted() {
		t.Fatalf("a fresh registry should not be halted")
	}
	m.Halt()
	if !m.IsHalted() {
		t.Fatalf("Halt() did not flip Halted")
	}
	// The registered cancel func should have been invoked.
	if ctx.Err() == nil {
		t.Fatalf("Halt() did not cancel the in-flight run ctx")
	}
	// Second Halt is a no-op.
	m.Halt()
	if len(host.bus.published) != 1 {
		t.Fatalf("second Halt() should not re-publish; got %d", len(host.bus.published))
	}
	// The published event carries the cancelled_runs count and no reason.
	ev := host.bus.published[0]
	if got, want := ev.Subject, "kernel.halt"; got != want {
		t.Errorf("subject = %q, want %q", got, want)
	}
	if pl, ok := ev.Payload.(map[string]any); !ok || pl["cancelled_runs"].(int) != 1 {
		t.Errorf("payload = %+v, want cancelled_runs=1", ev.Payload)
	}
	// The registry is empty after the halt: the sweep took every entry.
	if got := m.ActiveRuns(); got != 0 {
		t.Errorf("ActiveRuns() after Halt = %d, want 0", got)
	}
}

func TestHaltWith_RecordsReason(t *testing.T) {
	m, _, host := newFixture()
	m.HaltWith("rolling deploy")
	ev := host.bus.published[0]
	pl := ev.Payload.(map[string]any)
	if pl["reason"] != "rolling deploy" {
		t.Errorf("reason = %v, want %q", pl["reason"], "rolling deploy")
	}
}

func TestResume_ClearsFlag(t *testing.T) {
	m, _, host := newFixture()
	m.Halt()
	m.Resume()
	if m.IsHalted() {
		t.Fatalf("Resume() did not clear Halted")
	}
	// Resume on an un-halted kernel is a no-op.
	m.Resume()
	if len(host.bus.published) != 2 {
		t.Fatalf("expected exactly 2 events (halt + resume); got %d", len(host.bus.published))
	}
}

func TestResumeWith_RecordsReason(t *testing.T) {
	m, _, host := newFixture()
	m.Halt()
	m.ResumeWith("manual ok")
	ev := host.bus.published[1]
	if ev.Subject != "kernel.resume" {
		t.Errorf("subject = %q, want %q", ev.Subject, "kernel.resume")
	}
	if pl, ok := ev.Payload.(map[string]any); !ok || pl["reason"] != "manual ok" {
		t.Errorf("payload = %+v, want reason=manual ok", ev.Payload)
	}
}

func TestCancelRun_OnlyMatching(t *testing.T) {
	m, reg, _ := newFixture()
	ctx1, c1 := context.WithCancel(context.Background())
	ctx2, c2 := context.WithCancel(context.Background())
	reg.Lock()
	reg.Register("a", c1)
	reg.Register("b", c2)
	reg.Unlock()

	if !m.CancelRun("a") {
		t.Fatalf("CancelRun(a) returned false, want true")
	}
	if ctx1.Err() == nil {
		t.Errorf("CancelRun(a) did not cancel ctx1")
	}
	if ctx2.Err() != nil {
		t.Errorf("CancelRun(a) cancelled ctx2 (only the matching id should be cancelled)")
	}
	// Second CancelRun for the same id is a no-op.
	if m.CancelRun("a") {
		t.Errorf("second CancelRun(a) returned true, want false")
	}
	// Halt is still false: CancelRun does not flip it.
	if m.IsHalted() {
		t.Errorf("CancelRun must not flip Halted")
	}
}

func TestDrainAndHalt_NoTimeout(t *testing.T) {
	m, reg, host := newFixture()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Lock()
	reg.Register("x", func() { _ = ctx }) // dummy cancel
	reg.Unlock()
	timedOut, active := m.DrainAndHalt(0)
	if timedOut {
		t.Errorf("DrainAndHalt(0) returned timedOut=true, want false")
	}
	if active != 0 {
		t.Errorf("DrainAndHalt(0) active = %d, want 0", active)
	}
	if !m.IsHalted() {
		t.Errorf("DrainAndHalt did not flip Halted")
	}
	if host.suspendN != 1 {
		t.Errorf("DrainAndHalt did not call Suspend exactly once; got %d", host.suspendN)
	}
}

func TestDrainAndHalt_WaitsForWaitGroup(t *testing.T) {
	m, reg, _ := newFixture()
	reg.Add(1)
	// Simulate an in-flight run that finishes 50ms after the drain starts.
	finished := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		reg.Done()
		close(finished)
	}()
	timedOut, _ := m.DrainAndHalt(500 * time.Millisecond)
	if timedOut {
		t.Errorf("DrainAndHalt timed out even though the WG was released")
	}
	<-finished
}

func TestActiveRunsAndIDs(t *testing.T) {
	m, reg, _ := newFixture()
	if m.ActiveRuns() != 0 {
		t.Errorf("ActiveRuns() on empty registry = %d, want 0", m.ActiveRuns())
	}
	for _, id := range []string{"c", "a", "b"} {
		_, c := context.WithCancel(context.Background())
		reg.Lock()
		reg.Register(id, c)
		reg.Unlock()
	}
	if got, want := m.ActiveRuns(), 3; got != want {
		t.Errorf("ActiveRuns() = %d, want %d", got, want)
	}
	ids := m.ActiveRunIDs()
	if len(ids) != 3 {
		t.Fatalf("ActiveRunIDs() returned %d ids, want 3", len(ids))
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Errorf("ActiveRunIDs() not sorted: %v", ids)
			break
		}
	}
}

func TestNewCorrelationAndSubject(t *testing.T) {
	m, _, _ := newFixture()
	c1 := m.NewCorrelation()
	c2 := m.NewCorrelation()
	if c1 == c2 {
		t.Errorf("NewCorrelation() returned the same id twice: %q", c1)
	}
	for _, id := range []string{c1, c2} {
		if len(id) < 4 || id[:4] != "run-" {
			t.Errorf("NewCorrelation() = %q, want run- prefix", id)
		}
	}
	if got, want := m.SubjectForRun("abc"), "agent.agent-abc.>"; got != want {
		t.Errorf("SubjectForRun = %q, want %q", got, want)
	}
}
