// SPDX-License-Identifier: MIT

// Package tools is the application entry for governed direct tool invocations.
// It binds the shared platform execution mechanism to host-owned ports.
package tools

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

type invoker struct {
	toolphaseapi.Phases
	pipeline toolapi.Invoker
}

// NewInvoker binds host-owned policy/audit/completion ports for one kernel.
// Direct calls use Invoke; root and delegated loops use the same service's
// phase port so batch admission, memoization and scheduling stay caller-owned.
func NewInvoker(deps toolpipeline.Dependencies) toolapi.Invoker {
	pipeline := toolpipeline.NewInvoker(deps)
	return &invoker{Phases: pipeline.(toolphaseapi.Phases), pipeline: pipeline}
}

func (s *invoker) Invoke(ctx context.Context, call toolapi.Invocation) (toolapi.Result, error) {
	return s.pipeline.Invoke(ctx, call)
}
