// SPDX-License-Identifier: MIT

// Payload extraction helpers for run entries: extractIntent/Agent/Tool/Iters/CostMicrocents/Model/AnswerPreview/SpawnLink/Reason and int64Arg.
// Code extracted from runs.go during the Day-37 god-file split. Public API unchanged.
package controlplane


import (
	"encoding/json"
	"strings"
)


func extractIntent(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Intent
}

// extractAgent pulls "agent" out of a task.received payload.
// Returns "" if missing or malformed. An empty string indicates a
// chat run started without --agent; a non-empty slug indicates a run
// started as a named roster agent (M73).
func extractAgent(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Agent string `json:"agent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Agent
}

// extractTool pulls "tool" out of a tool.invoked / tool.result payload
// (agent.go writes it as {"tool": "<name>", ...}). Returns "" if missing or
// malformed — the run then reports its phase without a tool name.
func extractTool(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Tool string `json:"tool"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Tool
}

// extractIters pulls "iters" out of a task.completed payload.
// Returns 0 on parse failure for the same reason as extractIntent.
func extractIters(payload json.RawMessage) int {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		// JSON numbers decode as float64 by default; accept either
		// shape so the field's wire type can evolve without breaking.
		Iters float64 `json:"iters"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int(p.Iters)
}

// extractCostMicrocents pulls cost_microcents out of a budget.consumed
// payload (M47). Returns 0 on parse failure or absence, so an unparseable
// spend event contributes nothing rather than crashing the fold. JSON
// numbers decode as float64; int64(...) truncates the fractional part the
// integer microcents never has.
func extractCostMicrocents(payload json.RawMessage) int64 {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		Cost float64 `json:"cost_microcents"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int64(p.Cost)
}

// int64Arg decodes a control-plane numeric arg to int64. JSON numbers arrive as
// float64; an absent or non-numeric arg yields 0. Shared by the cost-band filter
// (M125) and any other handler that takes an integer arg.
func int64Arg(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	default:
		return 0
	}
}

// extractModel pulls the model name out of a budget.consumed payload (M123).
// Returns "" on parse failure or absence, so an unparseable or model-less spend
// event leaves the run's model unset rather than crashing the fold.
func extractModel(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Model
}

// answerPreviewRunes bounds the one-line answer excerpt folded into a run row
// (M52). Short enough for a single inline ↳ line; the full answer is a
// `agt runs show <child>` away.
const answerPreviewRunes = 80

// extractAnswerPreview pulls the M51 `answer` out of a task.completed payload
// and returns a one-line excerpt (M52): newlines/tabs collapsed to single
// spaces, trimmed, truncated to answerPreviewRunes with an ellipsis. Returns ""
// on parse failure or an empty/whitespace answer so the renderer simply omits it.
func extractAnswerPreview(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Answer string `json:"answer"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	one := strings.Join(strings.Fields(p.Answer), " ") // collapse all whitespace runs
	if one == "" {
		return ""
	}
	r := []rune(one)
	if len(r) > answerPreviewRunes {
		return string(r[:answerPreviewRunes]) + "…"
	}
	return one
}

// extractSpawnLink pulls child + parent correlation ids out of a
// subagent.spawned payload (M41). Returns ("","") on parse failure so the
// fold simply skips an unparseable delegation rather than crashing.
func extractSpawnLink(payload json.RawMessage) (child, parent string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		Child  string `json:"child_correlation"`
		Parent string `json:"parent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.Child, p.Parent
}

// extractReason pulls "reason" out of a task.failed payload (M30) —
// the classified failure tag (error|max_iters|canceled|timeout). Returns
// "" on parse failure or absence, so the renderer falls back gracefully.
func extractReason(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Reason
}
