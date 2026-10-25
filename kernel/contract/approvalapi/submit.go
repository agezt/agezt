// SPDX-License-Identifier: MIT

package approvalapi

// SubmitSpec is what a caller hands the approval registry: what it wants to
// do, why, and what the operator needs to see to answer.
//
// The row lives here because the workflow engine's port carries it: the engine
// is a MODULE, and a module's port is the contract's. The registry itself —
// the queue, the timeout, the event — stays in the platform (kernel/approval);
// the platform keeps the name its own callers use as an **alias**, so there is
// one row and one spelling of it.
type SubmitSpec struct {
	Capability    string
	ToolName      string
	Input         string
	Reason        string
	Actor         string
	CorrelationID string
	// AutoRec is the auto-recommendation for the operator (e.g., "allow" or "deny").
	AutoRec string
	// ValuePreview is a masked preview of the value being accessed.
	ValuePreview          string
	EffectClass           string
	PredictedEffects      []string
	AffectedResources     []string
	RollbackNotes         string
	Confidence            float64
	CanonicalIntent       string
	HarmfulInterpretation string
	AmbiguityScore        float64
	RegretAxes            map[string]float64
	ConfirmationPrompt    string
}

// Outcome is the registry's answer: the verdict, why it came out that way,
// and who resolved it.
type Outcome struct {
	Decision Decision
	Reason   string
	// ResolvedBy is "operator" (Resolve called) or "system" (timeout /
	// cancel). Future channel sources will set their own identity.
	ResolvedBy string
}
