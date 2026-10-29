// SPDX-License-Identifier: MIT

// Package configtool is the app-layer adapter that lets the config tool — a
// plugin — reach the live daemon without naming the runtime.
//
// The tool's port lives in the contract layer (kernel/contract/configapi).
// Answering it needs the live kernel, and the app layer is the layer that
// may see it: the plugin may not.
package configtool

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/configapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// kernelSource answers the config tool's port from the tool's own ports.
//
// The source holds the PORTS, not the handle they were filled from: a file
// that names the runtime is the port's business, and the adapter that
// answers the contract is not.
type kernelSource struct {
	ports Ports
}

// NewKernelSource builds the config tool's source from its ports.
//
// A host with no daemon holds no daemon to answer for, and the tool is handed
// a NIL port rather than a port that calls into nothing: the refusal is
// written **once**, in the ports, and the adapter does not spell it again.
func NewKernelSource(ports Ports) configapi.Kernel {
	if !ports.Ready() {
		return nil
	}
	return &kernelSource{ports: ports}
}

// UpdateProfile edits the agent named by ref — the agent-scoped write path.
func (s *kernelSource) UpdateProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	return s.ports.EditProfile(ref, mutate)
}

// Reload rebuilds the provider in place — the live-apply path. The
// catalogue the reload produced is dropped: the tool may not name it.
func (s *kernelSource) Reload() error {
	return s.ports.Rebuild()
}

// AgentConfigOverrides returns the config overrides the run in this context
// carries — the daemon's own context convention, read through the port.
func (s *kernelSource) AgentConfigOverrides(ctx context.Context) map[string]string {
	return kernelruntime.AgentConfigOverrides(ctx)
}
