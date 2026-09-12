// SPDX-License-Identifier: MIT

// Runtime policy helpers (agent-noise + memory + approval bundle + effect-class).
// Code extracted from policy.go during the Day-82 god-file split.
// Public API unchanged.
package runtime


import (
	"context"
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)


func (k *Kernel) agentNoisePolicyDenial(ctx context.Context, tc agent.ToolCall) (string, bool) {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok {
		return "", false
	}
	if tc.Name == "memory" && policy.disableMemoryWrites && memoryToolActionWrites(tc.Input) {
		return "agent noise policy: memory writes are disabled", true
	}
	if tc.Name != "notify" {
		return "", false
	}
	if min := notifySeverityRank(policy.minNotifySeverity); min > notifySeverityRank(notifySeverityFromInput(tc.Input)) {
		return fmt.Sprintf("agent noise policy: notify severity must be at least %s", policy.minNotifySeverity), true
	}
	if policy.minNotifyIntervalSec <= 0 {
		return "", false
	}
	slug, _ := agentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return "", false
	}
	nowMS := time.Now().UnixMilli()
	var st agentNoiseState
	if raw, ok, err := k.state.Get(agentNoiseStateNS, slug); err != nil {
		return "agent noise policy: notify cooldown state unavailable: " + err.Error(), true
	} else if ok {
		_ = json.Unmarshal(raw, &st)
	}
	if st.PendingNotifyMS > 0 && nowMS-st.PendingNotifyMS < int64(agentNoisePendingNotifyTTL/time.Millisecond) {
		return "agent noise policy: notify send already in progress", true
	}
	if st.LastNotifyMS > 0 {
		elapsed := nowMS - st.LastNotifyMS
		minMS := int64(policy.minNotifyIntervalSec) * 1000
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

func (k *Kernel) completeAgentNoiseNotify(ctx context.Context, tc agent.ToolCall, res agent.Result) {
	policy, ok := agentNoisePolicyFromCtx(ctx)
	if !ok || policy.minNotifyIntervalSec <= 0 || tc.Name != "notify" {
		return
	}
	slug, _ := agentIdentFromCtx(ctx)
	if strings.TrimSpace(slug) == "" {
		return
	}
	var st agentNoiseState
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

func notifySeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 3
	case "warning", "warn":
		return 2
	case "info", "":
		return 1
	default:
		return 0
	}
}

type approvalBundle struct {
	EffectClass       string
	PredictedEffects  []string
	AffectedResources []string
	RollbackNotes     string
	Confidence        float64
}

func (k *Kernel) approvalDecisionBundle(toolName string, cap edict.Capability, input json.RawMessage, def agent.ToolDef) approvalBundle {
	effect := def.Effect
	if effect.Class == "" && len(effect.PredictedEffects) == 0 && len(effect.AffectedResources) == 0 {
		if tool, ok := k.tools[toolName]; ok {
			effect = tool.Definition().Effect
		}
	}

	class := normalizeEffectClass(effect.Class)
	if class == "" {
		class = defaultEffectClass(cap)
	}
	resources := append([]string(nil), effect.AffectedResources...)
	if len(resources) == 0 {
		resources = affectedResourcesFromInput(toolName, cap, input)
	}
	predicted := append([]string(nil), effect.PredictedEffects...)
	if len(predicted) == 0 {
		predicted = []string{fmt.Sprintf("invoke %s under %s", toolName, cap)}
	}
	rollback := strings.TrimSpace(effect.RollbackNotes)
	if rollback == "" {
		rollback = defaultRollbackNotes(class)
	}
	confidence := effect.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = defaultEffectConfidence(class)
	}
	return approvalBundle{
		EffectClass:       class,
		PredictedEffects:  predicted,
		AffectedResources: resources,
		RollbackNotes:     rollback,
		Confidence:        confidence,
	}
}

func normalizeEffectClass(class agent.EffectClass) string {
	switch class {
	case agent.EffectReadOnly, agent.EffectReversible, agent.EffectCompensable, agent.EffectIrreversible:
		return string(class)
	default:
		return ""
	}
}

func defaultEffectClass(cap edict.Capability) string {
	switch cap {
	case edict.CapFileRead, edict.CapFileList, edict.CapHTTPGet, edict.CapBrowserRead,
		edict.CapHomeAssistantRead, edict.CapWebSearch, edict.CapRunsRead,
		edict.CapIntrospect, edict.CapConfigRead, edict.CapProviderCall:
		return string(agent.EffectReadOnly)
	case edict.CapFileWrite, edict.CapMemory, edict.CapWorld, edict.CapSchedule,
		edict.CapStanding, edict.CapBoard, edict.CapSkill, edict.CapOversee,
		edict.CapToolForge, edict.CapConfigWrite, edict.CapWorkflow:
		return string(agent.EffectReversible)
	case edict.CapNotify, edict.CapHTTPPost, edict.CapRemoteRun:
		return string(agent.EffectCompensable)
	case edict.CapShell, edict.CapFileDelete, edict.CapCoding, edict.CapACPAgent,
		edict.CapHomeAssistantCall, edict.CapCodeExec, edict.CapMCPInstall, edict.CapMCP,
		edict.CapMarket:
		return string(agent.EffectIrreversible)
	default:
		return string(agent.EffectIrreversible)
	}
}

func affectedResourcesFromInput(toolName string, cap edict.Capability, input json.RawMessage) []string {
	out := []string{"tool:" + toolName, "capability:" + string(cap)}
	var obj map[string]any
	if err := json.Unmarshal(input, &obj); err != nil {
		return out
	}
	for _, key := range []string{"path", "url", "endpoint", "entity_id", "service", "command", "op", "name", "id"} {
		if v, ok := obj[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, key+":"+s)
			}
		}
	}
	return out
}

func defaultRollbackNotes(class string) string {
	switch class {
	case string(agent.EffectReadOnly):
		return "No rollback required for read-only action."
	case string(agent.EffectReversible):
		return "Use the corresponding revert/delete/restore operation or journaled state to undo if needed."
	case string(agent.EffectCompensable):
		return "No guaranteed rollback; compensate with a follow-up action if the outcome is wrong."
	case string(agent.EffectIrreversible):
		return "No reliable rollback path declared; approve only if the effect is acceptable."
	default:
		return "No rollback information declared."
	}
}

func defaultEffectConfidence(class string) float64 {
	switch class {
	case string(agent.EffectReadOnly):
		return 0.95
	case string(agent.EffectReversible):
		return 0.75
	case string(agent.EffectCompensable):
		return 0.6
	case string(agent.EffectIrreversible):
		return 0.5
	default:
		return 0.4
	}
}
