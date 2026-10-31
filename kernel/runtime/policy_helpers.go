// SPDX-License-Identifier: MIT

package runtime

// Provenance: Runtime policy helpers (agent-noise + memory + approval bundle +
//             effect-class). Code extracted from policy.go during the Day-82
//             god-file split. Public API unchanged.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"encoding/json"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/modules/runs/runctx"
	"github.com/agezt/agezt/kernel/platform/policy/admission"
)

// agentNoiseStateNS is the daemon's state-store namespace for the per-agent
// noise bookkeeping. The namespace is the daemon's own (it owns the state
// store); the ROW it holds is the run context's (runctx.AgentNoiseState).
const agentNoiseStateNS = "agent_noise"

func (k *Kernel) agentNoisePolicyDenial(ctx context.Context, tc llm.ToolCall) (string, bool) {
	policy, ok := runctx.AgentNoisePolicyFromCtx(ctx)
	if !ok {
		return "", false
	}
	if tc.Name == "memory" && policy.DisableMemoryWrites && memoryToolActionWrites(tc.Input) {
		return "agent noise policy: memory writes are disabled", true
	}
	if tc.Name != "notify" {
		return "", false
	}
	if min := runctx.NotifySeverityRank(policy.MinNotifySeverity); min > runctx.NotifySeverityRank(notifySeverityFromInput(tc.Input)) {
		return fmt.Sprintf("agent noise policy: notify severity must be at least %s", policy.MinNotifySeverity), true
	}
	if policy.MinNotifyIntervalSec <= 0 {
		return "", false
	}
	slug, _ := runctx.AgentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return "", false
	}
	nowMS := time.Now().UnixMilli()
	var st runctx.AgentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	if st.PendingNotifyMS > 0 && nowMS-st.PendingNotifyMS < int64(runctx.AgentNoisePendingNotifyTTL/time.Millisecond) {
		return "agent noise policy: notify send already in progress", true
	}
	if st.LastNotifyMS > 0 {
		elapsed := nowMS - st.LastNotifyMS
		minMS := int64(policy.MinNotifyIntervalSec) * 1000
		if elapsed < minMS {
			remaining := time.Duration(minMS-elapsed) * time.Millisecond
			return "agent noise policy: notify cooldown active for " + remaining.Round(time.Second).String(), true
		}
	}
	st.PendingNotifyMS = nowMS
	if err := k.state.Set(agentNoiseStateNS, slug, st); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	}
	return "", false
}

func (k *Kernel) completeAgentNoiseNotify(ctx context.Context, tc llm.ToolCall, res toolapi.Result) {
	policy, ok := runctx.AgentNoisePolicyFromCtx(ctx)
	if !ok || policy.MinNotifyIntervalSec <= 0 || tc.Name != "notify" {
		return
	}
	slug, _ := runctx.AgentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return
	}
	var st runctx.AgentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	st.PendingNotifyMS = 0
	if !res.IsError {
		st.LastNotifyMS = time.Now().UnixMilli()
	}
	_ = k.state.Set(agentNoiseStateNS, slug, st)
}

func memoryToolActionWrites(raw json.RawMessage) bool {
	var in struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(in.Action)) {
	case "remember", "forget", "bulk_forget":
		return true
	default:
		return false
	}
}

func notifySeverityFromInput(raw json.RawMessage) string {
	var in struct {
		Severity string `json:"severity"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return "info"
	}
	severity := strings.ToLower(strings.TrimSpace(in.Severity))
	if severity == "" {
		return "info"
	}
	return severity
}

// approvalDecisionBundle assembles what the operator is shown about a call.
//
// The rule is the platform's (W3.75): the effect class, the predicted
// effects, the resources, the rollback notes and the confidence. The daemon
// keeps the doors — its tool map — and asks.
func (k *Kernel) approvalDecisionBundle(toolName string, cap edict.Capability, input json.RawMessage, def toolapi.ToolDef) admission.Bundle {
	return admission.BundleFor(toolName, cap, input, def, k.lookupToolDef)
}

// lookupToolDef finds a tool's definition by name — the host's tool map.
func (k *Kernel) lookupToolDef(name string) (toolapi.ToolDef, bool) {
	if k == nil {
		return toolapi.ToolDef{}, false
	}
	tool, ok := k.tools[name]
	if !ok {
		return toolapi.ToolDef{}, false
	}
	return tool.Definition(), true
}
