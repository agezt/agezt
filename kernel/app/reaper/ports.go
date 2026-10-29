// SPDX-License-Identifier: MIT

package reaper

import (
	"github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/runtime"
)

// The reaper family's ports, filled from the kernel handle.
//
// The APPLICATION owns this. The reaper's scan is the daemon's own — the
// read-only pass that finds dead, degraded, misconfigured and
// routing-troubled agents and stale artifacts — and two adapter surfaces ask
// for it: the reaper's own operations and the roster's status collection.
//
// The port holds the kernel's own surface as a CAPABILITY — the scan — so a
// family that asks for the scan never names the kernel's verb to get it.
//
// The scan MAY answer with nothing: the reaper is a MODULE the composition
// root injects, and a daemon that was given none holds no scanner. The kernel
// answers such a daemon with an EMPTY report — no dead agents, no stale
// artifacts — and so does the port.

// Ports are the reaper family's capabilities.
type Ports struct {
	scan func(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport
}

// PortsFromKernel fills the reaper family's ports from the kernel handle.
//
// The handle may be nil: a host that holds no kernel holds no reaper either,
// and the ports say so instead of calling into nothing.
func PortsFromKernel(k *runtime.Kernel) Ports {
	if k == nil {
		return Ports{}
	}
	return Ports{scan: func(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
		return k.ReaperScan(agentIdleCutoffMs, artifactStaleCutoffMs)
	}}
}

// Scan is the reaper's read-only scan.
//
// The name is not "ReaperScan": a port may not carry the name of the kernel
// verb it stands for, or the adapter that asks the port would be seen reaching
// for the kernel's own verb.
//
// It MAY answer with nothing: a daemon with no reaper module answers an EMPTY
// report, and so does a host with no kernel at all. The surfaces that read it
// show an empty report rather than reading from nothing.
func (p Ports) Scan(agentIdleCutoffMs, artifactStaleCutoffMs int64) api.ReaperReport {
	if p.scan == nil {
		return api.ReaperReport{}
	}
	return p.scan(agentIdleCutoffMs, artifactStaleCutoffMs)
}
