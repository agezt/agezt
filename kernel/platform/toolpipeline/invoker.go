// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Dependencies are the host-owned ports bound once for one kernel. They do not
// capture per-call lookup overrides or effective artifact configuration.
type Dependencies struct {
	Tools  ToolLookup
	Policy PolicyChecker
	Events EventPublisher
	Noise  NoiseNotifier
}

// Factory constructs an invocation service for a single host. The runtime can
// receive an app constructor without importing the higher application layer.
type Factory func(Dependencies) toolapi.Invoker

type invoker struct{ deps Dependencies }

// NewInvoker adapts the legacy pipeline to the invocation port. Its canonical
// Run/RunWithOptions behavior remains the standalone compatibility path.
func NewInvoker(deps Dependencies) toolapi.Invoker { return &invoker{deps: deps} }

func (s *invoker) Invoke(ctx context.Context, call toolapi.Invocation) (toolapi.Result, error) {
	lookup := call.Lookup
	if lookup == nil {
		lookup = s.deps.Tools
	}
	return RunWithOptions(ctx, call.CorrelationID, call.CallID, call.Name, call.Input, lookup, s.deps.Policy, s.deps.Events, s.deps.Noise,
		Options{Artifacts: call.Artifacts, ArtifactThreshold: call.ArtifactThreshold, Phases: s})
}
