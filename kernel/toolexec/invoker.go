// SPDX-License-Identifier: MIT

package toolexec

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

// Dependencies retains the host ports compatibility type.
type Dependencies = toolpipeline.Dependencies

// Factory retains the per-kernel constructor compatibility type.
type Factory = toolpipeline.Factory

type invoker struct {
	toolphaseapi.Phases
	deps Dependencies
}

// NewInvoker adapts the legacy pipeline to the invocation port. Its canonical
// Run/RunWithOptions behavior remains the standalone compatibility path.
func NewInvoker(deps Dependencies) toolapi.Invoker {
	return &invoker{Phases: toolpipeline.NewInvoker(deps).(toolphaseapi.Phases), deps: deps}
}

func (s *invoker) Invoke(ctx context.Context, call toolapi.Invocation) (toolapi.Result, error) {
	lookup := call.Lookup
	if lookup == nil {
		lookup = s.deps.Tools
	}
	return RunWithOptions(ctx, call.CorrelationID, call.CallID, call.Name, call.Input, lookup, s.deps.Policy, s.deps.Events, s.deps.Noise,
		Options{Artifacts: call.Artifacts, ArtifactThreshold: call.ArtifactThreshold})
}
