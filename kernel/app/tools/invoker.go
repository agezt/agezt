// SPDX-License-Identifier: MIT

// Package tools is the application entry for governed direct tool invocations.
// It binds the shared platform execution mechanism to host-owned ports.
package tools

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

type invoker struct{ pipeline toolapi.Invoker }

// NewInvoker binds host-owned policy/audit/completion ports for one kernel.
// It preserves the existing execution pipeline while callers converge on the
// lower-layer invocation port. The agent loop has not converged on this entry.
func NewInvoker(deps toolpipeline.Dependencies) toolapi.Invoker {
	return &invoker{pipeline: toolpipeline.NewInvoker(deps)}
}

func (s *invoker) Invoke(ctx context.Context, call toolapi.Invocation) (toolapi.Result, error) {
	return s.pipeline.Invoke(ctx, call)
}
