// SPDX-License-Identifier: MIT

// Runtime teardown: Close + closeAll.
// Code extracted from compose.go during the Day-79 god-file split. Public API unchanged.
package runtime


import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/event"
)


// cancelled via Halt, then given a bounded drain window (M883) so a run
// mid-journal-write finishes cleanly instead of racing store teardown.
func (k *Kernel) Close() error {
	k.Suspend("close") // M1002: classify in-flight runs as resumable before Halt cancels them
	k.Halt()           // cancel any in-flight runs first
	// Drain: cancelled runs still need to unwind — publish their terminal
	// task.failed, release fan-out tallies, return from tools that honour the
	// cancel late. Wait bounded; a run wedged in a cancel-ignoring tool must
	// not block shutdown forever.
	drain := k.cfg.ShutdownDrainTimeout
	if drain == 0 {
		drain = DefaultShutdownDrainTimeout
	}
	if drain > 0 {
		settled := make(chan struct{})
		go func() {
			k.runWG.Wait()
			close(settled)
		}()
		t := time.NewTimer(drain)
		select {
		case <-settled:
			t.Stop()
		case <-t.C:
			// Best-effort breadcrumb: the journal is still open here, so the
			// abandonment is auditable. The wedged goroutine dies with the
			// process.
			_, _ = k.bus.Publish(event.Spec{
				Subject: "kernel.shutdown",
				Kind:    event.KindAnomalyDetected,
				Actor:   "kernel",
				Payload: map[string]any{
					"anomaly":  "shutdown_drain_timeout",
					"waited":   drain.String(),
					"detail":   "in-flight runs did not settle after Halt; closing stores anyway",
					"severity": "warning",
				},
			})
		}
	}
	k.closeMCPConns() // detach every live MCP server (kills the children)
	k.bus.Close()
	// Close every store even if an earlier one errors — the previous short-circuit
	// returned on the first error and leaked the remaining handles, notably the
	// journal's OS file descriptor (a held handle blocks a re-Open of the dir on
	// Windows). errors.Join reports all failures. (M477)
	return closeAll(
		k.state.Close,
		k.memoryDir.Close,
		k.worldDir.Close,
		k.skillDir.Close,
		k.journal.Close,
		func() error { return k.agentGW.Close() },
	)
}

// closeAll invokes every close func (none skipped) and joins their errors.
func closeAll(closers ...func() error) error {
	errs := make([]error, 0, len(closers))
	for _, c := range closers {
		errs = append(errs, c())
	}
	return errors.Join(errs...)
}
