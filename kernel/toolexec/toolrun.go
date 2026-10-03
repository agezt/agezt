// SPDX-License-Identifier: MIT

// Package toolexec retains the legacy tool execution compatibility entry points.
// The single governed pipeline implementation lives in platform/toolpipeline.
package toolexec

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/platform/toolpipeline"
)

// ToolLookup retains the tool registry compatibility type.
type ToolLookup = toolpipeline.ToolLookup

// PolicyChecker retains the policy port compatibility type.
type PolicyChecker = toolpipeline.PolicyChecker

// EventPublisher retains the audit port compatibility type.
type EventPublisher = toolpipeline.EventPublisher

// NoiseNotifier retains the completion port compatibility type.
type NoiseNotifier = toolpipeline.NoiseNotifier

// Options retains the audit representation compatibility type.
type Options = toolpipeline.Options

// Run forwards to the canonical pipeline, retaining the public call contract.
func Run(ctx context.Context, corr, callID, toolName string, args json.RawMessage, tools ToolLookup, policy PolicyChecker, events EventPublisher, noise NoiseNotifier) (toolapi.Result, error) {
	return toolpipeline.Run(ctx, corr, callID, toolName, args, tools, policy, events, noise)
}

// RunWithOptions keeps Run as the legacy entry and uses the shared audit adapter.
func RunWithOptions(ctx context.Context, corr, callID, toolName string, args json.RawMessage, tools ToolLookup, policy PolicyChecker, events EventPublisher, noise NoiseNotifier, options Options) (toolapi.Result, error) {
	return Run(ctx, corr, callID, toolName, args, tools, policy, toolpipeline.WithOutputOptions(events, options), noise)
}
