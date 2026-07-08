// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"encoding/json"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolexec"
)

// compile-time check: Kernel satisfies the toolexec dependency interfaces.
var _ toolexec.ToolLookup = (*Kernel)(nil)
var _ toolexec.PolicyChecker = (*Kernel)(nil)
var _ toolexec.EventPublisher = (*Kernel)(nil)
var _ toolexec.NoiseNotifier = (*Kernel)(nil)

// LookupTool implements toolexec.ToolLookup.
func (k *Kernel) LookupTool(name string) (agent.Tool, bool) {
	t, ok := k.tools[name]
	return t, ok
}

// CheckPolicy implements toolexec.PolicyChecker.
func (k *Kernel) CheckPolicy(ctx context.Context, tc agent.ToolCall) agent.PolicyVerdict {
	return k.policyHook(ctx, tc)
}

// PublishEvent implements toolexec.EventPublisher.
func (k *Kernel) PublishEvent(spec event.Spec) error {
	_, err := k.bus.Publish(spec)
	return err
}

// NotifyNoise implements toolexec.NoiseNotifier.
func (k *Kernel) NotifyNoise(ctx context.Context, tc agent.ToolCall, res agent.Result) {
	k.completeAgentNoiseNotify(ctx, tc, res)
}

// RunTool executes one registered in-process tool under the same schema and
// policy gate used by agent/workflow tool calls, then journals tool.invoked and
// tool.result under corr. The implementation is delegated to toolexec.Run.
func (k *Kernel) RunTool(ctx context.Context, corr, callID, toolName string, args json.RawMessage) (agent.Result, error) {
	return toolexec.Run(ctx, corr, callID, toolName, args, k, k, k, k)
}
