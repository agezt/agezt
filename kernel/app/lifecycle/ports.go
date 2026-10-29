// SPDX-License-Identifier: MIT

// Package lifecycle owns the daemon's lifecycle: draining the runs in flight,
// waiting for them, and halting the daemon.
//
// The daemon's OVERVIEW (kernel/app/system) reports what the daemon is doing;
// this package owns what the daemon does to itself when it must stop.
package lifecycle

import (
	"errors"
	"time"

	"github.com/agezt/agezt/kernel/runtime"
)

// The daemon lifecycle family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The daemon's own lifecycle capability — drain the
// runs in flight and halt — is what the update flow asks for: an update that
// lands while runs are still going would be applied under a daemon that is
// changing underneath it.
//
// The port holds the kernel's own surface as a CAPABILITY — the drain — so a
// family that asks for the drain never names the kernel's verb to get it.
//
// The handle MAY be absent: a host that holds no kernel holds no daemon to
// drain, and the port says so instead of calling into nothing.

// errNoDrain is what the port says when the host holds no kernel.
//
// The drain is not a file that may be missing — the daemon always has one —
// but the HOST that holds no kernel holds no daemon, and it must be told the
// drain is unavailable rather than handed a call into nothing.
var errNoDrain = errors.New("the daemon's drain is not available")

// Ports are the daemon lifecycle family's capabilities.
type Ports struct {
	drain func(timeout time.Duration) (bool, int)
}

// PortsFromKernel fills the daemon lifecycle family's ports from the kernel
// handle.
//
// The handle may be nil: a host that holds no kernel holds no daemon to
// drain, and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{drain: func(timeout time.Duration) (bool, int) {
		return k.DrainAndHalt(timeout)
	}}
}

// Drain cancels the runs in flight, waits for them, and halts the daemon.
//
// It reports whether the wait timed out and how many runs were still active
// when it gave up.
//
// The name is not "DrainAndHalt": a port may not carry the name of the kernel
// verb it stands for, or the adapter that asks the port would be seen
// reaching for the kernel's own verb.
//
// It MAY refuse: a host with no kernel has no daemon to drain, and the caller
// must hear that rather than see an idle daemon it never drained.
func (p Ports) Drain(timeout time.Duration) (bool, int, error) {
	if p.drain == nil {
		return false, 0, errNoDrain
	}
	timedOut, active := p.drain(timeout)
	return timedOut, active, nil
}
