// SPDX-License-Identifier: MIT

// Package epistemic is the platform's EPISTEMIC GUARD: the rule that decides
// whether a tool call needs a human, because the model cannot see what the
// journal remembers.
//
// It is deliberately outside the model. It treats the model's tool call as a
// PROPOSAL, then uses the tool's metadata plus the journaled outcomes to
// decide whether the normal policy verdict needs human escalation.
//
// The guard lives in the platform because a policy rule is infrastructure, not
// a module's business: the runtime (and every other caller) uses it the way
// it uses the Edict engine and the intent model — by asking, never by
// reaching.
//
// The three ports are the host's:
//
//   - Scan walks the journal's policy decisions and tool results (the
//     history the guard reasons over);
//   - Now is the clock the history's age is measured against;
//   - Lookup finds a tool's definition by name.
//
// A host that hands over no journal answers "nothing is known" — the guard
// then treats the tool as novel, which is the SAFE direction: unknown means
// ask.
package epistemic

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
)

// ActionAllow is the decision that leaves the call to the normal policy
// verdict.
const ActionAllow = "allow"

// ActionEscalate is the decision that sends the call to a human.
const ActionEscalate = "escalate"

// historyLimit is how many journal entries the guard keeps while it reasons.
const historyLimit = 4096

// Decision is the guard's answer: what it decided, why, and the signals it
// read to decide.
type Decision struct {
	// Action is ActionAllow or ActionEscalate.
	Action string
	// Reason is the human-readable explanation.
	Reason string
	// Signals are the machine-readable reasons (the dynamic surface, the
	// matched failures, the low confidence, …).
	Signals []string
	// Confidence is the effect confidence the decision was made under.
	Confidence float64
	// FailureMatches is how many journaled failures matched the call's
	// conditions.
	FailureMatches int
	// WeightedFailures is the same count, weighted by how long ago each
	// failure happened.
	WeightedFailures float64
	// SchemaHash is the tool's input schema's fingerprint.
	SchemaHash string
	// InputShape is the call input's shape (its structure, not its values).
	InputShape string
	// Temporal says the call reads the world's time (today's price, the
	// latest release) — a shape whose meaning decays.
	Temporal bool
	// NovelTool says the journal has never seen this tool under these
	// conditions.
	NovelTool bool
}

// Escalates says whether the decision sends the call to a human.
func (d Decision) Escalates() bool { return d.Action == ActionEscalate }

// Ports are the host's seams the guard reasons through.
type Ports struct {
	// Scan walks the journal's policy decisions and tool results.
	//
	// A nil port means the host has no journal: the guard then knows
	// nothing, which makes every tool novel — and unknown means ask.
	Scan func(visit func(*event.Event) error) error

	// Now is the clock the history's age is measured against.
	//
	// A nil port means the host has no clock: the guard then measures no
	// age, which weighs every failure at full strength.
	Now func() time.Time

	// Lookup finds a tool's definition by name — the host's tool map.
	//
	// A nil port means the host has no tool map: the guard then works with
	// the definition the caller handed it, and with nothing else.
	Lookup func(name string) (toolapi.ToolDef, bool)
}

// Gate is the epistemic guard over a host's ports.
type Gate struct {
	ports Ports
}

// New builds the guard over the host's ports.
func New(ports Ports) *Gate { return &Gate{ports: ports} }

// Decide reads the call's proposal and the journal's memory, and answers
// whether a human must see it.
//
// The inputs are the call's identity (toolName, cap), the call itself
// (input), the tool's definition (def — the caller usually has it; the guard
// looks it up when it does not), and the effect the call carries (class and
// confidence).
func (g *Gate) Decide(toolName, cap string, input json.RawMessage, def toolapi.ToolDef, class string, confidence float64) Decision {
	if def.Name == "" && g.ports.Lookup != nil {
		if tool, ok := g.ports.Lookup(toolName); ok {
			def = tool
		}
	}
	if def.Name == "" {
		return Decision{Action: ActionAllow, Reason: "tool definition unavailable; handled before epistemic gate"}
	}
	schemaHash := hashSchema(def.InputSchema)
	inputShape := inputShape(input)
	if confidence <= 0 || confidence > 1 {
		confidence = DefaultEffectConfidence(class)
	}

	signals := make([]string, 0, 5)
	if strings.HasPrefix(toolName, "mcp_") || strings.HasPrefix(toolName, "forge_") {
		signals = append(signals, "dynamic_tool_surface")
	}
	if temporalSensitive(toolName, def.Description, input) {
		signals = append(signals, "temporal_sensitive")
	}
	seenTool, failures, weightedFailures := g.matchHistoricalToolOutcomes(toolName, cap, schemaHash, inputShape, class)
	if !seenTool {
		signals = append(signals, "novel_tool_conditions")
	}
	if failures > 0 {
		signals = append(signals, fmt.Sprintf("matched_failure_conditions:%d", failures))
	}
	if confidence < confidenceFloor(class) {
		signals = append(signals, fmt.Sprintf("low_effect_confidence:%.2f", confidence))
	}
	if schemaPermissive(def.InputSchema) && class != string(toolapi.EffectReadOnly) {
		signals = append(signals, "permissive_schema_effectful_tool")
	}

	decision := Decision{
		Action:           ActionAllow,
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
	if shouldEscalate(class, confidence, failures, weightedFailures, signals) {
		decision.Action = ActionEscalate
		decision.Reason = "epistemic policy requires human review: " + strings.Join(signals, ", ")
	}
	return decision
}

// DefaultEffectConfidence is the confidence an effect class carries when the
// tool declared none.
//
// It is the platform's row: the runtime's approval bundle and the guard both
// need it, and neither may reach into the other's package.
func DefaultEffectConfidence(class string) float64 {
	switch class {
	case string(toolapi.EffectReadOnly):
		return 0.95
	case string(toolapi.EffectReversible):
		return 0.75
	case string(toolapi.EffectCompensable):
		return 0.6
	case string(toolapi.EffectIrreversible):
		return 0.5
	default:
		return 0.4
	}
}
