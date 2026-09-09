// SPDX-License-Identifier: MIT

package lifecycle

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

// Manager owns the run-lifecycle surface (Halt/Resume/Cancel/Drain
// + read-side queries). It delegates to the host kernel via the
// KernelAPI interface so this sub-package does not import
// kernel/runtime — the dependency arrow is one-way:
//
//   kernel/runtime  →  kernel/runtime/lifecycle
//
// Construction: Manager is a stateless value around the host
// kernel; one Manager per *Kernel, initialised in Open() and
// shared for the kernel's whole lifetime.
type Manager struct {
	k KernelAPI
}

// New constructs the Manager. k is the host kernel (must implement
// KernelAPI). The Manager does not need a separate bus reference —
// it publishes via k.PublishBus.
func New(k KernelAPI) *Manager {
	return &Manager{k: k}
}

// IsHalted reports whether Run will refuse to start.
func (m *Manager) IsHalted() bool {
	mu := m.k.RunsMu()
	mu.Lock()
	defer mu.Unlock()
	return m.k.Halted()
}

// Halt cancels every in-flight run and prevents new ones. It emits a
// `halt` event to the journal so the action is auditable. Equivalent
// to HaltWith("") for callers that have no reason to record.
func (m *Manager) Halt() { m.HaltWith("") }

// HaltWith is Halt plus a free-text reason that the operator (or
// upstream automation) gave when issuing the halt. The reason is
// journaled on the kernel.halt event so postmortems can answer
// "why was the daemon halted at 14:32?". Empty reason is fine and
// rendered as omitted in the payload.
func (m *Manager) HaltWith(reason string) {
	mu := m.k.RunsMu()
	mu.Lock()
	if m.k.Halted() {
		mu.Unlock()
		return
	}
	m.k.SetHalted(true)
	cancels := make([]context.CancelFunc, 0, len(m.k.Runs()))
	for _, c := range m.k.Runs() {
		cancels = append(cancels, c)
	}
	// Drop every entry from the live registry. We cannot reassign
	// the map (interface-implemented field), but delete-everything
	// has the same effect: the next RunWith registration is the
	// only writer of new entries, and it does not depend on
	// pre-existing entries being absent.
	for corr := range m.k.Runs() {
		delete(m.k.Runs(), corr)
	}
	mu.Unlock()
	for _, c := range cancels {
		c()
	}
	payload := map[string]any{"cancelled_runs": len(cancels)}
	if reason != "" {
		payload["reason"] = reason
	}
	_, _ = m.k.PublishBus(event.Spec{
		Subject: "kernel.halt",
		Kind:    event.KindHalt,
		Actor:   "kernel",
		Payload: payload,
	})
}

// DrainAndHalt cancels all in-flight runs (equivalent to Halt()) and
// waits for them to unwind. It is the drain-phase primitive used by
// Close and by the self-update engine (M860). The timeout caps how
// long it waits; if exceeded, the function returns true (timedOut)
// with remaining runs still counted. A timeout of zero skips the
// drain wait entirely (cancels runs but does not wait).
//
// Use this instead of Halt() when the caller needs to know whether
// the drain completed within the timeout, e.g. for update vs.
// shutdown decisions.
func (m *Manager) DrainAndHalt(timeout time.Duration) (timedOut bool, activeRuns int) {
	m.k.Suspend("drain") // M1002: classify in-flight runs as resumable BEFORE cancelling them
	m.Halt()             // cancel and mark halted; no-op if already halted
	mu := m.k.RunsMu()
	mu.Lock()
	activeRuns = len(m.k.Runs())
	mu.Unlock()
	if timeout <= 0 {
		return false, activeRuns
	}
	settled := make(chan struct{})
	go func() {
		m.k.RunWG().Wait()
		close(settled)
	}()
	t := time.NewTimer(timeout)
	select {
	case <-settled:
		t.Stop()
		return false, 0
	case <-t.C:
		return true, activeRuns
	}
}

// CancelRun cancels a single in-flight run by correlation id, leaving
// the kernel un-halted and every other run untouched (M32). Returns
// true if a matching live run was found and cancelled, false if there
// is no such active run (already finished, never existed, or wrong
// id).
func (m *Manager) CancelRun(corr string) bool {
	mu := m.k.RunsMu()
	mu.Lock()
	runs := m.k.Runs()
	cancel, ok := runs[corr]
	if ok {
		delete(runs, corr)
	}
	mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

// Resume clears the halt flag, allowing new runs. Already-cancelled
// runs stay cancelled; only future Run calls will succeed.
// Equivalent to ResumeWith("").
func (m *Manager) Resume() { m.ResumeWith("") }

// ResumeWith is Resume plus a free-text reason recorded on the
// kernel.resume event. Symmetric with HaltWith for postmortem
// reconstruction.
func (m *Manager) ResumeWith(reason string) {
	mu := m.k.RunsMu()
	mu.Lock()
	if !m.k.Halted() {
		mu.Unlock()
		return
	}
	m.k.SetHalted(false)
	mu.Unlock()
	var payload any
	if reason != "" {
		payload = map[string]any{"reason": reason}
	}
	_, _ = m.k.PublishBus(event.Spec{
		Subject: "kernel.resume",
		Kind:    event.KindResume,
		Actor:   "kernel",
		Payload: payload,
	})
}

// ActiveRuns returns the number of runs currently registered with
// the kernel. Takes the same mutex as Halt, so it is safe under
// concurrent run starts/completes.
func (m *Manager) ActiveRuns() int {
	mu := m.k.RunsMu()
	mu.Lock()
	defer mu.Unlock()
	return len(m.k.Runs())
}

// ActiveRunIDs returns the correlation ids of the runs in flight
// right now — the live keys of the cancel registry, sorted for
// determinism. Safe under concurrent run starts/completes.
func (m *Manager) ActiveRunIDs() []string {
	mu := m.k.RunsMu()
	mu.Lock()
	defer mu.Unlock()
	ids := make([]string, 0, len(m.k.Runs()))
	for corr := range m.k.Runs() {
		ids = append(ids, corr)
	}
	// Insertion sort — small slices, avoids importing sort just for one call.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j-1] > ids[j]; j-- {
			ids[j-1], ids[j] = ids[j], ids[j-1]
		}
	}
	return ids
}

// NewCorrelation mints a fresh correlation ID suitable for the run
// loop. Useful for callers (e.g. the control plane) that want to
// subscribe to the per-run event subject *before* starting the run.
// Implemented here so the format is co-located with the rest of
// the lifecycle surface; the actual ULID generation is delegated
// to kernel/ulid.
func (m *Manager) NewCorrelation() string { return "run-" + ulid.New() }

// SubjectForRun returns the bus subject pattern that matches every
// event emitted by the agent.Run identified by corr. Use with
// bus.Subscribe to stream a single run's events without seeing
// others.
func (m *Manager) SubjectForRun(corr string) string { return "agent.agent-" + corr + ".>" }
