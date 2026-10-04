// SPDX-License-Identifier: MIT

package toolpipeline

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/contract/toolphaseapi"
	"github.com/agezt/agezt/kernel/event"
)

var _ toolphaseapi.Phases = (*invoker)(nil)

func (s *invoker) Resolve(call llm.ToolCall, lookup func(string) (toolapi.Tool, bool)) Resolution {
	if lookup == nil {
		lookup = s.deps.Tools.LookupTool
	}
	return Resolve(call, lookup)
}

func (*invoker) Decide(ctx context.Context, call llm.ToolCall, def toolapi.ToolDef, policy policyapi.Policy, audit func(llm.ToolCall, policyapi.PolicyVerdict) error) (Decision, error) {
	return Decide(ctx, call, def, policy, audit)
}

func (*invoker) Announce(call llm.ToolCall, publish func(string, map[string]any) error) error {
	return Announce(call, func(kind event.Kind, payload map[string]any) error { return publish(string(kind), payload) })
}

func (*invoker) Execute(ctx context.Context, tool toolapi.Tool, input json.RawMessage, timeout time.Duration, panicError func(any) error) Execution {
	return Execute(ctx, tool, input, timeout, panicError)
}

func (*invoker) Settle(call llm.ToolCall, result toolapi.Result, fields map[string]any, publish func(string, map[string]any) error) error {
	return Settle(call, result, fields, func(kind event.Kind, payload map[string]any) error { return publish(string(kind), payload) })
}
