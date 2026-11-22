// SPDX-License-Identifier: MIT

package main

// The self-repair module's ports, filled from the kernel handle.
//
// The composition root is the one place that knows BOTH the host's kernel and
// the module: a module may not hold the kernel (archcheck's principle P5), so
// the root hands the module the capabilities it needs, one by one.
//
// Until W5.4 the self-repair module held the kernel handle itself and asked
// it for eleven verbs. That was the last cross-module edge in the tree, and it
// was the daemon's own dependency dressed up as the module's business. The
// module's ports (kernel/modules/agents/selfrepair/ports.go) now name the
// CAPABILITIES, and the daemon fills them here.
//
// The two run-context doors — the profile binding and the cost ceiling — are
// the DAEMON'S doors (kernel/runtime/accessors_ctx.go). They exist exactly for
// a module that may not name the runs module: "the profile and the ceiling —
// the self-repair module binds a run through them". The daemon's own ratchet
// pins them (kernel/runtime/runctx_ratchet_test.go), so they stay and the
// module asks for them through a port.

import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/agents/selfrepair"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// selfrepairPorts fills the self-repair module's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no healer either,
// and the ports say so instead of calling into nothing.
func selfrepairPorts(k *kernelruntime.Kernel) selfrepair.Ports {
	if k == nil {
		return selfrepair.Ports{}
	}
	return selfrepair.Ports{
		Bus:          k.Bus(),
		Journal:      selfrepairJournalRange(k),
		LiveProvider: k.Provider,
		Scan:         k.ReaperScan,
		Profile:      k.RosterGet,
		Profiles:     k.RosterList,
		Roster:       k.RosterService(),
		Runner:       k.RunnerService(),
		Correlation:  k.NewCorrelation,
		BindProfile:  k.WithAgentProfile,
		CapCost:      k.WithMaxCost,
	}
}

// selfrepairJournalRange walks the daemon's durable journal once.
//
// A daemon that holds no journal has no repair history: the walk reads
// nothing and the coordinator sees no previous attempt.
func selfrepairJournalRange(k *kernelruntime.Kernel) func(func(*event.Event) error) error {
	return func(fn func(*event.Event) error) error {
		j := k.Journal()
		if j == nil {
			return nil
		}
		return j.Range(fn)
	}
}
