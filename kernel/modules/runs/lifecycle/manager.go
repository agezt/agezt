// SPDX-License-Identifier: MIT

package lifecycle

import (
	"sort"
	"time"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/runs/registry"
	"github.com/agezt/agezt/kernel/ulid"
)

// Manager owns the run-lifecycle surface (Halt/Resume/Cancel/Drain
// + read-side queries) over the runs module's live-run registry.
//
// It holds the registry directly: the registry is the module's own data, so
// the Manager does not reach back to the host for it. The two side effects
// it cannot perform itself — suspending the daemon and publishing to the
// bus — come through the Host interface.
type Manager struct {
	reg  *registry.Registry
	host Host
}

// New constructs the Manager over a registry and its host.
func New(reg *registry.Registry, host Host) *Manager {
	return &Manager{reg: reg, host: host}
}

// IsHalted reports whether Run will refuse to start.
func (m *Manager) IsHalted() bool {
	m.reg.Lock()
	defer m.reg.Unlock()
	return m.reg.Halted()
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
	m.reg.Lock()
	if m.reg.Halted() {
		m.reg.Unlock()
		return
	}
	// HaltAll latches the halt flag, takes every live run's cancel and
	// clears the registry — all under the registry's lock. The cancels are
	// invoked AFTER the lock is released: cancelling a run under the lock
	// would run that run's teardown while the registry is held.
	cancels := m.reg.HaltAll()
	m.reg.Unlock()
	for _, c := range cancels {
		c()
	}
	payload := map[string]any{"cancelled_runs": len(cancels)}
	if reason != "" {
		payload["reason"] = reason
	}
	_, _ = m.host.PublishBus(event.Spec{
		Subject: "kernel.halt",
		Kind:    event.KindHalt,
		Actor:   "kernel",
		Payload: payload,
	})
}

// DrainAndHalt cancels all in-flight runs (equivalent to Halt()) and
// waits for them to unwind. This is the drain-phase primitive used by
// Close and by the self-update engine (M860). The timeout caps how
// long it waits; if exceeded, the function returns true (timedOut)
// with remaining runs still counted. A timeout of zero skips the
// drain wait entirely (cancels runs but does not wait).
//
// Use this instead of Halt() when the caller needs to know whether
// the drain completed within the timeout, e.g. for update vs.
// shutdown decisions.
func (m *Manager) DrainAndHalt(timeout time.Duration) (timedOut bool, activeRuns int) {
	m.host.Suspend("drain") // M1002: classify in-flight runs as resumable BEFORE cancelling them
	m.Halt()                // cancel and mark halted; no-op if already halted
	m.reg.Lock()
	activeRuns = m.reg.Count()
	m.reg.Unlock()
	if timeout <= 0 {
		return false, activeRuns
	}
	settled := make(chan struct{})
	go func() {
		m.reg.Wait()
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
	m.reg.Lock()
	cancel, ok := m.reg.Cancel(corr)
	m.reg.Unlock()
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
	m.reg.Lock()
	if !m.reg.Halted() {
		m.reg.Unlock()
		return
	}
	m.reg.SetHalted(false)
	m.reg.Unlock()
	var payload any
	if reason != "" {
		payload = map[string]any{"reason": reason}
	}
	_, _ = m.host.PublishBus(event.Spec{
		Subject: "kernel.resume",
		Kind:    event.KindResume,
		Actor:   "kernel",
		Payload: payload,
	})
}

// ActiveRuns returns the number of runs currently registered with
// the kernel. Takes the same lock as Halt, so it is safe under
// concurrent run starts/completes.
func (m *Manager) ActiveRuns() int {
	m.reg.Lock()
	defer m.reg.Unlock()
	return m.reg.Count()
}

// ActiveRunIDs returns the correlation ids of the runs in flight
// right now — the live keys of the cancel registry, sorted for
// determinism. Safe under concurrent run starts/completes.
func (m *Manager) ActiveRunIDs() []string {
	m.reg.Lock()
	ids := m.reg.IDs()
	m.reg.Unlock()
	sort.Strings(ids)
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
