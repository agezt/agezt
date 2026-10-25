// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's skill-lifecycle tuning. The daemon configures the
//             automatic lifecycle thresholds (auto-quarantine, auto-shadow,
//             auto-promote) on the curator it handed to the kernel.
//
//             The curator is the CONTRACT's port (W3.55), and a daemon with no
//             skill library holds a nil port — so the tuning is a no-op there
//             rather than a panic.

import kernelruntime "github.com/agezt/agezt/kernel/runtime"

// setAutoQuarantine tunes (or disables) the failure-driven auto-quarantine.
func setAutoQuarantine(k *kernelruntime.Kernel, minFailures int, rate float64) {
	if k == nil {
		return
	}
	f := k.Forge()
	if f == nil {
		return
	}
	f.SetAutoQuarantine(minFailures, rate)
}

// setAutoShadow enables or disables draft→shadow auto-staging.
func setAutoShadow(k *kernelruntime.Kernel, on bool) {
	if k == nil {
		return
	}
	f := k.Forge()
	if f == nil {
		return
	}
	f.SetAutoShadow(on)
}

// setAutoPromote tunes (or disables) shadow→active auto-promotion.
func setAutoPromote(k *kernelruntime.Kernel, minWins int, rate float64) {
	if k == nil {
		return
	}
	f := k.Forge()
	if f == nil {
		return
	}
	f.SetAutoPromote(minWins, rate)
}
