// SPDX-License-Identifier: MIT

package runner

// The run engine's roster doors (W3.76o).
//
// The run engine writes the roster's rows — it retires an agent whose run
// completed and advances the agent's cycle counter — and those writes are the
// AGENTS MODULE's operations. Asking the host to ask the module was a hop
// that bought nothing: the run engine may import another module's api
// package, so it asks the module itself.
//
// These tests pin that: the run engine asks the module, and a host that wired
// no roster service leaves the run engine quiet instead of broken.

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	agentsapi "github.com/agezt/agezt/kernel/modules/agents/api"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
)

// stubKernel answers only the doors the run engine opens in these tests.
//
// It EMBEDS the port, so a door nobody opens stays nil — and a test that
// opens one by accident panics loudly instead of passing quietly.
type stubKernel struct {
	KernelAPI
	profile agentapi.Profile
}

// RosterGet answers the host's nil-safe roster read.
func (s *stubKernel) RosterGet(ref string) (agentapi.Profile, bool) {
	if s.profile.Slug == "" || s.profile.Slug != ref {
		return agentapi.Profile{}, false
	}
	return s.profile, true
}

// stubRoster is the AGENTS MODULE's roster service, seen from the run
// engine: it records what the run engine asked the module to write.
type stubRoster struct {
	retired []string
	updated []string
	found   bool
}

// Add is not part of the run engine's business.
func (s *stubRoster) Add(p agentapi.Profile) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

// Update records the edit.
func (s *stubRoster) Update(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error) {
	s.updated = append(s.updated, ref)
	return agentapi.Profile{}, s.found, nil
}

// Remove is not part of the run engine's business.
func (s *stubRoster) Remove(ref string) (bool, error) { return false, nil }

// SetEnabled is not part of the run engine's business.
func (s *stubRoster) SetEnabled(ref string, enabled bool) (agentapi.Profile, error) {
	return agentapi.Profile{}, nil
}

// SetRetired records the retirement.
func (s *stubRoster) SetRetired(ref string, retired bool, reason ...string) (agentapi.Profile, error) {
	if retired {
		s.retired = append(s.retired, ref)
	}
	return agentapi.Profile{}, nil
}

// The stub satisfies the module's face.
var _ agentsapi.RosterService = (*stubRoster)(nil)

// TestTheRunEngineAsksTheAgentsModuleToRetireTheAgent: an agent whose run
// completed is retired THROUGH the module — the run engine does not ask the
// host to ask the module.
func TestTheRunEngineAsksTheAgentsModuleToRetireTheAgent(t *testing.T) {
	k := &stubKernel{profile: agentapi.Profile{
		Slug:      "worker",
		Lifecycle: agentapi.AgentLifecycle{RetireOnComplete: true},
	}}
	roster := &stubRoster{}
	r := New(k, roster, testState())

	ctx := runctx.WithAgentIdent(context.Background(), "worker", 0)
	r.CompleteAgentLifecycle(ctx, "corr-1")

	if len(roster.retired) != 1 || roster.retired[0] != "worker" {
		t.Errorf("the module was asked to retire %v, want the worker", roster.retired)
	}
}

// TestTheRunEngineStaysQuietWithoutARosterService: a host that wired no
// roster service has no roster to write. The run engine's own business (the
// run finished) is done either way — and nothing panics.
func TestTheRunEngineStaysQuietWithoutARosterService(t *testing.T) {
	k := &stubKernel{profile: agentapi.Profile{
		Slug:      "worker",
		Lifecycle: agentapi.AgentLifecycle{RetireOnComplete: true},
	}}
	r := New(k, nil, testState())

	ctx := runctx.WithAgentIdent(context.Background(), "worker", 0)
	r.CompleteAgentLifecycle(ctx, "corr-1")
}

// TestTheRunEngineLeavesAnUnknownAgentAlone: the host's read says there is
// no such agent, so the run engine asks the module nothing.
func TestTheRunEngineLeavesAnUnknownAgentAlone(t *testing.T) {
	k := &stubKernel{}
	roster := &stubRoster{}
	r := New(k, roster, testState())

	ctx := runctx.WithAgentIdent(context.Background(), "ghost", 0)
	r.CompleteAgentLifecycle(ctx, "corr-1")

	if len(roster.retired) != 0 || len(roster.updated) != 0 {
		t.Errorf("the module was asked to write %v/%v for an agent that does not exist", roster.retired, roster.updated)
	}
}
