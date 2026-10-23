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

// kernelSource answers the config tool's port from the live kernel.
type kernelSource struct {
	k *kernelruntime.Kernel
}

// NewKernelSource builds the kernel-backed source of the config tool's port.
//
// The nil check is on the CONCRETE type, on purpose: a nil *Kernel handed
// back as an interface would be a non-nil interface wrapping nothing, and
// the tool would call into a kernel that is not there instead of reporting
// that the daemon is not wired.
func NewKernelSource(k *kernelruntime.Kernel) configapi.Kernel {
	if k == nil {
		return nil
	}
	return &kernelSource{k: k}
}

// UpdateProfile edits the agent named by ref — the agent-scoped write path.
func (s *kernelSource) UpdateProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	return s.k.UpdateProfile(ref, mutate)
}

// Reload rebuilds the provider in place — the live-apply path. The
// catalogue the reload produced is dropped: the tool may not name it.
func (s *kernelSource) Reload() error {
	_, _, err := s.k.Reload()
	return err
}

// AgentConfigOverrides returns the config overrides the run in this context
// carries — the daemon's own context convention, read through the port.
func (s *kernelSource) AgentConfigOverrides(ctx context.Context) map[string]string {
	return kernelruntime.AgentConfigOverrides(ctx)
}
