// SPDX-License-Identifier: MIT

package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)


const epistemicHistoryLimit = 4096

type epistemicDecision struct {
	Action           string
	Reason           string
	Signals          []string
	Confidence       float64
	FailureMatches   int
	WeightedFailures float64
	SchemaHash       string
	InputShape       string
	Temporal         bool
	NovelTool        bool
}

func (d epistemicDecision) escalates() bool { return d.Action == "escalate" }

// epistemicGate is deliberately outside the model. It treats the model's tool
// call as a proposal, then uses tool metadata plus journaled outcomes to decide
// whether the normal policy verdict needs human escalation.
func (k *Kernel) epistemicGate(toolName string, cap edict.Capability, input json.RawMessage, def agent.ToolDef, bundle approvalBundle) epistemicDecision {
	if def.Name == "" {
		if tool, ok := k.tools[toolName]; ok {
			def = tool.Definition()
		}
	}
	if def.Name == "" {
		return epistemicDecision{Action: "allow", Reason: "tool definition unavailable; handled before epistemic gate"}
	}
	schemaHash := hashSchema(def.InputSchema)
	inputShape := inputShape(input)
	class := bundle.EffectClass
	confidence := bundle.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = defaultEffectConfidence(class)
	}

	signals := make([]string, 0, 5)
	if strings.HasPrefix(toolName, "mcp_") || strings.HasPrefix(toolName, "forge_") {
		signals = append(signals, "dynamic_tool_surface")
	}
	if temporalSensitive(toolName, def.Description, input) {
		signals = append(signals, "temporal_sensitive")
	}
	seenTool, failures, weightedFailures := k.matchHistoricalToolOutcomes(toolName, string(cap), schemaHash, inputShape, class)
	if !seenTool {
		signals = append(signals, "novel_tool_conditions")
	}
	if failures > 0 {
		signals = append(signals, fmt.Sprintf("matched_failure_conditions:%d", failures))
	}
	if confidence < confidenceFloor(class) {
		signals = append(signals, fmt.Sprintf("low_effect_confidence:%.2f", confidence))
	}
	if schemaPermissive(def.InputSchema) && class != string(agent.EffectReadOnly) {
		signals = append(signals, "permissive_schema_effectful_tool")
	}

	decision := epistemicDecision{
		Action:           "allow",
		Reason:           "epistemic policy allowed",
		Signals:          signals,
		Confidence:       confidence,
		FailureMatches:   failures,
		WeightedFailures: weightedFailures,
		SchemaHash:       schemaHash,
		InputShape:       inputShape,
		Temporal:         containsSignal(signals, "temporal_sensitive"),
		NovelTool:        !seenTool,
	}
	if shouldEscalateEpistemic(class, confidence, failures, weightedFailures, signals) {
		decision.Action = "escalate"
		decision.Reason = "epistemic policy requires human review: " + strings.Join(signals, ", ")
	}
	return decision
}
