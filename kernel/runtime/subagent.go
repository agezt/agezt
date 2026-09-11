// SPDX-License-Identifier: MIT

// Sub-agent tool types: subAgentTool, subAgentAwaitTool, plus their zero-arg constructors.
// Code extracted from subagent.go during the Day-40 god-file split. Public API unchanged.
package runtime


import (
	"context"
)



// subAgentTool is the in-process `delegate` tool (P6-MULTI-01). Its runners
// are wired to k.runSubAgent / k.runSubAgentAsync after the kernel is
// constructed (the tool is built during Open before *Kernel exists).
type subAgentTool struct {
	run   func(ctx context.Context, task, model, taskType, agentRef string) (string, error)
	spawn func(ctx context.Context, task, model, taskType, agentRef string) (string, error)
}
